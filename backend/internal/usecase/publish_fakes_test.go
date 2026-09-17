package usecase

import (
	"context"
	"time"

	"github.com/google/uuid"

	"github.com/loadify/backend/internal/domain"
	"github.com/loadify/backend/pkg/apperror"
)

// The fakes below implement only what the publish flow calls. Everything else
// returns a zero value: a method that grows a caller will fail loudly in the
// test that added it rather than passing quietly here.

type fakeAdRepo struct{ ads map[uuid.UUID]*domain.LoadAd }

func (f *fakeAdRepo) Create(context.Context, *domain.LoadAd) error { return nil }
func (f *fakeAdRepo) Update(context.Context, *domain.LoadAd) error { return nil }

func (f *fakeAdRepo) UpdateColumns(_ context.Context, _, id uuid.UUID, columns map[string]any) error {
	ad, found := f.ads[id]
	if !found {
		return apperror.ErrNotFound
	}
	if status, ok := columns["status"].(domain.LoadAdStatus); ok {
		ad.Status = status
	}
	return nil
}

func (f *fakeAdRepo) FindByID(_ context.Context, _, id uuid.UUID) (*domain.LoadAd, error) {
	ad, found := f.ads[id]
	if !found {
		return nil, apperror.ErrNotFound
	}
	return ad, nil
}

func (f *fakeAdRepo) List(context.Context, uuid.UUID, domain.LoadAdFilter, int, int) ([]domain.LoadAd, int64, error) {
	return nil, 0, nil
}
func (f *fakeAdRepo) Archive(context.Context, uuid.UUID, uuid.UUID) error { return nil }

type fakeAccountRepo struct{ accounts []domain.TelegramAccount }

func (f *fakeAccountRepo) Create(context.Context, *domain.TelegramAccount) error { return nil }

func (f *fakeAccountRepo) FindByID(_ context.Context, _, id uuid.UUID) (*domain.TelegramAccount, error) {
	return f.find(id)
}

func (f *fakeAccountRepo) FindForUser(_ context.Context, _, userID, id uuid.UUID) (*domain.TelegramAccount, error) {
	account, err := f.find(id)
	if err != nil {
		return nil, err
	}
	if account.UserID != userID {
		return nil, apperror.ErrNotFound
	}
	return account, nil
}

func (f *fakeAccountRepo) find(id uuid.UUID) (*domain.TelegramAccount, error) {
	for i := range f.accounts {
		if f.accounts[i].ID == id {
			return &f.accounts[i], nil
		}
	}
	return nil, apperror.ErrNotFound
}

func (f *fakeAccountRepo) ListByCompany(context.Context, uuid.UUID) ([]domain.TelegramAccount, error) {
	return f.accounts, nil
}

func (f *fakeAccountRepo) ListByUser(_ context.Context, _, userID uuid.UUID) ([]domain.TelegramAccount, error) {
	var out []domain.TelegramAccount
	for _, account := range f.accounts {
		if account.UserID == userID {
			out = append(out, account)
		}
	}
	return out, nil
}

func (f *fakeAccountRepo) StoreSession(context.Context, uuid.UUID, []byte, int16, int64) error {
	return nil
}
func (f *fakeAccountRepo) SetStatus(context.Context, uuid.UUID, domain.TelegramAccountStatus, *string, *time.Time) error {
	return nil
}
func (f *fakeAccountRepo) TouchUsed(context.Context, uuid.UUID, time.Time) error  { return nil }
func (f *fakeAccountRepo) Disconnect(context.Context, uuid.UUID, uuid.UUID) error { return nil }

type fakeGroupRepo struct{ candidates []domain.GroupCandidate }

func (f *fakeGroupRepo) Upsert(context.Context, *domain.TelegramGroup) error { return nil }

func (f *fakeGroupRepo) FindByID(_ context.Context, _, id uuid.UUID) (*domain.TelegramGroup, error) {
	for i := range f.candidates {
		if f.candidates[i].Group.ID == id {
			return &f.candidates[i].Group, nil
		}
	}
	return nil, apperror.ErrNotFound
}

func (f *fakeGroupRepo) ListByAccount(context.Context, uuid.UUID, uuid.UUID) ([]domain.TelegramGroup, error) {
	return nil, nil
}
func (f *fakeGroupRepo) ListByCompany(context.Context, uuid.UUID) ([]domain.TelegramGroup, error) {
	return nil, nil
}

func (f *fakeGroupRepo) Candidates(context.Context, uuid.UUID, domain.GroupCandidateQuery) ([]domain.GroupCandidate, error) {
	return f.candidates, nil
}

func (f *fakeGroupRepo) SaveRules(context.Context, *domain.TelegramGroupRules, []domain.TelegramGroupRoute) error {
	return nil
}
func (f *fakeGroupRepo) Rules(context.Context, uuid.UUID, uuid.UUID) (*domain.TelegramGroupRules, []domain.TelegramGroupRoute, error) {
	return nil, nil, nil
}
func (f *fakeGroupRepo) Deactivate(context.Context, uuid.UUID, uuid.UUID, string) error { return nil }
func (f *fakeGroupRepo) DisablePosting(context.Context, uuid.UUID, uuid.UUID, string) error {
	return nil
}
func (f *fakeGroupRepo) MarkPosted(context.Context, uuid.UUID, time.Time) error { return nil }
func (f *fakeGroupRepo) Remove(context.Context, uuid.UUID, uuid.UUID) error     { return nil }
func (f *fakeGroupRepo) CountActive(context.Context, uuid.UUID) (int64, error)  { return 0, nil }

type fakeCargoRepo struct{}

