package usecase

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/google/uuid"

	"github.com/loadify/backend/internal/config"
	"github.com/loadify/backend/internal/domain"
	"github.com/loadify/backend/internal/dto"
	"github.com/loadify/backend/internal/infrastructure/cache"
	"github.com/loadify/backend/pkg/logger"
)

type fixture struct {
	usecase  *PublishUsecase
	ads      *fakeAdRepo
	groups   *fakeGroupRepo
	accounts *fakeAccountRepo
	posting  *fakePosting
	guard    *fakeGuard

	companyID uuid.UUID
	userID    uuid.UUID
	accountID uuid.UUID
	adID      uuid.UUID
	groupIDs  []uuid.UUID
}

func newFixture(t *testing.T, groupCount int) *fixture {
	t.Helper()

	server := miniredis.RunT(t)
	host, port, found := strings.Cut(server.Addr(), ":")
	if !found {
		t.Fatalf("unexpected miniredis address %q", server.Addr())
	}
	portNumber := 0
	for _, r := range port {
		portNumber = portNumber*10 + int(r-'0')
	}

	log := logger.New("test", "error")
	redisCache, err := cache.New(config.RedisConfig{Host: host, Port: portNumber, PoolSize: 4}, log)
	if err != nil {
		t.Fatalf("cache: %v", err)
	}
	t.Cleanup(func() { _ = redisCache.Close() })

	f := &fixture{
		companyID: uuid.New(),
		userID:    uuid.New(),
		accountID: uuid.New(),
		adID:      uuid.New(),
		ads:       &fakeAdRepo{ads: map[uuid.UUID]*domain.LoadAd{}},
		groups:    &fakeGroupRepo{},
		accounts:  &fakeAccountRepo{},
		posting:   &fakePosting{},
		guard:     &fakeGuard{refuse: map[uuid.UUID]domain.GuardDecision{}},
	}

	consent := time.Now().Add(-24 * time.Hour)
	f.accounts.accounts = []domain.TelegramAccount{{
		ID: f.accountID, CompanyID: f.companyID, UserID: f.userID,
		Phone: "+998901234567", Status: domain.TelegramAccountActive,
		SessionCiphertext: []byte("sealed"), ConsentAt: &consent,
	}}

	ad := fullAd()
	ad.ID = f.adID
	ad.CompanyID = f.companyID
	ad.OriginCountry, ad.DestCountry = pointer("UZ"), pointer("RU")
	ad.Fingerprint = ad.ComputeFingerprint()
	f.ads.ads[f.adID] = ad

	for i := range groupCount {
		id := uuid.New()
		f.groupIDs = append(f.groupIDs, id)
		f.groups.candidates = append(f.groups.candidates, domain.GroupCandidate{
			Group: domain.TelegramGroup{
				ID: id, CompanyID: f.companyID, AccountID: f.accountID,
				TgPeerID: int64(-100100 - i), Title: string(rune('A'+i)) + " guruh",
				Language: "ru", CanSend: true, PostingEnabled: true, Active: true,
			},
			Rules: domain.TelegramGroupRules{
				GroupID: id, CompanyID: f.companyID,
				HourlyLimit: 2, DailyLimit: 3, CooldownMinutes: 60,
			},
			Routes: []domain.TelegramGroupRoute{{
				GroupID: id, OriginCountry: pointer("UZ"), DestCountry: pointer("RU"),
			}},
		})
	}

	f.usecase = NewPublishUsecase(
		f.ads, f.accounts, f.groups, fakeCargoRepo{}, f.posting, f.guard, nil,
		redisCache, config.TelegramConfig{GroupCooldown: time.Hour, GroupDaily: 3},
		noopAudit{}, log,
	)
	return f
}

func (f *fixture) preview(t *testing.T) *dto.PublishPreviewResponse {
	t.Helper()
	res, err := f.usecase.Preview(context.Background(), f.userID, f.companyID, f.adID, nil)
	if err != nil {
		t.Fatalf("Preview: %v", err)
	}
	return res
}

