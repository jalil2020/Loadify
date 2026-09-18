package usecase

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"math"
	"math/big"
	"slices"
	"time"

	"github.com/google/uuid"

	"github.com/loadify/backend/internal/config"
	"github.com/loadify/backend/internal/domain"
	"github.com/loadify/backend/internal/dto"
	"github.com/loadify/backend/internal/infrastructure/cache"
	"github.com/loadify/backend/internal/matching"
	"github.com/loadify/backend/pkg/apperror"
	"github.com/loadify/backend/pkg/logger"
)

// previewTTL is how long a reviewed list stays publishable. Ten minutes is long
// enough to read it and short enough that the world has not moved on.
const previewTTL = 10 * time.Minute

// PublishUsecase turns a reviewed advert into queued messages.
//
// Nothing here talks to Telegram. It decides what should be sent, writes that
// down, and leaves the sending to the worker — which is what makes the decision
// auditable and the sending restartable.
type PublishUsecase struct {
	ads        domain.LoadAdRepository
	accounts   domain.TelegramAccountRepository
	groups     domain.TelegramGroupRepository
	cargoTypes domain.CargoTypeRepository
	posting    domain.PostingRepository
	guard      domain.PostingGuard
	quota      domain.QuotaGuard
	cache      cache.Cache
	limits     config.TelegramConfig
	audit      domain.AuditLogger
	log        logger.Logger
	now        func() time.Time
}

// NewPublishUsecase wires the publish flow.
func NewPublishUsecase(
	ads domain.LoadAdRepository,
	accounts domain.TelegramAccountRepository,
	groups domain.TelegramGroupRepository,
	cargoTypes domain.CargoTypeRepository,
	posting domain.PostingRepository,
	guard domain.PostingGuard,
	quota domain.QuotaGuard,
	c cache.Cache,
	limits config.TelegramConfig,
	audit domain.AuditLogger,
	log logger.Logger,
) *PublishUsecase {
	return &PublishUsecase{
		ads: ads, accounts: accounts, groups: groups, cargoTypes: cargoTypes,
		posting: posting, guard: guard, quota: quota, cache: c, limits: limits,
		audit: audit, log: log, now: time.Now,
	}
}

// preview is what a person reviewed, kept so the publish that follows sends
// exactly that and not whatever the filter would return a minute later.
type preview struct {
	CompanyID   uuid.UUID   `json:"company_id"`
	UserID      uuid.UUID   `json:"user_id"`
	LoadAdID    uuid.UUID   `json:"load_ad_id"`
	AccountID   uuid.UUID   `json:"account_id"`
	Fingerprint string      `json:"fingerprint"`
	GroupIDs    []uuid.UUID `json:"group_ids"`
}