func (fakeCargoRepo) List(context.Context) ([]domain.CargoType, error) { return nil, nil }
func (fakeCargoRepo) FindByID(context.Context, uuid.UUID) (*domain.CargoType, error) {
	return nil, apperror.ErrNotFound
}

type fakePosting struct {
	runs []domain.PublishRun
	jobs []domain.PostingJob
}

func (f *fakePosting) CreateRun(_ context.Context, run *domain.PublishRun, jobs []domain.PostingJob) error {
	for _, existing := range f.runs {
		if existing.CompanyID == run.CompanyID && existing.IdempotencyKey == run.IdempotencyKey {
			return apperror.ErrConflict
		}
	}
	if run.ID == uuid.Nil {
		run.ID = uuid.New()
	}
	for i := range jobs {
		jobs[i].RunID = run.ID
	}
	f.runs = append(f.runs, *run)
	f.jobs = append(f.jobs, jobs...)
	return nil
}

func (f *fakePosting) FindRunByIdempotencyKey(_ context.Context, companyID uuid.UUID, key string) (*domain.PublishRun, error) {
	for i := range f.runs {
		if f.runs[i].CompanyID == companyID && f.runs[i].IdempotencyKey == key {
			return &f.runs[i], nil
		}
	}
	return nil, apperror.ErrNotFound
}

func (f *fakePosting) FindJob(context.Context, uuid.UUID, uuid.UUID) (*domain.PostingJob, error) {
	return nil, apperror.ErrNotFound
}

// QueuedGroups mirrors the index: only jobs still waiting count.
func (f *fakePosting) QueuedGroups(_ context.Context, companyID, loadAdID uuid.UUID) (map[uuid.UUID]bool, error) {
	queued := map[uuid.UUID]bool{}
	for _, job := range f.jobs {
		if job.CompanyID != companyID || job.LoadAdID != loadAdID {
			continue
		}
		switch job.Status {
		case domain.PostingPending, domain.PostingProcessing, domain.PostingRateLimited:
			queued[job.GroupID] = true
		}
	}
	return queued, nil
}

func (f *fakePosting) ListJobs(_ context.Context, companyID uuid.UUID, filter domain.PostingJobFilter, _, _ int) ([]domain.PostingJob, int64, error) {
	var out []domain.PostingJob
	for _, job := range f.jobs {
		if job.CompanyID != companyID {
			continue
		}
		if filter.LoadAdID != nil && job.LoadAdID != *filter.LoadAdID {
			continue
		}
		out = append(out, job)
	}
	return out, int64(len(out)), nil
}

func (f *fakePosting) DueJobs(context.Context, int) ([]domain.PostingJob, error) { return nil, nil }
func (f *fakePosting) Claim(context.Context, uuid.UUID, string, time.Duration) (*domain.PostingJob, error) {
	return nil, nil
}
func (f *fakePosting) Finish(context.Context, uuid.UUID, domain.JobOutcome) error { return nil }
func (f *fakePosting) Cancel(context.Context, uuid.UUID, uuid.UUID) (bool, error) {
	return false, nil
}
func (f *fakePosting) CancelForLoad(_ context.Context, companyID, loadAdID uuid.UUID, _ string) (int64, error) {
	var n int64
	for i := range f.jobs {
		job := &f.jobs[i]
		if job.CompanyID != companyID || job.LoadAdID != loadAdID {
			continue
		}
		switch job.Status {
		case domain.PostingPending, domain.PostingProcessing, domain.PostingRateLimited:
			job.Status = domain.PostingCancelled
			n++
		}
	}
	return n, nil
}
func (f *fakePosting) CancelForAccount(context.Context, uuid.UUID, string) (int64, error) {
	return 0, nil
}
func (f *fakePosting) AppendEvent(context.Context, *domain.PostingEvent) error  { return nil }
func (f *fakePosting) RecordSpamEvent(context.Context, *domain.SpamEvent) error { return nil }

// fakeGuard answers with whatever the test set up, so the publish flow can be
// exercised without Redis. The guard has its own tests.
type fakeGuard struct {
	refuse map[uuid.UUID]domain.GuardDecision
}

func (f *fakeGuard) Plan(_ context.Context, targets []domain.GuardTarget, _ time.Time) ([]domain.GuardDecision, error) {
	decisions := make([]domain.GuardDecision, 0, len(targets))
	for _, target := range targets {
		if decision, refused := f.refuse[target.GroupID]; refused {
			decision.GroupID = target.GroupID
			decisions = append(decisions, decision)
			continue
		}
		decisions = append(decisions, domain.GuardDecision{GroupID: target.GroupID, Allowed: true})
	}
	return decisions, nil
}

func (f *fakeGuard) Reserve(context.Context, domain.GuardTarget, time.Time) (domain.GuardDecision, error) {
	return domain.GuardDecision{Allowed: true}, nil
}
func (f *fakeGuard) Release(context.Context, domain.GuardTarget) error { return nil }
func (f *fakeGuard) PauseAccount(context.Context, uuid.UUID, time.Time) error {
	return nil
}
func (f *fakeGuard) AccountPausedUntil(context.Context, uuid.UUID) (time.Time, bool, error) {
	return time.Time{}, false, nil
}

type noopAudit struct{}

func (noopAudit) Record(context.Context, domain.AuditEntry) error { return nil }

func (f *fakeGroupRepo) UpdateSettings(context.Context, uuid.UUID, uuid.UUID, *string, *bool) error {
	return nil
}

func (f *fakeGroupRepo) ListWithRules(context.Context, uuid.UUID) ([]domain.GroupCandidate, error) {
	return f.candidates, nil
}
