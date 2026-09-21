// Package worker sends queued posts, one job at a time, and writes down exactly
// what happened to each.
//
// Every branch here answers one question: after this, is it safe to try again?
// Telegram asking for a wait is a wait for the whole account. Telegram calling
// the account a spammer stops everything from it. A refusal from a group closes
// that group. A network failure is retried — with the same random_id, so a
// message that did go out is not sent twice. And a job whose worker died in the
// middle of a send is not sent again at all until Telegram's handling of that
// random_id has been confirmed, because a missed post is recoverable and a
// duplicate is spam.
package worker

import (
	"context"
	"errors"
	"math/rand/v2"
	"time"

	"github.com/google/uuid"

	"github.com/loadify/backend/internal/config"
	"github.com/loadify/backend/internal/domain"
	"github.com/loadify/backend/pkg/apperror"
	"github.com/loadify/backend/pkg/logger"
	"github.com/loadify/backend/pkg/secret"
)

// guardUnavailableWait is how long a job waits when the guard itself cannot be
// reached. The guard failing closed is the point: without the limits, nothing
// goes out.
const guardUnavailableWait = 30 * time.Second

// Sender processes posting jobs.
type Sender struct {
	posting  domain.PostingRepository
	accounts domain.TelegramAccountRepository
	groups   domain.TelegramGroupRepository
	ads      domain.LoadAdRepository
	guard    domain.PostingGuard
	telegram domain.TelegramSender
	vault    domain.SessionVault
	alerts   domain.DistributionAlerter
	limits   config.TelegramConfig
	owner    string
	log      logger.Logger
	metrics  Metrics

	now    func() time.Time
	jitter func(time.Duration) time.Duration
}

// Metrics is what the sender reports as it works. The values are closed sets
// (a status, a guard reason, a Telegram error kind), never ids or messages.
type Metrics interface {
	JobFinished(status domain.PostingJobStatus)
	GuardRefused(reason domain.SkipReason)
	// TelegramSend times one send; kind is empty when Telegram accepted it.
	TelegramSend(took time.Duration, kind domain.TelegramErrorKind)
	Alerted(kind domain.DistributionAlertKind)
}

type nopMetrics struct{}

func (nopMetrics) JobFinished(domain.PostingJobStatus)                  {}
func (nopMetrics) GuardRefused(domain.SkipReason)                       {}
func (nopMetrics) TelegramSend(time.Duration, domain.TelegramErrorKind) {}
func (nopMetrics) Alerted(domain.DistributionAlertKind)                 {}

// New builds a sender. owner names this worker in the lease, so a stuck job
// can be traced to the process that took it. alerts may be nil, in which case
// problems are only recorded, not reported to anyone.
func New(
	posting domain.PostingRepository,
	accounts domain.TelegramAccountRepository,
	groups domain.TelegramGroupRepository,
	ads domain.LoadAdRepository,
	guard domain.PostingGuard,
	telegram domain.TelegramSender,
	vault domain.SessionVault,
	alerts domain.DistributionAlerter,
	limits config.TelegramConfig,
	owner string,
	log logger.Logger,
) *Sender {
	return &Sender{
		posting: posting, accounts: accounts, groups: groups, ads: ads,
		guard: guard, telegram: telegram, vault: vault, alerts: alerts, limits: limits,
		owner: owner, log: log, metrics: nopMetrics{}, now: time.Now, jitter: spread,
	}
}

// WithMetrics makes the sender report what it does to m.
func (s *Sender) WithMetrics(m Metrics) *Sender {
	if m != nil {
		s.metrics = m
	}
	return s
}

// spread adds up to a fifth again to a wait, so a Telegram hiccup does not
// bring every waiting job back in the same second.
func spread(d time.Duration) time.Duration {
	if d <= 0 {
		return d
	}
	return d + time.Duration(rand.Int64N(int64(d)/5+1))
}

