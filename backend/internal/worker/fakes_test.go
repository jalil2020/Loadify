package worker

import (
	"context"
	"time"

	"github.com/google/uuid"

	"github.com/loadify/backend/internal/domain"
	"github.com/loadify/backend/pkg/apperror"
)

// Each fake records what the worker did to it, because the consequences of a
// Telegram refusal are side effects on the account, the group and the queue —
// the job's own status is only part of the answer.

type postingStore struct {
	claim      *domain.PostingJob
	finished   []domain.JobOutcome
	cancelled  []uuid.UUID
	spamEvents []string
}

func (p *postingStore) Claim(context.Context, uuid.UUID, string, time.Duration) (*domain.PostingJob, error) {
	return p.claim, nil
}

func (p *postingStore) Finish(_ context.Context, _ uuid.UUID, outcome domain.JobOutcome) error {
	p.finished = append(p.finished, outcome)
	return nil
}

func (p *postingStore) CancelForAccount(_ context.Context, accountID uuid.UUID, _ string) (int64, error) {
	p.cancelled = append(p.cancelled, accountID)
	return 1, nil
}

func (p *postingStore) RecordSpamEvent(_ context.Context, event *domain.SpamEvent) error {
	p.spamEvents = append(p.spamEvents, event.Kind)
	return nil
}

func (p *postingStore) CreateRun(context.Context, *domain.PublishRun, []domain.PostingJob) error {
	return nil
}
func (p *postingStore) FindRunByIdempotencyKey(context.Context, uuid.UUID, string) (*domain.PublishRun, error) {
	return nil, apperror.ErrNotFound
}
func (p *postingStore) FindJob(context.Context, uuid.UUID, uuid.UUID) (*domain.PostingJob, error) {
	return nil, apperror.ErrNotFound
}
func (p *postingStore) ListJobs(context.Context, uuid.UUID, domain.PostingJobFilter, int, int) ([]domain.PostingJob, int64, error) {
	return nil, 0, nil
}
func (p *postingStore) DueJobs(context.Context, int) ([]domain.PostingJob, error) { return nil, nil }
func (p *postingStore) Cancel(context.Context, uuid.UUID, uuid.UUID) (bool, error) {
	return false, nil
}
func (p *postingStore) CancelForLoad(context.Context, uuid.UUID, uuid.UUID, string) (int64, error) {
	return 0, nil
}
func (p *postingStore) AppendEvent(context.Context, *domain.PostingEvent) error { return nil }
func (p *postingStore) CountSentBetween(context.Context, uuid.UUID, time.Time, time.Time) (int64, error) {
	return 0, nil
}

type accountStore struct {
	account  *domain.TelegramAccount
	statuses []domain.TelegramAccountStatus
	touched  bool
}

func (a *accountStore) FindByID(context.Context, uuid.UUID, uuid.UUID) (*domain.TelegramAccount, error) {
	if a.account == nil {
		return nil, apperror.ErrNotFound
	}
	return a.account, nil
}

func (a *accountStore) SetStatus(_ context.Context, _ uuid.UUID, status domain.TelegramAccountStatus, _ *string, _ *time.Time) error {
	a.statuses = append(a.statuses, status)
	return nil
}

func (a *accountStore) TouchUsed(context.Context, uuid.UUID, time.Time) error {
	a.touched = true
	return nil
}

func (a *accountStore) Create(context.Context, *domain.TelegramAccount) error { return nil }
func (a *accountStore) FindForUser(context.Context, uuid.UUID, uuid.UUID, uuid.UUID) (*domain.TelegramAccount, error) {
	return a.account, nil
}
func (a *accountStore) ListByCompany(context.Context, uuid.UUID) ([]domain.TelegramAccount, error) {
	return nil, nil
}
func (a *accountStore) ListByUser(context.Context, uuid.UUID, uuid.UUID) ([]domain.TelegramAccount, error) {
	return nil, nil
}
func (a *accountStore) StoreSession(context.Context, uuid.UUID, []byte, int16, int64) error {
	return nil
}
func (a *accountStore) Disconnect(context.Context, uuid.UUID, uuid.UUID) error { return nil }

type groupStore struct {
	group       *domain.TelegramGroup
	disabled    bool
	deactivated bool
	posted      bool
}

func (g *groupStore) FindByID(context.Context, uuid.UUID, uuid.UUID) (*domain.TelegramGroup, error) {
	if g.group == nil {
		return nil, apperror.ErrNotFound
	}
	return g.group, nil
}