// The funnel is the product: this many groups matched, this many are approved,
// and every one held back says why.
func TestPreviewShowsTheWholeFunnel(t *testing.T) {
	f := newFixture(t, 4)
	f.guard.refuse[f.groupIDs[0]] = domain.GuardDecision{
		Reason: domain.SkipCooldown, RetryAfter: 40 * time.Minute,
	}
	f.guard.refuse[f.groupIDs[1]] = domain.GuardDecision{Reason: domain.SkipDuplicate}

	res := f.preview(t)

	if res.Candidates != 4 {
		t.Errorf("candidates = %d, want 4", res.Candidates)
	}
	if len(res.Approved) != 2 || len(res.Skipped) != 2 {
		t.Fatalf("approved %d, skipped %d, want 2 and 2", len(res.Approved), len(res.Skipped))
	}
	for _, skipped := range res.Skipped {
		if skipped.Reason == "" {
			t.Errorf("group %s was held back without a reason", skipped.Title)
		}
	}
	var sawWait bool
	for _, skipped := range res.Skipped {
		if skipped.Reason == string(domain.SkipCooldown) && skipped.RetryAfterSeconds > 0 {
			sawWait = true
		}
	}
	if !sawWait {
		t.Error("a cooldown is a wait and should say how long")
	}
	if !strings.Contains(res.MessagePreview, "Москва") {
		t.Errorf("the message preview does not look like the advert:\n%s", res.MessagePreview)
	}
	if res.PreviewToken == "" || res.ExpiresAt.IsZero() {
		t.Error("a preview must be publishable and must expire")
	}
}

func TestPublishQueuesTheReviewedGroups(t *testing.T) {
	f := newFixture(t, 3)
	preview := f.preview(t)

	res, err := f.usecase.Publish(context.Background(), f.userID, f.companyID, f.adID, "key-1",
		dto.PublishRequest{PreviewToken: preview.PreviewToken})
	if err != nil {
		t.Fatalf("Publish: %v", err)
	}

	if res.Queued != 3 || len(res.Jobs) != 3 {
		t.Errorf("queued = %d with %d jobs, want 3", res.Queued, len(res.Jobs))
	}
	for _, job := range res.Jobs {
		if job.Status != string(domain.PostingPending) {
			t.Errorf("job %s is %q, want pending", job.ID, job.Status)
		}
	}
	if len(f.posting.jobs) != 3 {
		t.Errorf("%d jobs stored, want 3", len(f.posting.jobs))
	}

	// Every job carries its own random id, which is what Telegram refuses a
	// second time; sharing one would make two groups one message.
	seen := map[int64]bool{}
	for _, job := range f.posting.jobs {
		if job.RandomID == 0 || seen[job.RandomID] {
			t.Errorf("job %s has a reused or empty random id", job.ID)
		}
		seen[job.RandomID] = true
		if job.MessageText == "" {
			t.Errorf("job %s carries no message", job.ID)
		}
	}
	if f.ads.ads[f.adID].Status != domain.LoadAdPublishing {
		t.Errorf("advert status = %q, want publishing", f.ads.ads[f.adID].Status)
	}
}

// A retried request must not post the advert twice.
func TestPublishIsIdempotent(t *testing.T) {
	f := newFixture(t, 2)
	preview := f.preview(t)

	first, err := f.usecase.Publish(context.Background(), f.userID, f.companyID, f.adID, "key-1",
		dto.PublishRequest{PreviewToken: preview.PreviewToken})
	if err != nil {
		t.Fatalf("Publish: %v", err)
	}
	second, err := f.usecase.Publish(context.Background(), f.userID, f.companyID, f.adID, "key-1",
		dto.PublishRequest{PreviewToken: preview.PreviewToken})
	if err != nil {
		t.Fatalf("second Publish: %v", err)
	}

	if first.RunID != second.RunID {
		t.Errorf("run ids %s and %s differ, so the retry started a second run", first.RunID, second.RunID)
	}
	if len(f.posting.jobs) != 2 {
		t.Errorf("%d jobs stored after a retry, want 2", len(f.posting.jobs))
	}
}

// The token ties a publish to the list that was reviewed. Naming other groups
// narrows that list and never extends it.
func TestPublishOnlyNarrowsTheReviewedList(t *testing.T) {
	f := newFixture(t, 3)
	preview := f.preview(t)
	stranger := uuid.New()

	res, err := f.usecase.Publish(context.Background(), f.userID, f.companyID, f.adID, "key-1",
		dto.PublishRequest{
			PreviewToken: preview.PreviewToken,
			GroupIDs:     []uuid.UUID{f.groupIDs[0], stranger},
		})
	if err != nil {
		t.Fatalf("Publish: %v", err)
	}

	if len(res.Jobs) != 1 || res.Jobs[0].GroupID != f.groupIDs[0] {
		t.Errorf("jobs = %+v, want only the reviewed group", res.Jobs)
	}
}

