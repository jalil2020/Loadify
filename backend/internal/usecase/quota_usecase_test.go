package usecase

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/loadify/backend/internal/domain"
	"github.com/loadify/backend/pkg/apperror"
	"github.com/loadify/backend/pkg/logger"
)

// planBook answers the two questions the quota guard asks of subscriptions.
// The embedded interface is nil: anything else it is asked panics, which is the
// point.
type planBook struct {
	domain.SubscriptionRepository
	active     *domain.Subscription
	defaultErr error
	fallback   *domain.SubscriptionPlan
}

func (p *planBook) ActiveForCompany(context.Context, uuid.UUID) (*domain.Subscription, error) {
	return p.active, nil
}

func (p *planBook) DefaultPlan(context.Context, string) (*domain.SubscriptionPlan, error) {
	if p.defaultErr != nil {
		return nil, p.defaultErr
	}
	return p.fallback, nil
}

type usageBook struct {
	used     map[domain.QuotaMetric]int
	from, to time.Time
}

func (u *usageBook) Usage(_ context.Context, _ uuid.UUID, from, to time.Time) (map[domain.QuotaMetric]int, error) {
	u.from, u.to = from, to
	out := make(map[domain.QuotaMetric]int, len(u.used))
	for k, v := range u.used {
		out[k] = v
	}
	return out, nil
}

// lockBook is a lock that can be held by someone else or be unreachable.
type lockBook struct {
	held     bool
	err      error
	taken    int
	released int
}

func (l *lockBook) Lock(context.Context, string, time.Duration) (func(), bool, error) {
	if l.err != nil {
		return nil, false, l.err
	}
	if l.held {
		return nil, false, nil
	}
	l.taken++
	return func() { l.released++ }, true, nil
}

func plan(limits map[string]any) *domain.SubscriptionPlan {
	return &domain.SubscriptionPlan{Code: "test", Limits: limits}
}

type quotaFixture struct {
	guard *QuotaUsecase
	plans *planBook
	usage *usageBook
	locks *lockBook
}

func newQuotaFixture(limits map[string]any) *quotaFixture {
	f := &quotaFixture{
		plans: &planBook{fallback: plan(limits)},
		usage: &usageBook{used: map[domain.QuotaMetric]int{}},
		locks: &lockBook{},
	}
	tashkent := time.FixedZone("+05", 5*60*60)
	f.guard = NewQuotaUsecase(f.plans, f.usage, f.locks, tashkent, logger.New("test", "error"))
	// 23:30 UTC on 30 September is already 1 October in Tashkent.
	f.guard.now = func() time.Time { return time.Date(2026, 9, 30, 23, 30, 0, 0, time.UTC) }
	return f
}

func TestTheMonthStartsInTheCompanysTimezone(t *testing.T) {
	f := newQuotaFixture(map[string]any{"posts": float64(10)})

	status, err := f.guard.Status(context.Background(), uuid.New(), domain.QuotaPostsPerMonth)
	if err != nil {
		t.Fatalf("Status: %v", err)
	}
	if got := f.usage.from.Format(time.RFC3339); got != "2026-10-01T00:00:00+05:00" {
		t.Errorf("month starts %s, want 1 October in Tashkent", got)
	}
	if got := status.ResetsAt.Format(time.RFC3339); got != "2026-11-01T00:00:00+05:00" {
		t.Errorf("resets %s, want 1 November", got)
	}
}

func TestAStandingLimitNeverResets(t *testing.T) {
	f := newQuotaFixture(map[string]any{"tg_groups": float64(10)})

	status, err := f.guard.Status(context.Background(), uuid.New(), domain.QuotaTelegramGroups)
	if err != nil {
		t.Fatalf("Status: %v", err)
	}
	if !status.ResetsAt.IsZero() {
		t.Errorf("a group seat does not come back with the month, got ResetsAt %v", status.ResetsAt)
	}
}

