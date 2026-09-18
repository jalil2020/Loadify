package usecase

import (
	"context"
	"errors"
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

// routeParser reads every advert as Tashkent to Moscow with a phone, and counts
// how often it was asked.
type routeParser struct{ calls int }

func (p *routeParser) Parse(context.Context, string) (domain.AdDraft, error) {
	p.calls++
	draft := domain.AdDraft{Ad: domain.LoadAd{
		RawText: "rewritten by the parser", OriginText: "Toshkent", DestText: "Moskva",
		OriginCountry: pointer("UZ"), DestCountry: pointer("RU"), ContactPhone: pointer("+998901234567"),
	}}
	draft.Note(domain.FieldOrigin, domain.FieldFromRule, "Toshkent")
	return draft, nil
}

// failingParser cannot read anything.
type failingParser struct{}

func (failingParser) Parse(context.Context, string) (domain.AdDraft, error) {
	return domain.AdDraft{}, errors.New("vocabulary unavailable")
}

func newAdUsecase(parser domain.LoadAdParser) (*LoadAdUsecase, *fakeAdRepo) {
	ads := &fakeAdRepo{ads: map[uuid.UUID]*domain.LoadAd{}}
	return NewLoadAdUsecase(ads, parser, nil, &fakePosting{}, sameTx{}, noopAudit{}, logger.New("test", "error")), ads
}

// Saving only the text reads it on the server: the reading is kept for
// matching, and the text is stored exactly as the person wrote it.
func TestSavingTextReadsItAndKeepsItAsWritten(t *testing.T) {
	parser := &routeParser{}
	u, ads := newAdUsecase(parser)
	text := "  Toshkent - Moskva, 20t tent\nTel 90 123 45 67 "

	res, err := u.Create(context.Background(), uuid.New(), uuid.New(), dto.SaveLoadAdRequest{RawText: text})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if parser.calls != 1 {
		t.Fatalf("parser asked %d times, want once", parser.calls)
	}
	stored := ads.ads[res.ID]
	if stored.RawText != text {
		t.Errorf("stored text = %q, want it exactly as written %q", stored.RawText, text)
	}
	if stored.OriginText != "Toshkent" || stored.DestCountry == nil || *stored.DestCountry != "RU" {
		t.Errorf("the reading was not kept: origin %q, destination country %v", stored.OriginText, stored.DestCountry)
	}
	if stored.FieldSources[domain.FieldOrigin].Source != domain.FieldFromRule {
		t.Errorf("origin source = %+v, want the rule that read it", stored.FieldSources[domain.FieldOrigin])
	}
	if stored.Status != domain.LoadAdReady || len(res.Missing) != 0 {
		t.Errorf("status %q with missing %v, want ready with nothing missing", stored.Status, res.Missing)
	}
	// Clients read its length: nothing missing is an empty list, never null.
	if res.Missing == nil {
		t.Error("missing is nil, which goes over the wire as null")
	}
	if stored.Fingerprint != stored.ComputeFingerprint() {
		t.Error("the fingerprint does not describe what was stored")
	}
}

// A client that sends fields has decided them; the server does not second-guess
// them with a reading of its own.
func TestFieldsFromTheClientAreNotReadAgain(t *testing.T) {
	parser := &routeParser{}
	u, ads := newAdUsecase(parser)

	res, err := u.Create(context.Background(), uuid.New(), uuid.New(), dto.SaveLoadAdRequest{
		RawText: "Samarqand - Almaty", OriginText: "Samarqand", DestText: "Almaty",
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if parser.calls != 0 {
		t.Errorf("parser asked %d times for an advert that came with fields", parser.calls)
	}
	if got := ads.ads[res.ID].OriginText; got != "Samarqand" {
		t.Errorf("origin = %q, want the client's", got)
	}
}

// A reading that fails costs the advert its route, not the save.
func TestAnAdvertIsSavedWhenTheTextCannotBeRead(t *testing.T) {
	u, ads := newAdUsecase(failingParser{})

	res, err := u.Create(context.Background(), uuid.New(), uuid.New(), dto.SaveLoadAdRequest{RawText: "Yuk bor"})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if got := ads.ads[res.ID]; got.RawText != "Yuk bor" || got.Status != domain.LoadAdReady {
		t.Errorf("stored %q as %q, want the text, ready to post", got.RawText, got.Status)
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
