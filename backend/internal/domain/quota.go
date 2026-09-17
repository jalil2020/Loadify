package domain

import (
	"context"
	"time"

	"github.com/google/uuid"
)

// QuotaMetric is one thing a tariff limits. The values are the keys inside
// subscription_plans.limits, so changing an allowance needs no code.
type QuotaMetric string

// What the tariff counts.
const (
	// Standing counts: how many exist right now.
	QuotaTelegramAccounts QuotaMetric = "tg_accounts"
	QuotaTelegramGroups   QuotaMetric = "tg_groups"

	// Monthly counts: how many were made this calendar month.
	QuotaLoadsPerMonth QuotaMetric = "loads"
	QuotaPostsPerMonth QuotaMetric = "posts"
	QuotaAIRequests    QuotaMetric = "ai_requests"
)

// QuotaMetrics is every metric in the order a usage page lists them.
var QuotaMetrics = []QuotaMetric{
	QuotaLoadsPerMonth,
	QuotaPostsPerMonth,
	QuotaAIRequests,
	QuotaTelegramAccounts,
	QuotaTelegramGroups,
}

// Monthly reports whether the metric starts again each month. The others are
// a ceiling on what exists, and free up when something is removed.
func (m QuotaMetric) Monthly() bool {
	switch m {
	case QuotaLoadsPerMonth, QuotaPostsPerMonth, QuotaAIRequests:
		return true
	default:
		return false
	}
}

// Unlimited is the limit value that means no ceiling.
const Unlimited = -1

// BillingMonth is the calendar month containing t, in the company's timezone
// rather than UTC: a post at 02:00 on the first belongs to the month the person
// thinks it does.
func BillingMonth(t time.Time, loc *time.Location) (from, to time.Time) {
	if loc == nil {
		loc = time.UTC
	}
	local := t.In(loc)
	from = time.Date(local.Year(), local.Month(), 1, 0, 0, 0, 0, loc)
	return from, from.AddDate(0, 1, 0)
}

// QuotaStatus is one metric's standing, shaped so the interface can say
// "18 of 50 posts this month, resets on 1 October" without a second query.
type QuotaStatus struct {
	Metric QuotaMetric
	Limit  int
	Used   int
	// ResetsAt is when a monthly allowance starts again, and zero for a
	// standing one.
	ResetsAt time.Time
}

// Unlimited reports whether the tariff sets no ceiling for this metric.
func (q QuotaStatus) Unlimited() bool { return q.Limit < 0 }

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

// UsageRepository counts what a company has used.
//
// Usage is read from the records themselves — adverts, posting jobs, accounts,
// groups, model calls — rather than from a tally kept beside them. A tally has
// to be taken back by hand every time something does not happen after all; a
// count of the records cannot drift, and a cancelled or skipped post frees its
// allowance by being cancelled or skipped.
type UsageRepository interface {
	// Usage returns every metric, with the monthly ones counted between from
	// and to.
	Usage(ctx context.Context, companyID uuid.UUID, from, to time.Time) (map[QuotaMetric]int, error)
}

// QuotaGuard answers "may this company do n more of these", and is the only
// thing that should be asked.
type QuotaGuard interface {
	// Status reads one metric without taking anything.
	Status(ctx context.Context, companyID uuid.UUID, metric QuotaMetric) (QuotaStatus, error)
	// Reserve runs act only when n more fit, and holds the company's lock for
	// that metric while it does, so two requests cannot both pass the check and
	// then both act. act is what creates the records that are then counted.
	Reserve(ctx context.Context, companyID uuid.UUID, metric QuotaMetric, n int, act func(context.Context) error) error
	// Snapshot is every metric, for the usage page and the dashboard.
	Snapshot(ctx context.Context, companyID uuid.UUID) ([]QuotaStatus, error)
}
