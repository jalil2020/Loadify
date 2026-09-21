package worker

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/loadify/backend/internal/config"
	"github.com/loadify/backend/internal/domain"
	"github.com/loadify/backend/pkg/logger"
)

var clock = time.Date(2026, 9, 16, 9, 0, 0, 0, time.UTC)

type harness struct {
	sender   *Sender
	posting  *postingStore
	accounts *accountStore
	groups   *groupStore
	ads      *adStore
	guard    *guardStub
	telegram *telegramStub
	alerts   *alertBook
	job      *domain.PostingJob
}

func newHarness(t *testing.T) *harness {
	t.Helper()

	companyID := uuid.New()
	consent := clock.Add(-24 * time.Hour)
	version := int16(1)
	accessHash := int64(987654321)
	loading := time.Date(2026, 9, 20, 0, 0, 0, 0, time.UTC)

	account := &domain.TelegramAccount{
		ID: uuid.New(), CompanyID: companyID, UserID: uuid.New(), Phone: "+998901234567",
		Status: domain.TelegramAccountActive, ConsentAt: &consent,
		SessionCiphertext: []byte("sealed"), SessionKeyVersion: &version,
	}
	group := &domain.TelegramGroup{
		ID: uuid.New(), CompanyID: companyID, AccountID: account.ID,
		TgPeerID: -1001234567890, AccessHash: &accessHash, Language: "ru",
		Title: "Yuk markazi", CanSend: true, PostingEnabled: true, Active: true,
	}
	ad := &domain.LoadAd{ID: uuid.New(), CompanyID: companyID, LoadingDate: &loading}
	job := &domain.PostingJob{
		ID: uuid.New(), CompanyID: companyID, AccountID: account.ID, GroupID: group.ID,
		LoadAdID: ad.ID, Fingerprint: "fp", MessageText: "Ташкент → Москва",
		RandomID: 424242, Attempt: 1, Status: domain.PostingProcessing,
		PreviousStatus: domain.PostingPending,
	}

	h := &harness{
		posting:  &postingStore{claim: job},
		accounts: &accountStore{account: account},
		groups:   &groupStore{group: group},
		ads:      &adStore{ad: ad},
		guard:    &guardStub{decision: domain.GuardDecision{Allowed: true}},
		telegram: &telegramStub{result: domain.SendResult{MessageID: 777}},
		alerts:   &alertBook{},
		job:      job,
	}
	h.sender = New(h.posting, h.accounts, h.groups, h.ads, h.guard, h.telegram, vaultStub{}, h.alerts,
		config.TelegramConfig{MaxAttempts: 5, LeaseTTL: 2 * time.Minute, GroupCooldown: time.Hour},
		"worker-test", logger.New("test", "error"))
	h.sender.now = func() time.Time { return clock }
	h.sender.jitter = func(d time.Duration) time.Duration { return d }
	return h
}

func (h *harness) run(t *testing.T) domain.JobOutcome {
	t.Helper()
	if err := h.sender.Process(context.Background(), h.job.ID); err != nil {
		t.Fatalf("Process: %v", err)
	}
	if len(h.posting.finished) != 1 {
		t.Fatalf("recorded %d outcomes, want exactly one", len(h.posting.finished))
	}
	return h.posting.finished[0]
}

func (h *harness) refuseWith(kind domain.TelegramErrorKind, code string, wait time.Duration) {
	h.telegram.err = &domain.TelegramError{Kind: kind, Code: code, Wait: wait}
}

func TestASentPostIsRecordedWithItsMessageID(t *testing.T) {
	h := newHarness(t)

	outcome := h.run(t)

	if outcome.Status != domain.PostingSent {
		t.Fatalf("status = %q, want sent", outcome.Status)
	}
	if outcome.TgMessageID == nil || *outcome.TgMessageID != 777 {
		t.Errorf("message id = %v", outcome.TgMessageID)
	}
	if h.telegram.request.RandomID != 424242 {
		t.Errorf("sent with random id %d, want the job's own", h.telegram.request.RandomID)
	}
	if h.telegram.session != "opened session" {
		t.Error("the session was not opened from the vault")
	}
	for _, b := range h.telegram.request.Session {
		if b != 0 {
			t.Fatal("the opened session was left in memory after the send")
		}
	}
	if !h.groups.posted || !h.accounts.touched {
		t.Error("the group's last post and the account's last use were not recorded")
	}
}

