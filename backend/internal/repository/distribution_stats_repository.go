package repository

import (
	"context"
	"fmt"
	"time"

	"gorm.io/gorm"

	"github.com/loadify/backend/internal/domain"
)

type distributionStatsRepository struct{ Base }

// NewDistributionStatsRepository builds the dashboard reader.
func NewDistributionStatsRepository(db *gorm.DB) domain.DistributionStatsRepository {
	return &distributionStatsRepository{Base: NewBase(db)}
}

// Every query below takes the same arguments and filters to one person when
// @user is set. A person's posts are the ones sent from their own accounts,
// because that is whose name is on them in the group.

const statsLoadsSQL = `
SELECT count(*) FILTER (WHERE created_at >= @day_start AND created_at < @day_end) AS today,
       count(*) AS month
FROM load_ads
WHERE company_id = @company
  AND created_at >= @month_start AND created_at < @month_end
  AND (@user::uuid IS NULL OR created_by_user_id = @user::uuid)`

const statsPostsSQL = `
SELECT j.status,
       count(*) FILTER (WHERE j.created_at >= @day_start AND j.created_at < @day_end) AS today,
       count(*) AS month,
       max(j.sent_at) AS last_sent_at
FROM posting_jobs j
JOIN telegram_accounts a ON a.id = j.account_id
WHERE j.company_id = @company
  AND j.created_at >= @month_start AND j.created_at < @month_end
  AND (@user::uuid IS NULL OR a.user_id = @user::uuid)
GROUP BY j.status`

const statsGuardSQL = `
SELECT e.kind AS key, count(*) AS n
FROM spam_events e
LEFT JOIN telegram_accounts a ON a.id = e.account_id
WHERE e.company_id = @company
  AND e.created_at >= @day_start AND e.created_at < @day_end
  AND (@user::uuid IS NULL OR a.user_id = @user::uuid)
GROUP BY e.kind`

const statsAccountsSQL = `
SELECT status AS key, count(*) AS n
FROM telegram_accounts
WHERE company_id = @company
  AND deleted_at IS NULL
  AND status <> 'connecting'
  AND (@user::uuid IS NULL OR user_id = @user::uuid)
GROUP BY status`

const statsGroupsSQL = `
SELECT count(*) AS connected,
       count(*) FILTER (WHERE g.active AND g.posting_enabled AND g.can_send) AS postable
FROM telegram_groups g
JOIN telegram_accounts a ON a.id = g.account_id AND a.deleted_at IS NULL
WHERE g.company_id = @company
  AND g.deleted_at IS NULL
  AND (@user::uuid IS NULL OR a.user_id = @user::uuid)`

// A refused call cost nothing and is not a request; a cached answer is shown
// separately because it is the saving the cache exists for.
const statsAISQL = `
SELECT count(*) FILTER (WHERE created_at >= @day_start AND created_at < @day_end
                          AND NOT cache_hit AND status <> 'rejected') AS today,
       count(*) FILTER (WHERE NOT cache_hit AND status <> 'rejected') AS month,
       count(*) FILTER (WHERE cache_hit) AS cache_hits,
       COALESCE(sum(cost_micros), 0) AS cost_micros
FROM ai_tasks
WHERE company_id = @company
  AND created_at >= @month_start AND created_at < @month_end
  AND (@user::uuid IS NULL OR user_id = @user::uuid)`

const statsProblemsSQL = `
SELECT j.id AS job_id, j.group_id, g.title AS group_title, j.status,
       j.skip_reason, j.error_code, j.updated_at AS at
FROM posting_jobs j
JOIN telegram_groups g ON g.id = j.group_id
JOIN telegram_accounts a ON a.id = j.account_id
WHERE j.company_id = @company
  AND j.status IN ('failed', 'skipped')
  AND j.created_at >= @day_start AND j.created_at < @day_end
  AND (@user::uuid IS NULL OR a.user_id = @user::uuid)
ORDER BY j.updated_at DESC
LIMIT @limit`

// recentProblemsLimit keeps the dashboard a summary; the history page has the
// rest.
const recentProblemsLimit = 5

type statsCountRow struct {
	Key string
	N   int
}

type statsPostsRow struct {
	Status     string
	Today      int
	Month      int
	LastSentAt *time.Time
}

func (r *distributionStatsRepository) Stats(
	ctx context.Context,
	scope domain.DistributionScope,
) (*domain.DistributionStats, error) {
	args := map[string]any{
		"company":     scope.CompanyID,
		"user":        scope.UserID,
		"day_start":   scope.DayStart,
		"day_end":     scope.DayEnd,
		"month_start": scope.MonthStart,
		"month_end":   scope.MonthEnd,
		"limit":       recentProblemsLimit,
	}
	db := r.DB(ctx)
	stats := &domain.DistributionStats{
		PostsToday: map[domain.PostingJobStatus]int{},
		PostsMonth: map[domain.PostingJobStatus]int{},
		GuardToday: map[string]int{},
		Accounts:   map[domain.TelegramAccountStatus]int{},
	}

	var loads struct{ Today, Month int }
	if err := db.Raw(statsLoadsSQL, args).Scan(&loads).Error; err != nil {
		return nil, fmt.Errorf("count adverts: %w", err)
	}
	stats.LoadsToday, stats.LoadsMonth = loads.Today, loads.Month

	var posts []statsPostsRow
	if err := db.Raw(statsPostsSQL, args).Scan(&posts).Error; err != nil {
		return nil, fmt.Errorf("count posts: %w", err)
	}
	for _, row := range posts {
		status := domain.PostingJobStatus(row.Status)
		if row.Today > 0 {
			stats.PostsToday[status] = row.Today
		}
		stats.PostsMonth[status] = row.Month
		if row.LastSentAt != nil && (stats.LastSentAt == nil || row.LastSentAt.After(*stats.LastSentAt)) {
			stats.LastSentAt = row.LastSentAt
		}
	}

	var guard []statsCountRow
	if err := db.Raw(statsGuardSQL, args).Scan(&guard).Error; err != nil {
		return nil, fmt.Errorf("count guard decisions: %w", err)
	}
	for _, row := range guard {
		stats.GuardToday[row.Key] = row.N
	}

	var accounts []statsCountRow
	if err := db.Raw(statsAccountsSQL, args).Scan(&accounts).Error; err != nil {
		return nil, fmt.Errorf("count accounts: %w", err)
	}
	for _, row := range accounts {
		stats.Accounts[domain.TelegramAccountStatus(row.Key)] = row.N
	}

	var groups struct{ Connected, Postable int }
	if err := db.Raw(statsGroupsSQL, args).Scan(&groups).Error; err != nil {
		return nil, fmt.Errorf("count groups: %w", err)
	}
	stats.GroupsConnected, stats.GroupsPostable = groups.Connected, groups.Postable

	var ai struct {
		Today      int
		Month      int
		CacheHits  int
		CostMicros int64
	}
	if err := db.Raw(statsAISQL, args).Scan(&ai).Error; err != nil {
		return nil, fmt.Errorf("count model calls: %w", err)
	}
	stats.AIRequestsToday = ai.Today
	stats.AIRequestsMonth = ai.Month
	stats.AICacheHitsMonth = ai.CacheHits
	stats.AICostMonthMicros = ai.CostMicros

	if err := db.Raw(statsProblemsSQL, args).Scan(&stats.RecentProblems).Error; err != nil {
		return nil, fmt.Errorf("list recent problems: %w", err)
	}
	return stats, nil
}