// Preview works out where an advert would go, and explains every group it would
// not go to. It sends nothing and consumes no allowance.
func (u *PublishUsecase) Preview(
	ctx context.Context,
	userID, companyID, adID uuid.UUID,
	accountID *uuid.UUID,
) (*dto.PublishPreviewResponse, error) {
	ad, err := u.ads.FindByID(ctx, companyID, adID)
	if err != nil {
		return nil, err
	}
	if missing := ad.MissingForPublish(); len(missing) > 0 {
		return nil, apperror.ErrValidation.
			WithMessage("This advert still needs: " + joinWords(missing))
	}
	if ad.Status == domain.LoadAdArchived {
		return nil, apperror.ErrConflict.WithMessage("This advert is archived.")
	}

	account, err := u.resolveAccount(ctx, companyID, userID, accountID)
	if err != nil {
		return nil, err
	}

	candidates, err := u.candidates(ctx, companyID, account.ID, ad)
	if err != nil {
		return nil, err
	}
	now := u.now()
	ranked := matching.Rank(ad, candidates, now)

	targets := u.targets(ad, account.ID, companyID, userID, ranked.Ranked)
	decisions, err := u.guard.Plan(ctx, targets, now)
	if err != nil {
		return nil, err
	}
	queued, err := u.posting.QueuedGroups(ctx, companyID, ad.ID)
	if err != nil {
		return nil, err
	}

	response := &dto.PublishPreviewResponse{
		AccountID:      account.ID,
		Candidates:     len(candidates),
		Approved:       []dto.GroupDecision{},
		Skipped:        []dto.GroupDecision{},
		MessagePreview: messageFor(ad, previewLanguage(ranked.Ranked)),
		ExpiresAt:      now.Add(previewTTL),
	}

	byGroup := make(map[uuid.UUID]domain.GuardDecision, len(decisions))
	for _, decision := range decisions {
		byGroup[decision.GroupID] = decision
	}

	approvedIDs := make([]uuid.UUID, 0, len(ranked.Ranked))
	for _, candidate := range ranked.Ranked {
		entry := dto.GroupDecision{
			GroupID: candidate.Candidate.Group.ID,
			Title:   candidate.Candidate.Group.Title,
			Score:   candidate.Score,
			Reasons: reasonStrings(candidate.Reasons),
		}
		if queued[entry.GroupID] {
			// Already waiting there from an earlier publish. Offering it again
			// would only make the publish collide with its own queue.
			entry.Reason = string(domain.SkipAlreadyQueued)
			response.Skipped = append(response.Skipped, entry)
			continue
		}
		decision := byGroup[candidate.Candidate.Group.ID]
		if decision.Allowed {
			approvedIDs = append(approvedIDs, entry.GroupID)
			response.Approved = append(response.Approved, entry)
			continue
		}
		entry.Reason = string(decision.Reason)
		entry.RetryAfterSeconds = int(decision.RetryAfter.Seconds())
		response.Skipped = append(response.Skipped, entry)
	}

	// A group the matcher refused is shown too: "not offered" without a reason
	// is the kind of silence that makes people stop trusting the tool.
	for _, excluded := range ranked.Excluded {
		response.Skipped = append(response.Skipped, dto.GroupDecision{
			GroupID: excluded.Candidate.Group.ID,
			Title:   excluded.Candidate.Group.Title,
			Reason:  string(excluded.Reason),
		})
	}

	token, err := u.storePreview(ctx, preview{
		CompanyID:   companyID,
		UserID:      userID,
		LoadAdID:    adID,
		AccountID:   account.ID,
		Fingerprint: ad.Fingerprint,
		GroupIDs:    approvedIDs,
	})
	if err != nil {
		return nil, err
	}
	response.PreviewToken = token
	response.PostsLeft = u.postsLeft(ctx, companyID)
	return response, nil
}

// postsLeft is the month's remaining allowance, or nil for none. The preview is
// still useful without it, so a failure to read it is logged, not returned.
func (u *PublishUsecase) postsLeft(ctx context.Context, companyID uuid.UUID) *int {
	if u.quota == nil {
		return nil
	}
	status, err := u.quota.Status(ctx, companyID, domain.QuotaPostsPerMonth)
	if err != nil {
		u.log.Warn("post allowance not read for the preview", logger.Err(err))
		return nil
	}
	if status.Unlimited() {
		return nil
	}
	left := status.Remaining()
	return &left
}