// Telegram asking for a wait is a wait for the account, not for this message.
func TestFloodWaitPausesTheWholeAccount(t *testing.T) {
	h := newHarness(t)
	h.refuseWith(domain.TelegramFloodWait, "FLOOD_WAIT_600", 10*time.Minute)

	outcome := h.run(t)

	if outcome.Status != domain.PostingRateLimited {
		t.Fatalf("status = %q, want rate_limited", outcome.Status)
	}
	want := clock.Add(10 * time.Minute)
	if outcome.NextAttemptAt == nil || !outcome.NextAttemptAt.Equal(want) {
		t.Errorf("next attempt = %v, want %v", outcome.NextAttemptAt, want)
	}
	if len(h.guard.pauses) != 1 || !h.guard.pauses[0].Equal(want) {
		t.Errorf("guard pauses = %v, want one until %v", h.guard.pauses, want)
	}
	if len(h.accounts.statuses) != 1 || h.accounts.statuses[0] != domain.TelegramAccountPaused {
		t.Errorf("account statuses = %v, want paused", h.accounts.statuses)
	}
	if h.guard.releases != 1 {
		t.Errorf("released %d times, want 1: the message did not go out", h.guard.releases)
	}
	if outcome.ErrorCode == nil || *outcome.ErrorCode != "FLOOD_WAIT_600" {
		t.Errorf("error code = %v, want Telegram's own", outcome.ErrorCode)
	}
	assertSpam(t, h, "flood_wait")

	alert := h.alerts.only(t, domain.AlertAccountPaused)
	if alert.Until == nil || !alert.Until.Equal(want) || alert.OwnerID != h.accounts.account.UserID {
		t.Errorf("alert = %+v, want the owner told until when", alert)
	}
}

// Slow mode is the group's rule, so only that group waits.
func TestSlowmodeWaitsForTheGroupOnly(t *testing.T) {
	h := newHarness(t)
	h.refuseWith(domain.TelegramSlowmode, "SLOWMODE_WAIT_120", 2*time.Minute)

	outcome := h.run(t)

	if outcome.Status != domain.PostingRateLimited {
		t.Fatalf("status = %q", outcome.Status)
	}
	if len(h.guard.pauses) != 0 || len(h.accounts.statuses) != 0 {
		t.Error("slow mode in one group paused the whole account")
	}
	if len(h.alerts.alerts) != 0 {
		t.Errorf("a group's slow mode is routine and reported nothing, got %+v", h.alerts.alerts)
	}
	assertSpam(t, h, "slowmode")
}

// Telegram calling the account a spammer stops everything from it, and nothing
// is retried.
func TestPeerFloodStopsTheAccount(t *testing.T) {
	h := newHarness(t)
	h.refuseWith(domain.TelegramPeerFlood, "PEER_FLOOD", 0)

	outcome := h.run(t)

	if outcome.Status != domain.PostingFailed {
		t.Fatalf("status = %q, want failed", outcome.Status)
	}
	if outcome.NextAttemptAt != nil {
		t.Error("a restricted account's post was scheduled for another try")
	}
	if len(h.accounts.statuses) != 1 || h.accounts.statuses[0] != domain.TelegramAccountRestricted {
		t.Errorf("account statuses = %v, want restricted", h.accounts.statuses)
	}
	if len(h.posting.cancelled) != 1 {
		t.Error("the account's other queued posts were not cancelled")
	}
	assertSpam(t, h, "peer_flood")

	alert := h.alerts.only(t, domain.AlertAccountRestricted)
	if alert.Cancelled != 1 || alert.CompanyID != h.job.CompanyID {
		t.Errorf("alert = %+v, want the company and the cancelled count", alert)
	}
	// The number is how people tell accounts apart, and it is still not sent.
	if alert.AccountLabel != "+99890 *** ** 67" {
		t.Errorf("label = %q, want the masked number", alert.AccountLabel)
	}
	// The group did nothing wrong, so it keeps its settings: the account is
	// what Telegram stopped, and it may post there again once that lifts.
	if h.groups.disabled || h.groups.deactivated {
		t.Error("a group was turned off over an account-wide restriction")
	}
}

