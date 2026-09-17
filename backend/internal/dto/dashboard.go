package dto

import (
	"time"

	"github.com/google/uuid"
)

// Dashboard scopes.
const (
	DashboardScopeMine    = "mine"
	DashboardScopeCompany = "company"
)

// QuotaItem is one tariff allowance. Limit and Remaining are null when the plan
// sets no ceiling.
type QuotaItem struct {
	Metric    string     `json:"metric"`
	Limit     *int       `json:"limit"`
	Used      int        `json:"used"`
	Remaining *int       `json:"remaining"`
	ResetsAt  *time.Time `json:"resets_at,omitempty"`
}

// UsageResponse is what the company's plan allows and what is left.
type UsageResponse struct {
	PeriodStart time.Time   `json:"period_start"`
	PeriodEnd   time.Time   `json:"period_end"`
	Items       []QuotaItem `json:"items"`
}

// PostCounts breaks posts down by how they ended.
type PostCounts struct {
	Total int `json:"total"`
	// Queued is waiting, being sent, or waiting again after Telegram asked for
	// a pause.
	Queued    int `json:"queued"`
	Sent      int `json:"sent"`
	Failed    int `json:"failed"`
	Skipped   int `json:"skipped"`
	Cancelled int `json:"cancelled"`
}

// DashboardProblem is a recent post that did not go out, and why.
type DashboardProblem struct {
	JobID      uuid.UUID `json:"job_id"`
	GroupID    uuid.UUID `json:"group_id"`
	GroupTitle string    `json:"group_title"`
	Status     string    `json:"status"`
	Reason     string    `json:"reason"`
	At         time.Time `json:"at"`
}

// DashboardResponse is the distribution picture for today and this month.
type DashboardResponse struct {
	Scope    string `json:"scope"`
	Day      string `json:"day"`
	Timezone string `json:"timezone"`

	Loads struct {
		Today int `json:"today"`
		Month int `json:"month"`
	} `json:"loads"`

	Posts struct {
		Today PostCounts `json:"today"`
		Month PostCounts `json:"month"`
		// SuccessRate is sent over sent plus failed today, or null before
		// anything finished. Skips are the guard working, not failures.
		SuccessRate *float64   `json:"success_rate"`
		LastSentAt  *time.Time `json:"last_sent_at"`
	} `json:"posts"`

	// GuardToday is why posts were held back today, by reason.
	GuardToday map[string]int `json:"guard_today"`

	Accounts struct {
		Total    int            `json:"total"`
		ByStatus map[string]int `json:"by_status"`
	} `json:"accounts"`

	Groups struct {
		Connected int `json:"connected"`
		Postable  int `json:"postable"`
	} `json:"groups"`

	AI struct {
		Enabled         bool    `json:"enabled"`
		RequestsToday   int     `json:"requests_today"`
		RequestsMonth   int     `json:"requests_month"`
		CacheHitsMonth  int     `json:"cache_hits_month"`
		CostMonthUSD    float64 `json:"cost_month_usd"`
		CostMonthMicros int64   `json:"cost_month_micros"`
	} `json:"ai"`

	// Quota is the company's allowance, whichever scope was asked for: the
	// plan belongs to the company.
	Quota []QuotaItem `json:"quota"`

	RecentProblems []DashboardProblem `json:"recent_problems"`
}
