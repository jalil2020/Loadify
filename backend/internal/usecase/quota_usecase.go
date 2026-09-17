package usecase

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/loadify/backend/internal/domain"
	"github.com/loadify/backend/pkg/apperror"
	"github.com/loadify/backend/pkg/logger"
)

const (
	// quotaLockTTL outlives any write a reservation wraps, so a lock is never
	// lost while its holder is still inserting.
	quotaLockTTL = 30 * time.Second
	// quotaLockWait is how long a second request waits for the first before
	// being told to try again.
	quotaLockWait  = 3 * time.Second
	quotaLockRetry = 100 * time.Millisecond
)

// QuotaLocker is the part of the cache that serialises reservations.
type QuotaLocker interface {
	Lock(ctx context.Context, key string, ttl time.Duration) (func(), bool, error)
}

// QuotaUsecase enforces the tariff: how many adverts, posts, model calls,
// accounts and groups a company's plan allows.
//
// Limits come from the plan the company is on, or the free company plan when
// it has none, and a limit a plan does not mention is no limit. Usage is
// counted from the records themselves, so nothing has to be handed back when a
// post is cancelled.
type QuotaUsecase struct {
	subscriptions domain.SubscriptionRepository
	usage         domain.UsageRepository
	locker        QuotaLocker
	location      *time.Location
	log           logger.Logger
	now           func() time.Time
}

var _ domain.QuotaGuard = (*QuotaUsecase)(nil)

// NewQuotaUsecase wires the tariff guard. location is the timezone a month
// starts in.
func NewQuotaUsecase(
	subscriptions domain.SubscriptionRepository,
	usage domain.UsageRepository,
	locker QuotaLocker,
	location *time.Location,
	log logger.Logger,
) *QuotaUsecase {
	if location == nil {
		location = time.UTC
	}
	return &QuotaUsecase{
		subscriptions: subscriptions, usage: usage, locker: locker,
		location: location, log: log, now: time.Now,
	}
}

// Snapshot is every metric's standing.
func (u *QuotaUsecase) Snapshot(ctx context.Context, companyID uuid.UUID) ([]domain.QuotaStatus, error) {
	plan, err := u.plan(ctx, companyID)
	if err != nil {
		return nil, err
	}
	from, to := domain.BillingMonth(u.now(), u.location)
	used, err := u.usage.Usage(ctx, companyID, from, to)
	if err != nil {
		return nil, err
	}

	out := make([]domain.QuotaStatus, 0, len(domain.QuotaMetrics))
	for _, metric := range domain.QuotaMetrics {
		status := domain.QuotaStatus{Metric: metric, Limit: limitOf(plan, metric), Used: used[metric]}
		if metric.Monthly() {
			status.ResetsAt = to
		}
		out = append(out, status)
	}
	return out, nil
}

// Status is one metric's standing.
func (u *QuotaUsecase) Status(ctx context.Context, companyID uuid.UUID, metric domain.QuotaMetric) (domain.QuotaStatus, error) {
	all, err := u.Snapshot(ctx, companyID)
	if err != nil {
		return domain.QuotaStatus{}, err
	}
	for _, status := range all {
		if status.Metric == metric {
			return status, nil
		}
	}
	return domain.QuotaStatus{}, fmt.Errorf("quota: unknown metric %q", metric)
}

// Reserve runs act when n more fit, holding the company's lock for the metric.
func (u *QuotaUsecase) Reserve(
	ctx context.Context,
	companyID uuid.UUID,
	metric domain.QuotaMetric,
	n int,
	act func(context.Context) error,
) error {
	if n <= 0 {
		return act(ctx)
	}

	release, err := u.lock(ctx, companyID, metric)
	if err != nil {
		return err
	}
	defer release()

	status, err := u.Status(ctx, companyID, metric)
	if err != nil {
		return err
	}
	if status.Exceeded(n) {
		return quotaExceeded(status, n, u.location)
	}
	return act(ctx)
}