// Telegram words this one as though a single channel banned the account, but
// it means the account may not write in supergroups at all. Read as a group's
// refusal it would quietly turn off every group the person posts to.
func TestBeingBannedFromGroupsStopsTheAccountNotTheGroup(t *testing.T) {
	h := newHarness(t)
	h.refuseWith(domain.TelegramPeerFlood, "USER_BANNED_IN_CHANNEL", 0)

	outcome := h.run(t)

	if outcome.Status != domain.PostingFailed {
		t.Fatalf("status = %q, want failed", outcome.Status)
	}
	if len(h.accounts.statuses) != 1 || h.accounts.statuses[0] != domain.TelegramAccountRestricted {
		t.Errorf("account statuses = %v, want restricted", h.accounts.statuses)
	}
	if h.groups.disabled {
		t.Error("posting was turned off for the group instead of the account")
	}
	if outcome.Detail == nil || !strings.Contains(*outcome.Detail, "@SpamBot") {
		t.Errorf("detail = %v, want somewhere to find out why", outcome.Detail)
	}
}

func TestAGroupThatRefusesIsClosed(t *testing.T) {
	h := newHarness(t)
	h.refuseWith(domain.TelegramWriteForbidden, "CHAT_WRITE_FORBIDDEN", 0)

	outcome := h.run(t)

	if outcome.Status != domain.PostingFailed || !h.groups.disabled {
		t.Errorf("status %q, group disabled %v", outcome.Status, h.groups.disabled)
	}
	if h.guard.releases != 1 {
		t.Error("a refused post still counts against the limits")
	}
	assertSpam(t, h, "write_forbidden")

	alert := h.alerts.only(t, domain.AlertGroupClosed)
	if alert.GroupID == nil || *alert.GroupID != h.groups.group.ID || alert.GroupTitle != "Yuk markazi" {
		t.Errorf("alert = %+v, want the group named", alert)
	}
}

func TestAGroupTheAccountLeftIsDeactivated(t *testing.T) {
	h := newHarness(t)
	h.refuseWith(domain.TelegramGroupGone, "CHANNEL_PRIVATE", 0)

	outcome := h.run(t)

	if outcome.Status != domain.PostingFailed || !h.groups.deactivated {
		t.Errorf("status %q, group deactivated %v", outcome.Status, h.groups.deactivated)
	}
	h.alerts.only(t, domain.AlertGroupLost)
}

func TestARevokedSessionCancelsWhatIsQueued(t *testing.T) {
	h := newHarness(t)
	h.refuseWith(domain.TelegramSessionRevoked, "SESSION_REVOKED", 0)

	outcome := h.run(t)

	if outcome.Status != domain.PostingCancelled {
		t.Fatalf("status = %q, want cancelled", outcome.Status)
	}
	if len(h.accounts.statuses) != 1 || h.accounts.statuses[0] != domain.TelegramAccountRevoked {
		t.Errorf("account statuses = %v", h.accounts.statuses)
	}
	if len(h.posting.cancelled) != 1 {
		t.Error("posts queued from a dead session were left in the queue")
	}
	h.alerts.only(t, domain.AlertAccountRevoked)
}

// Telegram refusing a random_id it has seen means the message already went out.
func TestADuplicateRandomIDMeansItWasSent(t *testing.T) {
	h := newHarness(t)
	h.refuseWith(domain.TelegramDuplicate, "RANDOM_ID_DUPLICATE", 0)

	if outcome := h.run(t); outcome.Status != domain.PostingSent {
		t.Errorf("status = %q, want sent", outcome.Status)
	}
}

// A network failure is retried, and the reservation is kept: the message may
// have gone out, and counting a post that was sent is the safer mistake.
func TestATransientFailureIsRetriedWithoutReleasing(t *testing.T) {
	h := newHarness(t)
	h.telegram.err = errors.New("connection reset by peer")

	outcome := h.run(t)

	if outcome.Status != domain.PostingPending {
		t.Fatalf("status = %q, want pending", outcome.Status)
	}
	want := clock.Add(domain.Backoff(1))
	if outcome.NextAttemptAt == nil || !outcome.NextAttemptAt.Equal(want) {
		t.Errorf("next attempt = %v, want %v", outcome.NextAttemptAt, want)
	}
	if h.guard.releases != 0 {
		t.Error("an ambiguous failure gave its allowance back")
	}
}