func TestTheCompanysOwnPlanWinsOverTheFreeOne(t *testing.T) {
	f := newQuotaFixture(map[string]any{"posts": float64(10)})
	f.plans.active = &domain.Subscription{Plan: plan(map[string]any{"posts": float64(500)})}

	status, err := f.guard.Status(context.Background(), uuid.New(), domain.QuotaPostsPerMonth)
	if err != nil {
		t.Fatalf("Status: %v", err)
	}
	if status.Limit != 500 {
		t.Errorf("limit = %d, want the paid plan's 500", status.Limit)
	}
}

func TestALimitThePlanDoesNotNameIsNoLimit(t *testing.T) {
	f := newQuotaFixture(map[string]any{"posts": float64(10), "loads": float64(-1)})
	snapshot, err := f.guard.Snapshot(context.Background(), uuid.New())
	if err != nil {
		t.Fatalf("Snapshot: %v", err)
	}
	if len(snapshot) != len(domain.QuotaMetrics) {
		t.Fatalf("snapshot has %d metrics, want every one", len(snapshot))
	}
	for _, status := range snapshot {
		switch status.Metric {
		case domain.QuotaPostsPerMonth:
			if status.Unlimited() {
				t.Error("posts are limited to 10")
			}
		default:
			if !status.Unlimited() {
				t.Errorf("%s: limit %d, want none", status.Metric, status.Limit)
			}
		}
	}
}

func TestWithoutAFreePlanNothingIsLimited(t *testing.T) {
	f := newQuotaFixture(nil)
	f.plans.defaultErr = apperror.ErrNotFound

	status, err := f.guard.Status(context.Background(), uuid.New(), domain.QuotaPostsPerMonth)
	if err != nil {
		t.Fatalf("Status: %v", err)
	}
	if !status.Unlimited() {
		t.Errorf("limit = %d; a platform with no free plan has nothing to enforce", status.Limit)
	}
}

func TestAPlanLookupFailureIsNotMistakenForNoLimit(t *testing.T) {
	f := newQuotaFixture(nil)
	f.plans.defaultErr = errors.New("connection reset")

	if _, err := f.guard.Status(context.Background(), uuid.New(), domain.QuotaPostsPerMonth); err == nil {
		t.Fatal("a database failure must not read as an unlimited plan")
	}
}

func TestReserveActsWhenItFits(t *testing.T) {
	f := newQuotaFixture(map[string]any{"posts": float64(10)})
	f.usage.used[domain.QuotaPostsPerMonth] = 7

	acted := false
	err := f.guard.Reserve(context.Background(), uuid.New(), domain.QuotaPostsPerMonth, 3, func(context.Context) error {
		acted = true
		return nil
	})
	if err != nil {
		t.Fatalf("Reserve: %v", err)
	}
	if !acted {
		t.Error("7 of 10 used leaves room for 3")
	}
	if f.locks.taken != 1 || f.locks.released != 1 {
		t.Errorf("lock taken %d, released %d; want once each", f.locks.taken, f.locks.released)
	}
}

func TestReserveRefusesWhatWouldCrossTheLimit(t *testing.T) {
	f := newQuotaFixture(map[string]any{"posts": float64(10)})
	f.usage.used[domain.QuotaPostsPerMonth] = 7

	acted := false
	err := f.guard.Reserve(context.Background(), uuid.New(), domain.QuotaPostsPerMonth, 4, func(context.Context) error {
		acted = true
		return nil
	})
	wantCode(t, err, apperror.ErrQuotaExceeded.Code)
	if acted {
		t.Error("nothing may be created past the limit")
	}
	var appErr *apperror.Error
	if !errors.As(err, &appErr) || !strings.Contains(appErr.Message, "3 of 10 left") {
		t.Errorf("message should say what is left, got %v", err)
	}
	if f.locks.released != 1 {
		t.Error("a refusal must still release the lock")
	}
}