// Editing the advert after reviewing it changes what would be sent, so the
// review no longer describes it.
func TestAnEditedAdvertInvalidatesTheReview(t *testing.T) {
	f := newFixture(t, 2)
	preview := f.preview(t)

	ad := f.ads.ads[f.adID]
	ad.PriceAmount = pointer(int64(300000))
	ad.Fingerprint = ad.ComputeFingerprint()

	_, err := f.usecase.Publish(context.Background(), f.userID, f.companyID, f.adID, "key-1",
		dto.PublishRequest{PreviewToken: preview.PreviewToken})
	if err == nil {
		t.Fatal("publishing a changed advert from an old review was allowed")
	}
	if !strings.Contains(err.Error(), "changed") {
		t.Errorf("the error does not explain what happened: %v", err)
	}
}

func TestAPreviewBelongsToThePersonWhoMadeIt(t *testing.T) {
	f := newFixture(t, 2)
	preview := f.preview(t)

	_, err := f.usecase.Publish(context.Background(), uuid.New(), f.companyID, f.adID, "key-1",
		dto.PublishRequest{PreviewToken: preview.PreviewToken})
	if err == nil {
		t.Fatal("another person published from someone else's review")
	}
}

func TestPublishNeedsAnIdempotencyKey(t *testing.T) {
	f := newFixture(t, 1)
	preview := f.preview(t)

	_, err := f.usecase.Publish(context.Background(), f.userID, f.companyID, f.adID, "",
		dto.PublishRequest{PreviewToken: preview.PreviewToken})
	if err == nil {
		t.Fatal("a publish without an idempotency key was accepted")
	}
}

// A group that stopped being postable between the review and the press is
// recorded as skipped rather than dropped, so the run still adds up.
func TestAGroupThatClosedInBetweenIsRecordedAsSkipped(t *testing.T) {
	f := newFixture(t, 2)
	preview := f.preview(t)
	f.groups.candidates[0].Group.PostingEnabled = false

	res, err := f.usecase.Publish(context.Background(), f.userID, f.companyID, f.adID, "key-1",
		dto.PublishRequest{PreviewToken: preview.PreviewToken})
	if err != nil {
		t.Fatalf("Publish: %v", err)
	}

	if res.Queued != 1 || res.Skipped != 1 {
		t.Errorf("queued %d skipped %d, want 1 and 1", res.Queued, res.Skipped)
	}
	var found bool
	for _, job := range res.Jobs {
		if job.Status == string(domain.PostingSkipped) && job.SkipReason == string(domain.SkipGroupUnavailable) {
			found = true
		}
	}
	if !found {
		t.Errorf("no job explains the group that closed: %+v", res.Jobs)
	}
}

func TestARestrictedAccountCannotPublish(t *testing.T) {
	f := newFixture(t, 2)
	f.accounts.accounts[0].Status = domain.TelegramAccountRestricted

	_, err := f.usecase.Preview(context.Background(), f.userID, f.companyID, f.adID, nil)
	if err == nil {
		t.Fatal("a restricted account was allowed to preview a publish")
	}
	if !strings.Contains(strings.ToLower(err.Error()), "restrict") {
		t.Errorf("the error does not say why: %v", err)
	}
}

// An advert missing a phone number is not something to send to a hundred
// groups, and the message says exactly what is missing.
func TestAnIncompleteAdvertIsRefusedWithItsGaps(t *testing.T) {
	f := newFixture(t, 2)
	f.ads.ads[f.adID].ContactPhone = nil

	_, err := f.usecase.Preview(context.Background(), f.userID, f.companyID, f.adID, nil)
	if err == nil {
		t.Fatal("an advert with no contact was accepted for publishing")
	}
	if !strings.Contains(err.Error(), "contact_phone") {
		t.Errorf("the error does not name the missing field: %v", err)
	}
}

// ─── plan limits ────────────────────────────────────────────────────

func TestThePreviewSaysHowManyPostsThePlanHasLeft(t *testing.T) {
	f := newFixture(t, 3)
	quota := newQuotaStub(map[domain.QuotaMetric]int{domain.QuotaPostsPerMonth: 10})
	quota.used[domain.QuotaPostsPerMonth] = 8
	f.usecase.quota = quota

	preview := f.preview(t)
	if preview.PostsLeft == nil || *preview.PostsLeft != 2 {
		t.Errorf("posts_left = %v, want 2", preview.PostsLeft)
	}

	f.usecase.quota = newQuotaStub(nil)
	if unlimited := f.preview(t); unlimited.PostsLeft != nil {
		t.Errorf("posts_left = %d on a plan with no limit, want null", *unlimited.PostsLeft)
	}
}