// Process takes one job through to an outcome.
//
// A job someone else already holds is not an error: the queue can deliver the
// same id twice, and the claim is what decides who works on it.
func (s *Sender) Process(ctx context.Context, jobID uuid.UUID) error {
	job, err := s.posting.Claim(ctx, jobID, s.owner, s.limits.LeaseTTL)
	if err != nil {
		return err
	}
	if job == nil {
		return nil
	}

	outcome := s.handle(ctx, job)
	if err := s.posting.Finish(ctx, job.ID, outcome); err != nil {
		// The message may already be in the group. The lease will expire and
		// the job will be claimed again, where the previous-status check keeps
		// it from being sent twice.
		s.log.Error("posting outcome not recorded",
			logger.String("job_id", job.ID.String()),
			logger.String("status", string(outcome.Status)),
			logger.Err(err))
		return err
	}
	s.metrics.JobFinished(outcome.Status)
	return nil
}

func (s *Sender) handle(ctx context.Context, job *domain.PostingJob) domain.JobOutcome {
	now := s.now()

	if job.PreviousStatus == domain.PostingProcessing && !s.limits.TrustRandomID {
		return failed("outcome_unknown",
			"The worker stopped while sending this post. It is not sent again automatically, "+
				"because a second copy in the group would be spam. Publish again if it did not arrive.")
	}

	account, err := s.accounts.FindByID(ctx, job.CompanyID, job.AccountID)
	if err != nil {
		if apperror.Is(err, apperror.ErrNotFound.Code) {
			return cancelled("account_gone", "The Telegram account was disconnected.")
		}
		return s.retryLater(job, now, err)
	}

	if readiness := account.ReadyToPost(now); !readiness.Ready {
		if readiness.Reason == "paused" && readiness.Until != nil {
			return rateLimited(*readiness.Until, "flood_wait", "Telegram asked this account to wait.")
		}
		return cancelled(readiness.Reason, "The Telegram account cannot post right now.")
	}

	if until, paused, err := s.guard.AccountPausedUntil(ctx, account.ID); err != nil {
		return rateLimited(now.Add(guardUnavailableWait), "guard_unavailable",
			"The limits could not be checked, so nothing was sent.")
	} else if paused {
		return rateLimited(until, "flood_wait", "Telegram asked this account to wait.")
	}

	group, err := s.groups.FindByID(ctx, job.CompanyID, job.GroupID)
	if err != nil {
		if apperror.Is(err, apperror.ErrNotFound.Code) {
			return skipped(domain.SkipGroupUnavailable)
		}
		return s.retryLater(job, now, err)
	}
	if !group.Postable() {
		return skipped(domain.SkipGroupUnavailable)
	}

	target := s.target(ctx, job, account, group)
	decision, err := s.guard.Reserve(ctx, target, now)
	if err != nil {
		return rateLimited(now.Add(guardUnavailableWait), "guard_unavailable",
			"The limits could not be checked, so nothing was sent.")
	}
	if !decision.Allowed {
		s.metrics.GuardRefused(decision.Reason)
		s.recordSpam(ctx, job, string(decision.Reason), decision.RetryAfter)
		if decision.Reason == domain.SkipDuplicate {
			return skipped(domain.SkipDuplicate)
		}
		next := now.Add(s.jitter(decision.RetryAfter))
		if s.stale(ctx, job, next) {
			return skipped(domain.SkipStale)
		}
		return rateLimited(next, string(decision.Reason), "")
	}

	session, err := s.openSession(account)
	if err != nil {
		s.release(ctx, target)
		s.log.Error("telegram session could not be opened",
			logger.String("account_id", account.ID.String()), logger.Err(err))
		return failed("session_unreadable",
			"The stored Telegram session could not be read. Connect the account again.")
	}
	defer wipe(session)

	started := time.Now()
	result, err := s.telegram.Send(ctx, domain.SendRequest{
		Session:    session,
		PeerID:     group.TgPeerID,
		AccessHash: group.AccessHash,
		RandomID:   job.RandomID,
		Text:       job.MessageText,
	})
	s.metrics.TelegramSend(time.Since(started), telegramKind(err))
	if err == nil {
		return s.sent(ctx, account, group, result, now)
	}
	return s.failure(ctx, job, account, group, target, err, now)
}