// Publish queues the advert for the groups the person reviewed.
func (u *PublishUsecase) Publish(
	ctx context.Context,
	userID, companyID, adID uuid.UUID,
	idempotencyKey string,
	req dto.PublishRequest,
) (*dto.PublishResponse, error) {
	if idempotencyKey == "" {
		return nil, apperror.ErrBadRequest.
			WithMessage("An Idempotency-Key header is required, so a retried request cannot post twice.")
	}

	// A retry of the same request returns the same run rather than sending the
	// advert a second time.
	if existing, err := u.posting.FindRunByIdempotencyKey(ctx, companyID, idempotencyKey); err == nil {
		return u.runResponse(ctx, companyID, existing)
	} else if !apperror.Is(err, apperror.ErrNotFound.Code) {
		return nil, err
	}

	reviewed, err := u.loadPreview(ctx, req.PreviewToken)
	if err != nil {
		return nil, err
	}
	if reviewed.CompanyID != companyID || reviewed.UserID != userID || reviewed.LoadAdID != adID {
		return nil, apperror.ErrForbidden.WithMessage("This preview belongs to another request.")
	}

	ad, err := u.ads.FindByID(ctx, companyID, adID)
	if err != nil {
		return nil, err
	}
	if ad.Fingerprint != reviewed.Fingerprint {
		return nil, apperror.ErrConflict.
			WithMessage("The advert changed after you reviewed it. Check the groups again.")
	}

	account, err := u.accounts.FindForUser(ctx, companyID, userID, reviewed.AccountID)
	if err != nil {
		return nil, err
	}
	if readiness := account.ReadyToPost(u.now()); !readiness.Ready {
		return nil, accountNotReady(readiness)
	}

	wanted := reviewed.GroupIDs
	if len(req.GroupIDs) > 0 {
		// A narrowing, never an addition: a group that was not reviewed cannot
		// be posted to by naming it here.
		wanted = intersect(reviewed.GroupIDs, req.GroupIDs)
	}
	if len(wanted) == 0 {
		return nil, apperror.ErrValidation.
			WithMessage("No group from that preview is left to publish to.")
	}

	run := &domain.PublishRun{
		CompanyID:         companyID,
		LoadAdID:          adID,
		AccountID:         account.ID,
		RequestedByUserID: userID,
		IdempotencyKey:    idempotencyKey,
		Candidates:        len(reviewed.GroupIDs),
	}

	queued, err := u.posting.QueuedGroups(ctx, companyID, adID)
	if err != nil {
		return nil, err
	}
	jobs, titles, err := u.buildJobs(ctx, companyID, account.ID, ad, wanted, queued)
	if err != nil {
		return nil, err
	}
	for _, job := range jobs {
		if job.Status == domain.PostingSkipped {
			run.Skipped++
			continue
		}
		run.Approved++
	}

	// Only what will be attempted counts against the plan; a group the guard
	// already refused is written down but costs nothing.
	err = reserve(ctx, u.quota, companyID, domain.QuotaPostsPerMonth, run.Approved, func(ctx context.Context) error {
		return u.posting.CreateRun(ctx, run, jobs)
	})
	if err != nil {
		if apperror.Is(err, apperror.ErrConflict.Code) {
			// Two requests raced on the same key; the other one won.
			if existing, lookupErr := u.posting.FindRunByIdempotencyKey(ctx, companyID, idempotencyKey); lookupErr == nil {
				return u.runResponse(ctx, companyID, existing)
			}
		}
		return nil, err
	}

	if err := u.ads.UpdateColumns(ctx, companyID, adID, map[string]any{
		"status":       domain.LoadAdPublishing,
		"published_at": u.now(),
	}); err != nil {
		u.log.Warn("advert status not updated after publish",
			logger.String("load_ad_id", adID.String()), logger.Err(err))
	}

	// The token is single use: publishing twice from one review is exactly what
	// the idempotency key and this deletion are both there to prevent.
	if err := u.cache.Delete(ctx, previewKey(req.PreviewToken)); err != nil {
		u.log.Warn("preview token not cleared", logger.Err(err))
	}
	u.recordPublish(ctx, userID, run)

	return u.response(run, jobs, titles), nil
}

// resolveAccount picks the account to post from: the named one, or the
// person's own when they have exactly one usable.
func (u *PublishUsecase) resolveAccount(
	ctx context.Context,
	companyID, userID uuid.UUID,
	accountID *uuid.UUID,
) (*domain.TelegramAccount, error) {
	if accountID != nil {
		account, err := u.accounts.FindForUser(ctx, companyID, userID, *accountID)
		if err != nil {
			return nil, err
		}
		if readiness := account.ReadyToPost(u.now()); !readiness.Ready {
			return nil, accountNotReady(readiness)
		}
		return account, nil
	}

	accounts, err := u.accounts.ListByUser(ctx, companyID, userID)
	if err != nil {
		return nil, err
	}
	var lastReason domain.PostReadiness
	for i := range accounts {
		readiness := accounts[i].ReadyToPost(u.now())
		if readiness.Ready {
			return &accounts[i], nil
		}
		lastReason = readiness
	}
	if lastReason.Reason != "" {
		return nil, accountNotReady(lastReason)
	}
	return nil, apperror.ErrForbidden.
		WithMessage("Connect your Telegram account before publishing.")
}

