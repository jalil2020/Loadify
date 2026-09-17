package ai_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/loadify/backend/internal/ai"
	"github.com/loadify/backend/internal/config"
	"github.com/loadify/backend/internal/domain"
	"github.com/loadify/backend/pkg/logger"
)

type allowance struct {
	limit, used int
}

func (a allowance) Status(_ context.Context, _ uuid.UUID, metric domain.QuotaMetric) (domain.QuotaStatus, error) {
	return domain.QuotaStatus{Metric: metric, Limit: a.limit, Used: a.used}, nil
}

func (a allowance) Reserve(ctx context.Context, _ uuid.UUID, _ domain.QuotaMetric, _ int, act func(context.Context) error) error {
	return act(ctx)
}

func (a allowance) Snapshot(context.Context, uuid.UUID) ([]domain.QuotaStatus, error) {
	return nil, nil
}

type ledger struct {
	tasks []domain.AITask
	spent int64
	since time.Time
}

func (l *ledger) Record(_ context.Context, task *domain.AITask) error {
	l.tasks = append(l.tasks, *task)
	return nil
}

func (l *ledger) SpentSince(_ context.Context, since time.Time) (int64, error) {
	l.since = since
	return l.spent, nil
}

func (l *ledger) last(t *testing.T) domain.AITask {
	t.Helper()
	if len(l.tasks) == 0 {
		t.Fatal("nothing was recorded")
	}
	return l.tasks[len(l.tasks)-1]
}

// jsonCache stores values the way Redis would: serialised.
type jsonCache map[string][]byte

func (c jsonCache) Get(_ context.Context, key string, dest any) error {
	raw, ok := c[key]
	if !ok {
		return errors.New("miss")
	}
	return json.Unmarshal(raw, dest)
}

func (c jsonCache) Set(_ context.Context, key string, value any, _ time.Duration) error {
	raw, err := json.Marshal(value)
	if err != nil {
		return err
	}
	c[key] = raw
	return nil
}

type pricedExtractor struct {
	err   error
	calls int
}

func (p *pricedExtractor) Extract(context.Context, domain.AdExtractRequest) (domain.AdExtractResult, error) {
	p.calls++
	if p.err != nil {
		return domain.AdExtractResult{}, p.err
	}
	return domain.AdExtractResult{
		Fields:   map[string]domain.AdExtractedValue{domain.FieldOrigin: {Value: "Карши", Evidence: "Кашри"}},
		Provider: "llm", Model: "model-2026", TokensIn: 1000, TokensOut: 200,
	}, nil
}

var meterConfig = config.AIConfig{
	Provider: "llm", Model: "model", CacheTTL: time.Hour,
	PriceInPerMTok: 3, PriceOutPerMTok: 15,
}

var question = domain.AdExtractRequest{Text: "Кашри → Москва", Missing: []string{"origin"}, Unresolved: []string{"Кашри"}}

func charged(companyID, userID uuid.UUID) context.Context {
	return domain.WithActor(context.Background(), domain.Actor{CompanyID: companyID, UserID: userID})
}

func TestACallIsChargedWithWhatItCost(t *testing.T) {
	inner, book := &pricedExtractor{}, &ledger{}
	meter := ai.NewMetered(inner, allowance{limit: 10}, book, jsonCache{}, meterConfig, zone, logger.New("test", "error"))
	companyID, userID := uuid.New(), uuid.New()

	result, err := meter.Extract(charged(companyID, userID), question)
	if err != nil {
		t.Fatalf("Extract: %v", err)
	}
	if len(result.Fields) != 1 {
		t.Errorf("fields = %v", result.Fields)
	}

	task := book.last(t)
	if task.CompanyID != companyID || task.UserID == nil || *task.UserID != userID {
		t.Errorf("charged to %v / %v", task.CompanyID, task.UserID)
	}
	if task.Status != domain.AITaskOK || task.CacheHit {
		t.Errorf("task = %+v", task)
	}
	// 1000 tokens at $3 and 200 at $15 per million: $0.006.
	if task.CostMicros != 6000 || task.TokensIn != 1000 || task.TokensOut != 200 {
		t.Errorf("cost %d micros for %d/%d tokens, want 6000", task.CostMicros, task.TokensIn, task.TokensOut)
	}
	if task.Model != "model-2026" || len(task.InputHash) != 64 {
		t.Errorf("model %q, hash %q", task.Model, task.InputHash)
	}
}