func TestReservingNothingNeedsNoCheck(t *testing.T) {
	f := newQuotaFixture(map[string]any{"posts": float64(0)})
	f.usage.used[domain.QuotaPostsPerMonth] = 5 // over, after a downgrade

	acted := false
	err := f.guard.Reserve(context.Background(), uuid.New(), domain.QuotaPostsPerMonth, 0, func(context.Context) error {
		acted = true
		return nil
	})
	if err != nil || !acted {
		t.Fatalf("a run where the guard skipped every group costs nothing and must still be written (err %v)", err)
	}
	if f.locks.taken != 0 {
		t.Error("no lock is needed for nothing")
	}
}

func TestABusyAllowanceAsksToTryAgain(t *testing.T) {
	f := newQuotaFixture(map[string]any{"posts": float64(10)})
	f.locks.held = true
	clock := time.Date(2026, 9, 17, 12, 0, 0, 0, time.UTC)
	f.guard.now = func() time.Time {
		clock = clock.Add(time.Second)
		return clock
	}

	err := f.guard.Reserve(context.Background(), uuid.New(), domain.QuotaPostsPerMonth, 1, func(context.Context) error {
		t.Fatal("acted without the lock")
		return nil
	})
	wantCode(t, err, apperror.ErrRateLimited.Code)
}

func TestAnUnreachableLockStillChecksTheLimit(t *testing.T) {
	f := newQuotaFixture(map[string]any{"posts": float64(1)})
	f.locks.err = errors.New("redis down")
	f.usage.used[domain.QuotaPostsPerMonth] = 1

	err := f.guard.Reserve(context.Background(), uuid.New(), domain.QuotaPostsPerMonth, 1, func(context.Context) error {
		t.Fatal("acted past the limit")
		return nil
	})
	wantCode(t, err, apperror.ErrQuotaExceeded.Code)
}

func TestAFailedActionIsReturnedAsIs(t *testing.T) {
	f := newQuotaFixture(map[string]any{"loads": float64(10)})
	boom := errors.New("insert failed")

	err := f.guard.Reserve(context.Background(), uuid.New(), domain.QuotaLoadsPerMonth, 1, func(context.Context) error {
		return boom
	})
	if !errors.Is(err, boom) {
		t.Fatalf("err = %v, want the action's own error", err)
	}
}

// ─── a guard for the other usecases' tests ─────────────────────────

// quotaStub is a tariff in memory. A successful reservation counts, the way the
// records it creates would.
type quotaStub struct {
	limits map[domain.QuotaMetric]int
	used   map[domain.QuotaMetric]int
}

func newQuotaStub(limits map[domain.QuotaMetric]int) *quotaStub {
	return &quotaStub{limits: limits, used: map[domain.QuotaMetric]int{}}
}

func (q *quotaStub) Status(_ context.Context, _ uuid.UUID, metric domain.QuotaMetric) (domain.QuotaStatus, error) {
	limit, ok := q.limits[metric]
	if !ok {
		limit = domain.Unlimited
	}
	return domain.QuotaStatus{Metric: metric, Limit: limit, Used: q.used[metric]}, nil
}

func (q *quotaStub) Reserve(ctx context.Context, companyID uuid.UUID, metric domain.QuotaMetric, n int, act func(context.Context) error) error {
	status, _ := q.Status(ctx, companyID, metric)
	if n > 0 && status.Exceeded(n) {
		return quotaExceeded(status, n, time.UTC)
	}
	if err := act(ctx); err != nil {
		return err
	}
	q.used[metric] += n
	return nil
}

func (q *quotaStub) Snapshot(ctx context.Context, companyID uuid.UUID) ([]domain.QuotaStatus, error) {
	out := make([]domain.QuotaStatus, 0, len(domain.QuotaMetrics))
	for _, metric := range domain.QuotaMetrics {
		status, _ := q.Status(ctx, companyID, metric)
		out = append(out, status)
	}
	return out, nil
}