// candidates runs the deterministic filter.
func (u *PublishUsecase) candidates(
	ctx context.Context,
	companyID, accountID uuid.UUID,
	ad *domain.LoadAd,
) ([]domain.GroupCandidate, error) {
	query := domain.GroupCandidateQuery{
		AccountID:      accountID,
		OriginCountry:  ad.OriginCountry,
		OriginRegionID: ad.OriginRegionID,
		DestCountry:    ad.DestCountry,
		DestRegionID:   ad.DestRegionID,
		VehicleType:    ad.VehicleType,
		WeightKg:       ad.WeightKg,
	}
	if ad.CargoTypeID != nil {
		// Group rules are written in cargo codes, because that is what a person
		// picks from a list; the advert carries the id it resolved to.
		if cargo, err := u.cargoTypes.FindByID(ctx, *ad.CargoTypeID); err == nil {
			code := cargo.Code
			query.CargoTypeCode = &code
		}
	}
	return u.groups.Candidates(ctx, companyID, query)
}

// targets turns ranked groups into what the guard needs to judge them.
func (u *PublishUsecase) targets(
	ad *domain.LoadAd,
	accountID, companyID, userID uuid.UUID,
	ranked []matching.Ranked,
) []domain.GuardTarget {
	targets := make([]domain.GuardTarget, 0, len(ranked))
	for _, candidate := range ranked {
		group, rules := candidate.Candidate.Group, candidate.Candidate.Rules
		targets = append(targets, domain.GuardTarget{
			CompanyID:   companyID,
			UserID:      userID,
			AccountID:   accountID,
			GroupID:     group.ID,
			PeerID:      group.TgPeerID,
			Fingerprint: ad.Fingerprint,
			Cooldown:    rules.Cooldown(group.SlowmodeSeconds, u.limits.GroupCooldown),
			GroupHourly: int(rules.HourlyLimit),
			GroupDaily:  int(rules.DailyLimit),
		})
	}
	return targets
}

// buildJobs makes one job per group, each carrying the message it will send and
// the random id Telegram will refuse a second time.
func (u *PublishUsecase) buildJobs(
	ctx context.Context,
	companyID, accountID uuid.UUID,
	ad *domain.LoadAd,
	groupIDs []uuid.UUID,
	queued map[uuid.UUID]bool,
) ([]domain.PostingJob, map[uuid.UUID]string, error) {
	jobs := make([]domain.PostingJob, 0, len(groupIDs))
	titles := make(map[uuid.UUID]string, len(groupIDs))
	now := u.now()

	for _, groupID := range groupIDs {
		group, err := u.groups.FindByID(ctx, companyID, groupID)
		if err != nil {
			if apperror.Is(err, apperror.ErrNotFound.Code) {
				continue
			}
			return nil, nil, err
		}
		titles[group.ID] = group.Title

		randomID, err := randomInt63()
		if err != nil {
			return nil, nil, err
		}

		job := domain.PostingJob{
			ID:            uuid.New(),
			CompanyID:     companyID,
			LoadAdID:      ad.ID,
			AccountID:     accountID,
			GroupID:       group.ID,
			Fingerprint:   ad.Fingerprint,
			MessageText:   messageFor(ad, group.Language),
			Status:        domain.PostingPending,
			RandomID:      randomID,
			NextAttemptAt: now,
		}
		// A group that stopped being postable between the preview and now, or
		// that another publish queued this advert for in the meantime, is
		// recorded as skipped rather than dropped, so the run still adds up.
		switch {
		case !group.Postable():
			job.Status, job.SkipReason = domain.PostingSkipped, skipReason(domain.SkipGroupUnavailable)
		case queued[group.ID]:
			job.Status, job.SkipReason = domain.PostingSkipped, skipReason(domain.SkipAlreadyQueued)
		}
		jobs = append(jobs, job)
	}

	if len(jobs) == 0 {
		return nil, nil, apperror.ErrValidation.
			WithMessage("None of those groups can be posted to any more.")
	}
	return jobs, titles, nil
}

func (u *PublishUsecase) response(run *domain.PublishRun, jobs []domain.PostingJob, titles map[uuid.UUID]string) *dto.PublishResponse {
	response := &dto.PublishResponse{
		RunID:     run.ID,
		LoadAdID:  run.LoadAdID,
		AccountID: run.AccountID,
		Queued:    run.Approved,
		Skipped:   run.Skipped,
	}
	for _, job := range jobs {
		response.Jobs = append(response.Jobs, jobSummary(job, titles[job.GroupID]))
	}
	return response
}

