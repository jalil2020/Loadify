package domain

import (
	"context"
	"time"

	"github.com/google/uuid"
)

// QuotaMetric is one thing a tariff limits. The values are the keys inside
// subscription_plans.limits, so adding a limit to a plan needs no migration.
type QuotaMetric string

// What the tariff counts.
const (
	QuotaTelegramAccounts QuotaMetric = "tg_accounts"
	QuotaTelegramGroups   QuotaMetric = "tg_groups"
	QuotaLoadsPerMonth    QuotaMetric = "loads"
	QuotaPostsPerMonth    QuotaMetric = "posts"
	QuotaAIRequests       QuotaMetric = "ai_requests"
	QuotaSearches         QuotaMetric = "searches"
	QuotaMembers          QuotaMetric = "users"
)

// Unlimited is the limit value that means no ceiling.
const Unlimited = -1

// BillingPeriod is the month a usage row belongs to, in the company's own
// timezone rather than UTC: a post at 02:00 on the first belongs to the month
// the person thinks it does.
func BillingPeriod(t time.Time, loc *time.Location) string {
	if loc == nil {
		loc = time.UTC
	}
	return t.In(loc).Format("2006-01")
}

// UsageCounter is the durable monthly tally. Redis holds the hot counters; this
// is what the tariff is enforced and invoiced against.
type UsageCounter struct {
	CompanyID uuid.UUID   `gorm:"type:uuid;primaryKey"`
	Period    string      `gorm:"type:char(7);primaryKey"`
	Metric    QuotaMetric `gorm:"size:32;primaryKey"`
	Used      int         `gorm:"not null;default:0"`
	UpdatedAt time.Time
}

// TableName pins the usage table.
func (UsageCounter) TableName() string { return "usage_counters" }

// QuotaStatus is one metric's standing, shaped so the interface can say
// "18 of 50 posts this month, resets on 1 October" without a second query.
type QuotaStatus struct {
	Metric   QuotaMetric
	Limit    int
	Used     int
	ResetsAt time.Time
}

// Unlimited reports whether the tariff sets no ceiling for this metric.
func (q QuotaStatus) Unlimited() bool { return q.Limit == Unlimited }

// Remaining is how much of the allowance is left, and is never negative.
func (q QuotaStatus) Remaining() int {
	if q.Unlimited() {
		return Unlimited
	}
	if q.Used >= q.Limit {
		return 0
	}
	return q.Limit - q.Used
}

// Exceeded reports whether n more of this metric would cross the ceiling.
func (q QuotaStatus) Exceeded(n int) bool {
	if q.Unlimited() {
		return false
	}
	return q.Used+n > q.Limit
}

// QuotaRepository persists the monthly tallies.
type QuotaRepository interface {
	// Add moves a counter and returns its new value. A negative delta is a
	// refund and never takes the counter below zero.
	Add(ctx context.Context, companyID uuid.UUID, period string, metric QuotaMetric, delta int) (int, error)
	Usage(ctx context.Context, companyID uuid.UUID, period string) (map[QuotaMetric]int, error)
}

// QuotaGuard answers "may this company do one more of these", and is the only
// thing that should be asked.
//
// Check reads, Consume reads and takes. Consume is called at the point of
// action so two requests cannot both pass a check and then both act.
type QuotaGuard interface {
	Check(ctx context.Context, companyID uuid.UUID, metric QuotaMetric, want int) (QuotaStatus, error)
	Consume(ctx context.Context, companyID uuid.UUID, metric QuotaMetric, n int) (QuotaStatus, error)
	// Refund returns an allowance when the action did not happen after all —
	// a post the guard skipped, an AI call that failed before the model ran.
	Refund(ctx context.Context, companyID uuid.UUID, metric QuotaMetric, n int) error
	Snapshot(ctx context.Context, companyID uuid.UUID) ([]QuotaStatus, error)
}