func TestTransientFailuresStopAfterTheLastAttempt(t *testing.T) {
	h := newHarness(t)
	h.job.Attempt = 5
	h.telegram.err = errors.New("i/o timeout")

	if outcome := h.run(t); outcome.Status != domain.PostingFailed {
		t.Errorf("status = %q, want failed after the last attempt", outcome.Status)
	}
}

// Sending again on a failure nobody understands is how duplicates and bans
// happen.
func TestAnUnknownRefusalEndsTheJob(t *testing.T) {
	h := newHarness(t)
	h.refuseWith(domain.TelegramUnknown, "SOMETHING_NEW", 0)

	outcome := h.run(t)

	if outcome.Status != domain.PostingFailed || outcome.NextAttemptAt != nil {
		t.Errorf("status %q next %v, want failed with no retry", outcome.Status, outcome.NextAttemptAt)
	}
	if h.guard.releases != 0 {
		t.Error("an unexplained refusal gave its allowance back")
	}
	if alert := h.alerts.only(t, domain.AlertTelegramUnknown); alert.ErrorCode != "SOMETHING_NEW" {
		t.Errorf("alert code = %q, want Telegram's own", alert.ErrorCode)
	}
}

func TestTheGuardDecidesBeforeTelegramIsCalled(t *testing.T) {
	cases := map[string]struct {
		decision domain.GuardDecision
		status   domain.PostingJobStatus
	}{
		"cooldown is a wait": {
			decision: domain.GuardDecision{Reason: domain.SkipCooldown, RetryAfter: 20 * time.Minute},
			status:   domain.PostingRateLimited,
		},
		"a duplicate is a refusal": {
			decision: domain.GuardDecision{Reason: domain.SkipDuplicate},
			status:   domain.PostingSkipped,
		},
		"a wait past the loading day is pointless": {
			decision: domain.GuardDecision{Reason: domain.SkipGroupLimit, RetryAfter: 10 * 24 * time.Hour},
			status:   domain.PostingSkipped,
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			h := newHarness(t)
			h.guard.decision = tc.decision

			outcome := h.run(t)

			if outcome.Status != tc.status {
				t.Errorf("status = %q, want %q", outcome.Status, tc.status)
			}
			if h.telegram.calls != 0 {
				t.Error("Telegram was called for a post the guard held back")
			}
		})
	}
}

// A job whose worker died mid-send may already be in the group. Until Telegram's
// handling of random_id is confirmed, it is not sent again.
func TestAJobInterruptedMidSendIsNotResent(t *testing.T) {
	h := newHarness(t)
	h.job.PreviousStatus = domain.PostingProcessing

	outcome := h.run(t)

	if outcome.Status != domain.PostingFailed {
		t.Fatalf("status = %q, want failed", outcome.Status)
	}
	if outcome.ErrorCode == nil || *outcome.ErrorCode != "outcome_unknown" {
		t.Errorf("error code = %v", outcome.ErrorCode)
	}
	if h.telegram.calls != 0 || h.guard.reserves != 0 {
		t.Error("an interrupted job was sent again")
	}
}

func TestOnceRandomIDIsTrustedAnInterruptedJobIsRetried(t *testing.T) {
	h := newHarness(t)
	h.job.PreviousStatus = domain.PostingProcessing
	h.sender.limits.TrustRandomID = true

	if outcome := h.run(t); outcome.Status != domain.PostingSent {
		t.Errorf("status = %q, want sent", outcome.Status)
	}
	if h.telegram.request.RandomID != 424242 {
		t.Error("the retry did not reuse the job's random id")
	}
}

func TestAPausedAccountWaitsWithoutSending(t *testing.T) {
	h := newHarness(t)
	until := clock.Add(5 * time.Minute)
	h.guard.pausedUntil = &until

	outcome := h.run(t)

	if outcome.Status != domain.PostingRateLimited || !outcome.NextAttemptAt.Equal(until) {
		t.Errorf("status %q next %v, want rate_limited until %v", outcome.Status, outcome.NextAttemptAt, until)
	}
	if h.telegram.calls != 0 {
		t.Error("a paused account sent a message")
	}
}