// runResponse rebuilds the answer for a run that already exists, which is what
// a retried request gets.
func (u *PublishUsecase) runResponse(ctx context.Context, companyID uuid.UUID, run *domain.PublishRun) (*dto.PublishResponse, error) {
	jobs, _, err := u.posting.ListJobs(ctx, companyID,
		domain.PostingJobFilter{LoadAdID: &run.LoadAdID}, 0, 200)
	if err != nil {
		return nil, err
	}

	response := &dto.PublishResponse{
		RunID:     run.ID,
		LoadAdID:  run.LoadAdID,
		AccountID: run.AccountID,
		Queued:    run.Approved,
		Skipped:   run.Skipped,
	}
	for _, job := range jobs {
		if job.RunID != run.ID {
			continue
		}
		response.Jobs = append(response.Jobs, jobSummary(job, ""))
	}
	return response, nil
}

func jobSummary(job domain.PostingJob, title string) dto.PostingJobSummary {
	summary := dto.PostingJobSummary{
		ID:         job.ID,
		GroupID:    job.GroupID,
		GroupTitle: title,
		Status:     string(job.Status),
		Attempt:    int(job.Attempt),
		SentAt:     job.SentAt,
		CreatedAt:  job.CreatedAt,
	}
	if job.SkipReason != nil {
		summary.SkipReason = string(*job.SkipReason)
	}
	if job.ErrorCode != nil {
		summary.ErrorCode = *job.ErrorCode
	}
	if !job.Status.Terminal() {
		next := job.NextAttemptAt
		summary.NextTryAt = &next
	}
	return summary
}

func (u *PublishUsecase) storePreview(ctx context.Context, p preview) (string, error) {
	token, err := randomToken()
	if err != nil {
		return "", err
	}
	if err := u.cache.Set(ctx, previewKey(token), p, previewTTL); err != nil {
		return "", fmt.Errorf("store preview: %w", err)
	}
	return token, nil
}

func (u *PublishUsecase) loadPreview(ctx context.Context, token string) (preview, error) {
	var p preview
	if token == "" {
		return p, apperror.ErrValidation.WithMessage("Review the groups before publishing.")
	}
	if err := u.cache.Get(ctx, previewKey(token), &p); err != nil {
		return p, apperror.ErrConflict.
			WithMessage("That review has expired. Check the groups again.")
	}
	return p, nil
}

func (u *PublishUsecase) recordPublish(ctx context.Context, userID uuid.UUID, run *domain.PublishRun) {
	err := u.audit.Record(ctx, domain.AuditEntry{
		ActorUserID:  &userID,
		Action:       "load_ad.publish",
		ResourceType: "publish_run",
		ResourceID:   &run.ID,
		NewValues: map[string]any{
			"load_ad_id": run.LoadAdID,
			"account_id": run.AccountID,
			"queued":     run.Approved,
			"skipped":    run.Skipped,
		},
	})
	if err != nil {
		u.log.Warn("audit write failed", logger.String("action", "load_ad.publish"), logger.Err(err))
	}
}

func previewKey(token string) string { return "publish:preview:" + token }

// previewLanguage renders the sample in the language most of the approved
// groups read, because that is the message most people will see.
func previewLanguage(ranked []matching.Ranked) string {
	counts := map[string]int{}
	for _, candidate := range ranked {
		counts[candidate.Candidate.Group.Language]++
	}
	best, bestCount := "ru", 0
	for language, count := range counts {
		if count > bestCount || (count == bestCount && language < best) {
			best, bestCount = language, count
		}
	}
	return best
}

func accountNotReady(readiness domain.PostReadiness) error {
	switch readiness.Reason {
	case "paused":
		message := "Telegram asked this account to wait."
		if readiness.Until != nil {
			message += " Try again after " + readiness.Until.Format(time.RFC3339) + "."
		}
		return apperror.ErrConflict.WithMessage(message)
	case "restricted":
		return apperror.ErrConflict.
			WithMessage("Telegram has restricted this account. Posting is stopped until it is lifted.")
	case "session_revoked":
		return apperror.ErrConflict.
			WithMessage("The Telegram session has ended. Connect the account again.")
	default:
		return apperror.ErrForbidden.
			WithMessage("Connect your Telegram account before publishing.")
	}
}