func (g *groupStore) Rules(context.Context, uuid.UUID, uuid.UUID) (*domain.TelegramGroupRules, []domain.TelegramGroupRoute, error) {
	return &domain.TelegramGroupRules{HourlyLimit: 2, DailyLimit: 3, CooldownMinutes: 60}, nil, nil
}

func (g *groupStore) DisablePosting(context.Context, uuid.UUID, uuid.UUID, string) error {
	g.disabled = true
	return nil
}

func (g *groupStore) Deactivate(context.Context, uuid.UUID, uuid.UUID, string) error {
	g.deactivated = true
	return nil
}

func (g *groupStore) MarkPosted(context.Context, uuid.UUID, time.Time) error {
	g.posted = true
	return nil
}

func (g *groupStore) Upsert(context.Context, *domain.TelegramGroup) error { return nil }
func (g *groupStore) ListByAccount(context.Context, uuid.UUID, uuid.UUID) ([]domain.TelegramGroup, error) {
	return nil, nil
}
func (g *groupStore) ListByCompany(context.Context, uuid.UUID) ([]domain.TelegramGroup, error) {
	return nil, nil
}
func (g *groupStore) Candidates(context.Context, uuid.UUID, domain.GroupCandidateQuery) ([]domain.GroupCandidate, error) {
	return nil, nil
}
func (g *groupStore) SaveRules(context.Context, *domain.TelegramGroupRules, []domain.TelegramGroupRoute) error {
	return nil
}
func (g *groupStore) Remove(context.Context, uuid.UUID, uuid.UUID) error    { return nil }
func (g *groupStore) CountActive(context.Context, uuid.UUID) (int64, error) { return 0, nil }

type adStore struct{ ad *domain.LoadAd }

func (a *adStore) FindByID(context.Context, uuid.UUID, uuid.UUID) (*domain.LoadAd, error) {
	if a.ad == nil {
		return nil, apperror.ErrNotFound
	}
	return a.ad, nil
}

func (a *adStore) Create(context.Context, *domain.LoadAd) error { return nil }
func (a *adStore) Update(context.Context, *domain.LoadAd) error { return nil }
func (a *adStore) UpdateColumns(context.Context, uuid.UUID, uuid.UUID, map[string]any) error {
	return nil
}
func (a *adStore) List(context.Context, uuid.UUID, domain.LoadAdFilter, int, int) ([]domain.LoadAd, int64, error) {
	return nil, 0, nil
}
func (a *adStore) Archive(context.Context, uuid.UUID, uuid.UUID) error { return nil }
func (a *adStore) CountCreatedBetween(context.Context, uuid.UUID, time.Time, time.Time) (int64, error) {
	return 0, nil
}

type guardStub struct {
	decision    domain.GuardDecision
	pausedUntil *time.Time
	pauses      []time.Time
	releases    int
	reserves    int
}

func (g *guardStub) Reserve(context.Context, domain.GuardTarget, time.Time) (domain.GuardDecision, error) {
	g.reserves++
	return g.decision, nil
}

func (g *guardStub) Release(context.Context, domain.GuardTarget) error {
	g.releases++
	return nil
}

func (g *guardStub) PauseAccount(_ context.Context, _ uuid.UUID, until time.Time) error {
	g.pauses = append(g.pauses, until)
	return nil
}

func (g *guardStub) AccountPausedUntil(context.Context, uuid.UUID) (time.Time, bool, error) {
	if g.pausedUntil == nil {
		return time.Time{}, false, nil
	}
	return *g.pausedUntil, true, nil
}

func (g *guardStub) Plan(context.Context, []domain.GuardTarget, time.Time) ([]domain.GuardDecision, error) {
	return nil, nil
}

type telegramStub struct {
	result  domain.SendResult
	err     error
	calls   int
	request domain.SendRequest
	// session is copied at call time: the worker wipes its own buffer as soon
	// as the send returns, which is the point.
	session string
}

func (t *telegramStub) Send(_ context.Context, req domain.SendRequest) (domain.SendResult, error) {
	t.calls++
	t.request = req
	t.session = string(req.Session)
	return t.result, t.err
}

type vaultStub struct{}

func (vaultStub) Seal([]byte, string) ([]byte, int, error) { return nil, 1, nil }
func (vaultStub) Open([]byte, int, string) ([]byte, error) { return []byte("opened session"), nil }
