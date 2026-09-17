package usecase

import (
	"context"
	"math"
	"time"

	"github.com/google/uuid"

	"github.com/loadify/backend/internal/domain"
	"github.com/loadify/backend/internal/dto"
)

// DashboardUsecase reads the distribution picture: what was published today,
// how it went, why posts were held back, and what the plan still allows.
type DashboardUsecase struct {
	stats     domain.DistributionStatsRepository
	quota     domain.QuotaGuard
	aiEnabled bool
	location  *time.Location
	now       func() time.Time
}

// NewDashboardUsecase wires the dashboard. location is the timezone a day and a
// month start in.
func NewDashboardUsecase(
	stats domain.DistributionStatsRepository,
	quota domain.QuotaGuard,
	aiEnabled bool,
	location *time.Location,
) *DashboardUsecase {
	if location == nil {
		location = time.UTC
	}
	return &DashboardUsecase{
		stats: stats, quota: quota, aiEnabled: aiEnabled,
		location: location, now: time.Now,
	}
}

// Dashboard reads one person's numbers, or the whole company's.
func (u *DashboardUsecase) Dashboard(
	ctx context.Context,
	userID, companyID uuid.UUID,
	wholeCompany bool,
) (*dto.DashboardResponse, error) {
	now := u.now().In(u.location)
	dayStart := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, u.location)
	monthStart, monthEnd := domain.BillingMonth(now, u.location)

	scope := domain.DistributionScope{
		CompanyID:  companyID,
		DayStart:   dayStart,
		DayEnd:     dayStart.AddDate(0, 0, 1),
		MonthStart: monthStart,
		MonthEnd:   monthEnd,
	}
	scopeName := dto.DashboardScopeCompany
	if !wholeCompany {
		scope.UserID = &userID
		scopeName = dto.DashboardScopeMine
	}

	stats, err := u.stats.Stats(ctx, scope)
	if err != nil {
		return nil, err
	}
	quota, err := u.quota.Snapshot(ctx, companyID)
	if err != nil {
		return nil, err
	}

	res := &dto.DashboardResponse{
		Scope:          scopeName,
		Day:            dayStart.Format(time.DateOnly),
		Timezone:       u.location.String(),
		GuardToday:     stats.GuardToday,
		Quota:          quotaItems(quota),
		RecentProblems: make([]dto.DashboardProblem, 0, len(stats.RecentProblems)),
	}
	res.Loads.Today = stats.LoadsToday
	res.Loads.Month = stats.LoadsMonth

	res.Posts.Today = postCounts(stats.PostsToday)
	res.Posts.Month = postCounts(stats.PostsMonth)
	res.Posts.LastSentAt = stats.LastSentAt
	if finished := res.Posts.Today.Sent + res.Posts.Today.Failed; finished > 0 {
		rate := math.Round(float64(res.Posts.Today.Sent)/float64(finished)*1000) / 1000
		res.Posts.SuccessRate = &rate
	}

	res.Accounts.ByStatus = make(map[string]int, len(stats.Accounts))
	for status, n := range stats.Accounts {
		res.Accounts.ByStatus[string(status)] = n
		res.Accounts.Total += n
	}
	res.Groups.Connected = stats.GroupsConnected
	res.Groups.Postable = stats.GroupsPostable

	res.AI.Enabled = u.aiEnabled
	res.AI.RequestsToday = stats.AIRequestsToday
	res.AI.RequestsMonth = stats.AIRequestsMonth
	res.AI.CacheHitsMonth = stats.AICacheHitsMonth
	res.AI.CostMonthMicros = stats.AICostMonthMicros
	res.AI.CostMonthUSD = float64(stats.AICostMonthMicros) / 1e6

	for _, p := range stats.RecentProblems {
		res.RecentProblems = append(res.RecentProblems, dto.DashboardProblem{
			JobID:      p.JobID,
			GroupID:    p.GroupID,
			GroupTitle: p.GroupTitle,
			Status:     string(p.Status),
			Reason:     problemReason(p),
			At:         p.At,
		})
	}
	return res, nil
}

// Usage is the company's allowance for the current month.
func (u *DashboardUsecase) Usage(ctx context.Context, companyID uuid.UUID) (*dto.UsageResponse, error) {
	snapshot, err := u.quota.Snapshot(ctx, companyID)
	if err != nil {
		return nil, err
	}
	from, to := domain.BillingMonth(u.now(), u.location)
	return &dto.UsageResponse{PeriodStart: from, PeriodEnd: to, Items: quotaItems(snapshot)}, nil
}

func postCounts(byStatus map[domain.PostingJobStatus]int) dto.PostCounts {
	var c dto.PostCounts
	for status, n := range byStatus {
		c.Total += n
		switch status {
		case domain.PostingPending, domain.PostingProcessing, domain.PostingRateLimited:
			c.Queued += n
		case domain.PostingSent:
			c.Sent += n
		case domain.PostingFailed:
			c.Failed += n
		case domain.PostingSkipped:
			c.Skipped += n
		case domain.PostingCancelled:
			c.Cancelled += n
		}
	}
	return c
}

func quotaItems(snapshot []domain.QuotaStatus) []dto.QuotaItem {
	items := make([]dto.QuotaItem, 0, len(snapshot))
	for _, status := range snapshot {
		item := dto.QuotaItem{Metric: string(status.Metric), Used: status.Used}
		if !status.Unlimited() {
			limit, remaining := status.Limit, status.Remaining()
			item.Limit, item.Remaining = &limit, &remaining
		}
		if !status.ResetsAt.IsZero() {
			resets := status.ResetsAt
			item.ResetsAt = &resets
		}
		items = append(items, item)
	}
	return items
}

// problemReason is the skip reason for a post the guard held back and the
// error code for one Telegram refused.
func problemReason(p domain.DistributionProblem) string {
	if p.SkipReason != nil && *p.SkipReason != "" {
		return *p.SkipReason
	}
	if p.ErrorCode != nil {
		return *p.ErrorCode
	}
	return ""
}