func reasonStrings(reasons []matching.Reason) []string {
	out := make([]string, 0, len(reasons))
	for _, reason := range reasons {
		out = append(out, string(reason))
	}
	return out
}

func intersect(reviewed, requested []uuid.UUID) []uuid.UUID {
	out := make([]uuid.UUID, 0, len(requested))
	for _, id := range requested {
		if slices.Contains(reviewed, id) {
			out = append(out, id)
		}
	}
	return out
}

func joinWords(words []string) string {
	switch len(words) {
	case 0:
		return ""
	case 1:
		return words[0]
	default:
		return fmt.Sprintf("%s and %s", joinComma(words[:len(words)-1]), words[len(words)-1])
	}
}

func joinComma(words []string) string {
	out := ""
	for i, word := range words {
		if i > 0 {
			out += ", "
		}
		out += word
	}
	return out
}

// randomInt63 is the value Telegram uses to refuse a resend. It is generated
// once per job and never regenerated, so a retry after a crash carries the same
// one and cannot post a second copy.
func randomInt63() (int64, error) {
	n, err := rand.Int(rand.Reader, big.NewInt(math.MaxInt64))
	if err != nil {
		return 0, fmt.Errorf("random id: %w", err)
	}
	return n.Int64(), nil
}

func randomToken() (string, error) {
	buf := make([]byte, 24)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("random token: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
}

// History lists what went where, newest first.
func (u *PublishUsecase) History(
	ctx context.Context,
	userID, companyID uuid.UUID,
	query dto.PostingHistoryQuery,
	offset, limit int,
) ([]dto.PostingJobSummary, int64, error) {
	filter := domain.PostingJobFilter{
		GroupID:  query.GroupID,
		LoadAdID: query.LoadAdID,
	}
	if query.Status != "" {
		filter.Statuses = []domain.PostingJobStatus{domain.PostingJobStatus(query.Status)}
	}
	if query.Mine {
		filter.UserID = &userID
	}

	jobs, total, err := u.posting.ListJobs(ctx, companyID, filter, offset, limit)
	if err != nil {
		return nil, 0, err
	}
	return u.summarise(ctx, companyID, jobs), total, nil
}

// JobsForAd is the per-advert view: one line per group, with what happened.
func (u *PublishUsecase) JobsForAd(ctx context.Context, companyID, adID uuid.UUID) ([]dto.PostingJobSummary, error) {
	jobs, _, err := u.posting.ListJobs(ctx, companyID,
		domain.PostingJobFilter{LoadAdID: &adID}, 0, 500)
	if err != nil {
		return nil, err
	}
	return u.summarise(ctx, companyID, jobs), nil
}

// CancelJob stops a post that has not gone out yet.
func (u *PublishUsecase) CancelJob(ctx context.Context, userID, companyID, jobID uuid.UUID) error {
	cancelled, err := u.posting.Cancel(ctx, companyID, jobID)
	if err != nil {
		return err
	}
	if !cancelled {
		// Either it is already sent or it never existed; both mean there is
		// nothing to stop, and saying which would leak whose job it is.
		return apperror.ErrConflict.
			WithMessage("That post has already left the queue.")
	}

	if err := u.audit.Record(ctx, domain.AuditEntry{
		ActorUserID:  &userID,
		Action:       "posting_job.cancel",
		ResourceType: "posting_job",
		ResourceID:   &jobID,
	}); err != nil {
		u.log.Warn("audit write failed", logger.String("action", "posting_job.cancel"), logger.Err(err))
	}
	return nil
}

// summarise attaches the group titles, because a history line naming a UUID
// tells the person nothing about where their advert went.
func (u *PublishUsecase) summarise(ctx context.Context, companyID uuid.UUID, jobs []domain.PostingJob) []dto.PostingJobSummary {
	titles := make(map[uuid.UUID]string, len(jobs))
	for _, job := range jobs {
		if _, known := titles[job.GroupID]; known {
			continue
		}
		if group, err := u.groups.FindByID(ctx, companyID, job.GroupID); err == nil {
			titles[job.GroupID] = group.Title
		}
	}

	out := make([]dto.PostingJobSummary, 0, len(jobs))
	for _, job := range jobs {
		out = append(out, jobSummary(job, titles[job.GroupID]))
	}
	return out
}

func skipReason(reason domain.SkipReason) *domain.SkipReason { return &reason }
