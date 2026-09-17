package usecase

import (
	"context"
	"testing"

	"github.com/google/uuid"

	"github.com/loadify/backend/internal/domain"
	"github.com/loadify/backend/internal/dto"
	"github.com/loadify/backend/pkg/apperror"
	"github.com/loadify/backend/pkg/logger"
)

// sameTx runs the function in the caller's context; the fakes have no
// transactions to join.
type sameTx struct{}

func (sameTx) WithinTransaction(ctx context.Context, fn func(context.Context) error) error {
	return fn(ctx)
}

// actorParser remembers who a parse was charged to.
type actorParser struct {
	actor domain.Actor
	found bool
}

func (p *actorParser) Parse(ctx context.Context, text string) (domain.AdDraft, error) {
	p.actor, p.found = domain.ActorFrom(ctx)
	return domain.AdDraft{Ad: domain.LoadAd{RawText: text}}, nil
}

func TestParsingIsChargedToThePersonsCompany(t *testing.T) {
	parser := &actorParser{}
	u := NewLoadAdUsecase(&fakeAdRepo{ads: map[uuid.UUID]*domain.LoadAd{}}, parser, nil, &fakePosting{}, sameTx{},
		noopAudit{}, logger.New("test", "error"))
	userID, companyID := uuid.New(), uuid.New()

	res, err := u.Parse(context.Background(), userID, companyID, "Toshkent - Moskva")
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if !parser.found || parser.actor.CompanyID != companyID || parser.actor.UserID != userID {
		t.Errorf("actor = %+v (found %v), want the caller", parser.actor, parser.found)
	}
	if res.AIStatus != domain.AIStatusOff {
		t.Errorf("ai_status = %q with no model configured, want off", res.AIStatus)
	}
}

func TestTheMonthlyAdvertAllowanceIsEnforced(t *testing.T) {
	quota := newQuotaStub(map[domain.QuotaMetric]int{domain.QuotaLoadsPerMonth: 1})
	u := NewLoadAdUsecase(&fakeAdRepo{ads: map[uuid.UUID]*domain.LoadAd{}}, &actorParser{}, quota, &fakePosting{}, sameTx{},
		noopAudit{}, logger.New("test", "error"))
	userID, companyID := uuid.New(), uuid.New()
	req := dto.SaveLoadAdRequest{RawText: "Toshkent - Moskva"}

	if _, err := u.Create(context.Background(), userID, companyID, req); err != nil {
		t.Fatalf("first advert: %v", err)
	}
	_, err := u.Create(context.Background(), userID, companyID, req)
	if !apperror.Is(err, apperror.ErrQuotaExceeded.Code) {
		t.Fatalf("second advert: err = %v, want the plan limit", err)
	}
}

// Archiving an advert stops what is still queued for it; what already went out
// stays as it is.
func TestArchivingAnAdvertCancelsItsQueue(t *testing.T) {
	companyID, adID := uuid.New(), uuid.New()
	posting := &fakePosting{jobs: []domain.PostingJob{
		{ID: uuid.New(), CompanyID: companyID, LoadAdID: adID, Status: domain.PostingPending},
		{ID: uuid.New(), CompanyID: companyID, LoadAdID: adID, Status: domain.PostingRateLimited},
		{ID: uuid.New(), CompanyID: companyID, LoadAdID: adID, Status: domain.PostingSent},
		{ID: uuid.New(), CompanyID: companyID, LoadAdID: uuid.New(), Status: domain.PostingPending},
	}}
	ads := &fakeAdRepo{ads: map[uuid.UUID]*domain.LoadAd{adID: {ID: adID, CompanyID: companyID}}}
	u := NewLoadAdUsecase(ads, &actorParser{}, nil, posting, sameTx{}, noopAudit{}, logger.New("test", "error"))

	if err := u.Archive(context.Background(), uuid.New(), companyID, adID); err != nil {
		t.Fatalf("Archive: %v", err)
	}

	want := []domain.PostingJobStatus{
		domain.PostingCancelled, domain.PostingCancelled, domain.PostingSent, domain.PostingPending,
	}
	for i, job := range posting.jobs {
		if job.Status != want[i] {
			t.Errorf("job %d is %s, want %s", i, job.Status, want[i])
		}
	}
}