// lock waits briefly for the metric's lock. When the lock store itself is
// down the check still runs without it: the worst case is one request's worth
// over the allowance, which is a billing question, not a safety one — the
// posting guard enforces Telegram's limits on its own.
func (u *QuotaUsecase) lock(ctx context.Context, companyID uuid.UUID, metric domain.QuotaMetric) (func(), error) {
	key := "quota:" + string(metric) + ":" + companyID.String()
	deadline := u.now().Add(quotaLockWait)
	for {
		release, ok, err := u.locker.Lock(ctx, key, quotaLockTTL)
		if err != nil {
			u.log.Warn("quota lock unavailable, checking without it",
				logger.String("metric", string(metric)), logger.Err(err))
			return func() {}, nil
		}
		if ok {
			return release, nil
		}
		if u.now().After(deadline) {
			return nil, apperror.ErrRateLimited.
				WithMessage("Another request is using this allowance right now. Try again in a moment.")
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(quotaLockRetry):
		}
	}
}

// plan is the company's current plan, or the free one. A platform with no free
// company plan configured has no limits to enforce, and says so in the log
// rather than refusing everything.
func (u *QuotaUsecase) plan(ctx context.Context, companyID uuid.UUID) (*domain.SubscriptionPlan, error) {
	subscription, err := u.subscriptions.ActiveForCompany(ctx, companyID)
	if err != nil && !apperror.Is(err, apperror.ErrNotFound.Code) {
		return nil, err
	}
	if plan := subscriptionPlan(subscription); plan != nil {
		return plan, nil
	}

	plan, err := u.subscriptions.DefaultPlan(ctx, "company")
	if apperror.Is(err, apperror.ErrNotFound.Code) {
		u.log.Warn("no default company plan configured, quotas are not enforced")
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return plan, nil
}

// limitOf reads one limit; anything negative means no ceiling.
func limitOf(plan *domain.SubscriptionPlan, metric domain.QuotaMetric) int {
	if plan == nil {
		return domain.Unlimited
	}
	limit := plan.Limit(string(metric))
	if limit < 0 {
		return domain.Unlimited
	}
	return int(limit)
}

// quotaExceeded explains the refusal in terms of what the person was doing.
func quotaExceeded(status domain.QuotaStatus, n int, location *time.Location) error {
	resets := status.ResetsAt.In(location).Format("2 January")
	var message string
	switch status.Metric {
	case domain.QuotaLoadsPerMonth:
		message = fmt.Sprintf("Your plan allows %d adverts a month. It starts again on %s, or upgrade for more.",
			status.Limit, resets)
	case domain.QuotaPostsPerMonth:
		message = fmt.Sprintf("This would send %d posts and your plan has %d of %d left this month. "+
			"Choose fewer groups, wait until %s, or upgrade.",
			n, status.Remaining(), status.Limit, resets)
	case domain.QuotaAIRequests:
		message = fmt.Sprintf("Your plan's %d AI readings this month are used up. It starts again on %s.",
			status.Limit, resets)
	case domain.QuotaTelegramAccounts:
		message = fmt.Sprintf("Your plan allows %d connected Telegram accounts. Disconnect one or upgrade.",
			status.Limit)
	case domain.QuotaTelegramGroups:
		message = fmt.Sprintf("Your plan allows %d connected groups. Remove one or upgrade.", status.Limit)
	default:
		message = "Your plan limit has been reached."
	}
	return apperror.ErrQuotaExceeded.WithMessage(message)
}

// reserve is Reserve for callers whose guard is optional.
func reserve(
	ctx context.Context,
	quota domain.QuotaGuard,
	companyID uuid.UUID,
	metric domain.QuotaMetric,
	n int,
	act func(context.Context) error,
) error {
	if quota == nil {
		return act(ctx)
	}
	return quota.Reserve(ctx, companyID, metric, n, act)
}