func TestPublishingMoreThanThePlanHasLeftIsRefusedUntilNarrowed(t *testing.T) {
	f := newFixture(t, 3)
	quota := newQuotaStub(map[domain.QuotaMetric]int{domain.QuotaPostsPerMonth: 10})
	quota.used[domain.QuotaPostsPerMonth] = 8
	f.usecase.quota = quota
	preview := f.preview(t)

	_, err := f.usecase.Publish(context.Background(), f.userID, f.companyID, f.adID, "key-1",
		dto.PublishRequest{PreviewToken: preview.PreviewToken})
	wantCode(t, err, "QUOTA_EXCEEDED")
	if len(f.posting.jobs) != 0 {
		t.Fatalf("%d jobs were queued past the plan", len(f.posting.jobs))
	}

	// The review is still valid: the person picks two groups and goes on.
	res, err := f.usecase.Publish(context.Background(), f.userID, f.companyID, f.adID, "key-2",
		dto.PublishRequest{PreviewToken: preview.PreviewToken, GroupIDs: f.groupIDs[:2]})
	if err != nil {
		t.Fatalf("Publish narrowed: %v", err)
	}
	if res.Queued != 2 || quota.used[domain.QuotaPostsPerMonth] != 10 {
		t.Errorf("queued %d, used %d; want 2 and 10", res.Queued, quota.used[domain.QuotaPostsPerMonth])
	}
}

func TestAGroupTheGuardSkippedCostsNothing(t *testing.T) {
	f := newFixture(t, 2)
	quota := newQuotaStub(map[domain.QuotaMetric]int{domain.QuotaPostsPerMonth: 1})
	f.usecase.quota = quota
	preview := f.preview(t)
	f.groups.candidates[0].Group.PostingEnabled = false

	res, err := f.usecase.Publish(context.Background(), f.userID, f.companyID, f.adID, "key-1",
		dto.PublishRequest{PreviewToken: preview.PreviewToken})
	if err != nil {
		t.Fatalf("Publish: %v; one post fits a plan of one", err)
	}
	if res.Queued != 1 || res.Skipped != 1 {
		t.Errorf("queued %d skipped %d, want 1 and 1", res.Queued, res.Skipped)
	}
	if quota.used[domain.QuotaPostsPerMonth] != 1 {
		t.Errorf("used = %d, want only the post that will be attempted", quota.used[domain.QuotaPostsPerMonth])
	}
}

// Publishing an advert again only offers the groups it is not already waiting
// in; before, the second publish collided with its own queue and sent nothing.
func TestARepublishSkipsGroupsTheAdvertIsAlreadyQueuedIn(t *testing.T) {
	f := newFixture(t, 2)
	first := f.preview(t)
	if _, err := f.usecase.Publish(context.Background(), f.userID, f.companyID, f.adID, "key-1",
		dto.PublishRequest{PreviewToken: first.PreviewToken, GroupIDs: f.groupIDs[:1]}); err != nil {
		t.Fatalf("first Publish: %v", err)
	}

	again := f.preview(t)
	if len(again.Approved) != 1 || again.Approved[0].GroupID != f.groupIDs[1] {
		t.Fatalf("approved = %+v, want only the group the advert is not queued in", again.Approved)
	}
	var reason string
	for _, s := range again.Skipped {
		if s.GroupID == f.groupIDs[0] {
			reason = s.Reason
		}
	}
	if reason != string(domain.SkipAlreadyQueued) {
		t.Errorf("queued group reason = %q, want already_queued", reason)
	}

	res, err := f.usecase.Publish(context.Background(), f.userID, f.companyID, f.adID, "key-2",
		dto.PublishRequest{PreviewToken: again.PreviewToken})
	if err != nil {
		t.Fatalf("second Publish: %v", err)
	}
	if res.Queued != 1 {
		t.Errorf("queued %d, want the one new group", res.Queued)
	}
}

// A group queued by another request between the review and the publish is
// written down as skipped instead of failing the whole run.
func TestAGroupQueuedInBetweenIsSkippedNotACollision(t *testing.T) {
	f := newFixture(t, 2)
	mine := f.preview(t)
	theirs := f.preview(t)
	if _, err := f.usecase.Publish(context.Background(), f.userID, f.companyID, f.adID, "theirs",
		dto.PublishRequest{PreviewToken: theirs.PreviewToken, GroupIDs: f.groupIDs[:1]}); err != nil {
		t.Fatalf("other Publish: %v", err)
	}

	res, err := f.usecase.Publish(context.Background(), f.userID, f.companyID, f.adID, "mine",
		dto.PublishRequest{PreviewToken: mine.PreviewToken})
	if err != nil {
		t.Fatalf("Publish: %v", err)
	}
	if res.Queued != 1 || res.Skipped != 1 {
		t.Errorf("queued %d skipped %d, want 1 and 1", res.Queued, res.Skipped)
	}
}