// failure turns a Telegram refusal into the right consequence for the job, the
// account and the group.
func (s *Sender) failure(
	ctx context.Context,
	job *domain.PostingJob,
	account *domain.TelegramAccount,
	group *domain.TelegramGroup,
	target domain.GuardTarget,
	err error,
	now time.Time,
) domain.JobOutcome {
	tgErr := asTelegramError(err)
	code := tgErr.Code
	if code == "" {
		code = string(tgErr.Kind)
	}

	switch tgErr.Kind {
	case domain.TelegramDuplicate:
		// Telegram has already seen this random_id: the message went out on an
		// earlier attempt whose outcome we never recorded.
		return s.sent(ctx, account, group, domain.SendResult{}, now)

	case domain.TelegramFloodWait:
		// A wait for the account, not for this message: everything from this
		// account holds until it passes.
		until := now.Add(tgErr.Wait)
		reason := "Telegram asked this account to wait."
		if err := s.guard.PauseAccount(ctx, account.ID, until); err != nil {
			s.log.Warn("account pause not recorded in the guard", logger.Err(err))
		}
		if err := s.accounts.SetStatus(ctx, account.ID, domain.TelegramAccountPaused, &reason, &until); err != nil {
			s.log.Warn("account pause not recorded", logger.Err(err))
		}
		s.release(ctx, target)
		s.recordSpam(ctx, job, "flood_wait", tgErr.Wait)
		s.alert(ctx, account, nil, domain.DistributionAlert{Kind: domain.AlertAccountPaused, Until: &until})
		return rateLimitedCode(until, code, reason)

	case domain.TelegramSlowmode:
		// A wait for this group only.
		s.release(ctx, target)
		s.recordSpam(ctx, job, "slowmode", tgErr.Wait)
		return rateLimitedCode(now.Add(s.jitter(tgErr.Wait)), code, "The group has slow mode on.")

	case domain.TelegramPeerFlood:
		// Telegram is treating the account as a spammer. Nothing from it is
		// retried, and nothing queued from it goes out.
		reason := "Telegram has limited this account from writing in groups. Posting is stopped. " +
			"Write to @SpamBot in Telegram to see why and when it lifts."
		if err := s.accounts.SetStatus(ctx, account.ID, domain.TelegramAccountRestricted, &reason, nil); err != nil {
			s.log.Error("account restriction not recorded", logger.Err(err))
		}
		stopped, cancelErr := s.posting.CancelForAccount(ctx, account.ID, reason)
		if cancelErr != nil {
			s.log.Error("queued posts not cancelled after a restriction", logger.Err(cancelErr))
		}
		s.recordSpam(ctx, job, "peer_flood", 0)
		s.log.Error("telegram restricted an account",
			logger.String("account_id", account.ID.String()),
			logger.String("company_id", account.CompanyID.String()))
		s.alert(ctx, account, nil, domain.DistributionAlert{Kind: domain.AlertAccountRestricted, Cancelled: stopped})
		return failedCode(code, reason)

	case domain.TelegramWriteForbidden:
		reason := "This account is not allowed to write in the group."
		if err := s.groups.DisablePosting(ctx, group.CompanyID, group.ID, reason); err != nil {
			s.log.Warn("group not disabled after a refusal", logger.Err(err))
		}
		s.release(ctx, target)
		s.recordSpam(ctx, job, "write_forbidden", 0)
		s.alert(ctx, account, group, domain.DistributionAlert{Kind: domain.AlertGroupClosed})
		return failedCode(code, reason)

	case domain.TelegramGroupGone:
		reason := "The account is no longer in this group."
		if err := s.groups.Deactivate(ctx, group.CompanyID, group.ID, reason); err != nil {
			s.log.Warn("group not deactivated", logger.Err(err))
		}
		s.release(ctx, target)
		s.alert(ctx, account, group, domain.DistributionAlert{Kind: domain.AlertGroupLost})
		return failedCode(code, reason)

	case domain.TelegramSessionRevoked:
		reason := "The Telegram session has ended. Connect the account again."
		if err := s.accounts.SetStatus(ctx, account.ID, domain.TelegramAccountRevoked, &reason, nil); err != nil {
			s.log.Error("session revocation not recorded", logger.Err(err))
		}
		stopped, cancelErr := s.posting.CancelForAccount(ctx, account.ID, reason)
		if cancelErr != nil {
			s.log.Error("queued posts not cancelled after a revocation", logger.Err(cancelErr))
		}
		s.release(ctx, target)
		s.alert(ctx, account, nil, domain.DistributionAlert{Kind: domain.AlertAccountRevoked, Cancelled: stopped})
		return cancelledCode(code, reason)

	case domain.TelegramMessageTooLong:
		s.release(ctx, target)
		return failedCode(code, "The message is longer than Telegram allows.")

	case domain.TelegramTransient:
		// Not released: the message may have gone out, and counting a post
		// that was sent is the safer mistake.
		if int(job.Attempt) >= s.limits.MaxAttempts {
			return failedCode(code, "Telegram could not be reached after several attempts.")
		}
		next := now.Add(s.jitter(domain.Backoff(job.Attempt)))
		if s.stale(ctx, job, next) {
			return skipped(domain.SkipStale)
		}
		return pendingCode(next, code)

	default:
		// Sending again on a failure we do not understand is how duplicates
		// and bans happen, so an unknown error ends the job.
		s.log.Error("unclassified telegram failure",
			logger.String("job_id", job.ID.String()), logger.Err(err))
		s.alert(ctx, account, group, domain.DistributionAlert{Kind: domain.AlertTelegramUnknown, ErrorCode: code})
		return failedCode(code, "Telegram refused the post for a reason we do not recognise.")
	}
}