func TestTheSameQuestionIsAnsweredFromCacheForNothing(t *testing.T) {
	inner, book := &pricedExtractor{}, &ledger{}
	meter := ai.NewMetered(inner, allowance{limit: 10}, book, jsonCache{}, meterConfig, zone, logger.New("test", "error"))
	ctx := charged(uuid.New(), uuid.New())

	if _, err := meter.Extract(ctx, question); err != nil {
		t.Fatalf("first: %v", err)
	}
	again, err := meter.Extract(ctx, question)
	if err != nil {
		t.Fatalf("second: %v", err)
	}
	if inner.calls != 1 {
		t.Errorf("the model was called %d times for one question", inner.calls)
	}
	if len(again.Fields) != 1 {
		t.Errorf("the cached answer lost its fields: %+v", again)
	}
	hit := book.last(t)
	if !hit.CacheHit || hit.CostMicros != 0 || hit.TokensIn != 0 {
		t.Errorf("cache hit recorded as %+v, want free", hit)
	}
}

func TestAnExhaustedPlanRefusesBeforeTheModelIsCalled(t *testing.T) {
	inner, book := &pricedExtractor{}, &ledger{}
	meter := ai.NewMetered(inner, allowance{limit: 5, used: 5}, book, nil, meterConfig, zone, logger.New("test", "error"))

	_, err := meter.Extract(charged(uuid.New(), uuid.New()), question)
	if !errors.Is(err, domain.ErrAIQuotaExhausted) {
		t.Fatalf("err = %v, want the plan limit", err)
	}
	if inner.calls != 0 {
		t.Error("the model was called past the plan")
	}
	task := book.last(t)
	if task.Status != domain.AITaskRejected || task.RejectReason == nil || *task.RejectReason != domain.AIRejectQuota {
		t.Errorf("refusal recorded as %+v", task)
	}
}

func TestTheDailyBudgetStopsEveryCompany(t *testing.T) {
	inner := &pricedExtractor{}
	book := &ledger{spent: 2_000_000}
	cfg := meterConfig
	cfg.DailyBudgetUSD = 2
	meter := ai.NewMetered(inner, allowance{limit: domain.Unlimited}, book, nil, cfg, zone, logger.New("test", "error"))

	_, err := meter.Extract(charged(uuid.New(), uuid.New()), question)
	if !errors.Is(err, domain.ErrAIBudgetExhausted) {
		t.Fatalf("err = %v, want the budget brake", err)
	}
	if inner.calls != 0 {
		t.Error("the model was called past the budget")
	}
	if h, m, s := book.since.In(zone).Clock(); h != 0 || m != 0 || s != 0 {
		t.Errorf("spending counted since %v, want local midnight", book.since)
	}
	if reason := book.last(t).RejectReason; reason == nil || *reason != domain.AIRejectBudget {
		t.Errorf("reason = %v", reason)
	}
}

func TestAFailedCallIsRecordedAndNotCached(t *testing.T) {
	inner, book := &pricedExtractor{err: errors.New("timeout")}, &ledger{}
	store := jsonCache{}
	meter := ai.NewMetered(inner, allowance{limit: 10}, book, store, meterConfig, zone, logger.New("test", "error"))

	if _, err := meter.Extract(charged(uuid.New(), uuid.New()), question); err == nil {
		t.Fatal("the provider's failure was swallowed")
	}
	if book.last(t).Status != domain.AITaskFailed {
		t.Errorf("status = %s, want failed", book.last(t).Status)
	}
	if len(store) != 0 {
		t.Error("a failure was cached")
	}
}

func TestACallNobodyCanBeChargedForIsNotMade(t *testing.T) {
	inner, book := &pricedExtractor{}, &ledger{}
	meter := ai.NewMetered(inner, allowance{limit: 10}, book, nil, meterConfig, zone, logger.New("test", "error"))

	if _, err := meter.Extract(context.Background(), question); err == nil {
		t.Fatal("a call with no company in context went through")
	}
	if inner.calls != 0 || len(book.tasks) != 0 {
		t.Errorf("calls %d, records %d; want neither", inner.calls, len(book.tasks))
	}
}