func TestARestrictedAccountSendsNothing(t *testing.T) {
	h := newHarness(t)
	h.accounts.account.Status = domain.TelegramAccountRestricted

	outcome := h.run(t)

	if outcome.Status != domain.PostingCancelled || h.telegram.calls != 0 {
		t.Errorf("status %q, calls %d", outcome.Status, h.telegram.calls)
	}
}

func TestAClosedGroupIsSkipped(t *testing.T) {
	h := newHarness(t)
	h.groups.group.PostingEnabled = false

	outcome := h.run(t)

	if outcome.Status != domain.PostingSkipped || h.telegram.calls != 0 {
		t.Errorf("status %q, calls %d", outcome.Status, h.telegram.calls)
	}
}

// The queue can deliver the same id twice; the claim decides who works on it.
func TestAJobSomeoneElseHoldsIsLeftAlone(t *testing.T) {
	h := newHarness(t)
	h.posting.claim = nil

	if err := h.sender.Process(context.Background(), h.job.ID); err != nil {
		t.Fatalf("Process: %v", err)
	}
	if len(h.posting.finished) != 0 || h.telegram.calls != 0 {
		t.Error("a job claimed by another worker was processed here too")
	}
}

func assertSpam(t *testing.T, h *harness, kind string) {
	t.Helper()
	if slices.Contains(h.posting.spamEvents, kind) {
		return
	}
	t.Errorf("spam events = %v, want %q recorded", h.posting.spamEvents, kind)
}

// metricBook records what the sender reports.
type metricBook struct {
	finished []domain.PostingJobStatus
	refusals []domain.SkipReason
	sends    []domain.TelegramErrorKind
	alerts   []domain.DistributionAlertKind
}

func (m *metricBook) JobFinished(s domain.PostingJobStatus) { m.finished = append(m.finished, s) }
func (m *metricBook) GuardRefused(r domain.SkipReason)      { m.refusals = append(m.refusals, r) }
func (m *metricBook) TelegramSend(_ time.Duration, k domain.TelegramErrorKind) {
	m.sends = append(m.sends, k)
}
func (m *metricBook) Alerted(k domain.DistributionAlertKind) { m.alerts = append(m.alerts, k) }

// What monitoring sees has to match what happened: a send, a Telegram refusal
// by its kind, and a guard refusal that never reached Telegram at all.
func TestTheSenderReportsWhatHappened(t *testing.T) {
	t.Run("a sent post", func(t *testing.T) {
		h := newHarness(t)
		book := &metricBook{}
		h.sender.WithMetrics(book)
		h.run(t)

		if !slices.Equal(book.finished, []domain.PostingJobStatus{domain.PostingSent}) {
			t.Errorf("finished = %v, want [sent]", book.finished)
		}
		if !slices.Equal(book.sends, []domain.TelegramErrorKind{""}) {
			t.Errorf("sends = %v, want one that succeeded", book.sends)
		}
	})

	t.Run("a flood wait", func(t *testing.T) {
		h := newHarness(t)
		book := &metricBook{}
		h.sender.WithMetrics(book)
		h.refuseWith(domain.TelegramFloodWait, "FLOOD_WAIT_600", 10*time.Minute)
		h.run(t)

		if !slices.Equal(book.sends, []domain.TelegramErrorKind{domain.TelegramFloodWait}) {
			t.Errorf("sends = %v, want one flood_wait", book.sends)
		}
		if !slices.Equal(book.finished, []domain.PostingJobStatus{domain.PostingRateLimited}) {
			t.Errorf("finished = %v, want [rate_limited]", book.finished)
		}
		if !slices.Equal(book.alerts, []domain.DistributionAlertKind{domain.AlertAccountPaused}) {
			t.Errorf("alerts = %v, want the pause", book.alerts)
		}
	})

	t.Run("a guard refusal", func(t *testing.T) {
		h := newHarness(t)
		book := &metricBook{}
		h.sender.WithMetrics(book)
		h.guard.decision = domain.GuardDecision{Reason: domain.SkipCooldown, RetryAfter: 20 * time.Minute}
		h.run(t)

		if !slices.Equal(book.refusals, []domain.SkipReason{domain.SkipCooldown}) {
			t.Errorf("refusals = %v, want [cooldown]", book.refusals)
		}
		if len(book.sends) != 0 {
			t.Errorf("a send was timed although Telegram was never called: %v", book.sends)
		}
	})
}