// asTelegramError reads a send failure. A failure we cannot classify is a
// transport problem until proven otherwise; the same random_id keeps a retry
// from duplicating.
func asTelegramError(err error) *domain.TelegramError {
	var tgErr *domain.TelegramError
	if !errors.As(err, &tgErr) {
		tgErr = &domain.TelegramError{Kind: domain.TelegramTransient, Err: err}
	}
	return tgErr
}

// telegramKind names a send's failure for metrics; empty when it succeeded.
func telegramKind(err error) domain.TelegramErrorKind {
	if err == nil {
		return ""
	}
	return asTelegramError(err).Kind
}

func (s *Sender) sent(
	ctx context.Context,
	account *domain.TelegramAccount,
	group *domain.TelegramGroup,
	result domain.SendResult,
	now time.Time,
) domain.JobOutcome {
	if err := s.groups.MarkPosted(ctx, group.ID, now); err != nil {
		s.log.Warn("group last-posted time not updated", logger.Err(err))
	}
	if err := s.accounts.TouchUsed(ctx, account.ID, now); err != nil {
		s.log.Warn("account last-used time not updated", logger.Err(err))
	}

	outcome := domain.JobOutcome{Status: domain.PostingSent, Response: result.Raw}
	if result.MessageID != 0 {
		id := result.MessageID
		outcome.TgMessageID = &id
	}
	return outcome
}

// target builds what the guard judges. The group's own rules are read at send
// time rather than copied into the job, because a person who tightens a limit
// expects the tighter limit to apply to what is still queued.
func (s *Sender) target(
	ctx context.Context,
	job *domain.PostingJob,
	account *domain.TelegramAccount,
	group *domain.TelegramGroup,
) domain.GuardTarget {
	rules := domain.TelegramGroupRules{HourlyLimit: 2, DailyLimit: 3, CooldownMinutes: 60}
	if stored, _, err := s.groups.Rules(ctx, job.CompanyID, group.ID); err == nil && stored != nil {
		rules = *stored
	}
	return domain.GuardTarget{
		CompanyID:   job.CompanyID,
		UserID:      account.UserID,
		AccountID:   account.ID,
		GroupID:     group.ID,
		PeerID:      group.TgPeerID,
		Fingerprint: job.Fingerprint,
		Cooldown:    rules.Cooldown(group.SlowmodeSeconds, s.limits.GroupCooldown),
		GroupHourly: int(rules.HourlyLimit),
		GroupDaily:  int(rules.DailyLimit),
	}
}

// stale reports whether a retry would land after the load has already left.
// Posting an advert for yesterday's loading is noise, however politely spaced.
func (s *Sender) stale(ctx context.Context, job *domain.PostingJob, next time.Time) bool {
	ad, err := s.ads.FindByID(ctx, job.CompanyID, job.LoadAdID)
	if err != nil || ad.LoadingDate == nil {
		return false
	}
	lastUseful := ad.LoadingDate.Add(24 * time.Hour)
	return next.After(lastUseful)
}

func (s *Sender) openSession(account *domain.TelegramAccount) ([]byte, error) {
	if len(account.SessionCiphertext) == 0 || account.SessionKeyVersion == nil {
		return nil, errors.New("the account has no stored session")
	}
	return s.vault.Open(account.SessionCiphertext, int(*account.SessionKeyVersion), account.SessionAAD())
}

func (s *Sender) release(ctx context.Context, target domain.GuardTarget) {
	if err := s.guard.Release(ctx, target); err != nil {
		s.log.Warn("guard reservation not released", logger.Err(err))
	}
}

func (s *Sender) retryLater(job *domain.PostingJob, now time.Time, cause error) domain.JobOutcome {
	s.log.Warn("posting job deferred",
		logger.String("job_id", job.ID.String()), logger.Err(cause))
	if int(job.Attempt) >= s.limits.MaxAttempts {
		return failed("unavailable", "The post could not be prepared after several attempts.")
	}
	return pendingCode(now.Add(s.jitter(domain.Backoff(job.Attempt))), "unavailable")
}

// alert tells the people who can act, filling in whose account and which
// group. The account is named by its label, or its masked number when it has
// none; the number itself never leaves.
func (s *Sender) alert(
	ctx context.Context,
	account *domain.TelegramAccount,
	group *domain.TelegramGroup,
	alert domain.DistributionAlert,
) {
	if s.alerts == nil {
		return
	}
	s.metrics.Alerted(alert.Kind)
	alert.CompanyID = account.CompanyID
	alert.AccountID = account.ID
	alert.OwnerID = account.UserID
	alert.AccountLabel = account.Label
	if alert.AccountLabel == "" {
		alert.AccountLabel = secret.MaskPhone(account.Phone)
	}
	if group != nil {
		id := group.ID
		alert.GroupID = &id
		alert.GroupTitle = group.Title
	}
	s.alerts.Alert(ctx, alert)
}

func (s *Sender) recordSpam(ctx context.Context, job *domain.PostingJob, kind string, wait time.Duration) {
	switch kind {
	case "duplicate", "cooldown", "group_limit", "account_limit", "flood_wait", "slowmode",
		"peer_flood", "write_forbidden", "stale":
	default:
		return
	}

	event := &domain.SpamEvent{
		CompanyID: job.CompanyID,
		AccountID: &job.AccountID,
		GroupID:   &job.GroupID,
		JobID:     &job.ID,
		Kind:      kind,
	}
	if wait > 0 {
		seconds := int(wait.Seconds())
		event.WaitSeconds = &seconds
	}
	if err := s.posting.RecordSpamEvent(ctx, event); err != nil {
		s.log.Warn("spam event not recorded", logger.String("kind", kind), logger.Err(err))
	}
}

// wipe clears the opened session as soon as the send is done. It shortens the
// window rather than closing it — the runtime may have copied the slice.
func wipe(b []byte) {
	for i := range b {
		b[i] = 0
	}
}

// ─── outcomes ───────────────────────────────────────────────────────

func skipped(reason domain.SkipReason) domain.JobOutcome {
	r := reason
	return domain.JobOutcome{Status: domain.PostingSkipped, SkipReason: &r}
}

func rateLimited(next time.Time, code, detail string) domain.JobOutcome {
	return rateLimitedCode(next, code, detail)
}

func rateLimitedCode(next time.Time, code, detail string) domain.JobOutcome {
	return domain.JobOutcome{
		Status:        domain.PostingRateLimited,
		NextAttemptAt: &next,
		ErrorCode:     optional(code),
		Detail:        optional(detail),
	}
}

func pendingCode(next time.Time, code string) domain.JobOutcome {
	return domain.JobOutcome{
		Status:        domain.PostingPending,
		NextAttemptAt: &next,
		ErrorCode:     optional(code),
	}
}

func failed(code, detail string) domain.JobOutcome {
	return failedCode(code, detail)
}

func failedCode(code, detail string) domain.JobOutcome {
	return domain.JobOutcome{
		Status:    domain.PostingFailed,
		ErrorCode: optional(code),
		Detail:    optional(detail),
	}
}

func cancelled(code, detail string) domain.JobOutcome {
	return cancelledCode(code, detail)
}

func cancelledCode(code, detail string) domain.JobOutcome {
	return domain.JobOutcome{
		Status:    domain.PostingCancelled,
		ErrorCode: optional(code),
		Detail:    optional(detail),
	}
}

func optional(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}
