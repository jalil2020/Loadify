package repository

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/loadify/backend/internal/domain"
)

type usageRepository struct{ Base }

// NewUsageRepository builds the usage counter.
func NewUsageRepository(db *gorm.DB) domain.UsageRepository {
	return &usageRepository{Base: NewBase(db)}
}

// usageSQL counts every metric in one round trip.
//
// Each count says what "used" means for that metric:
//   - an advert counts once it is saved, archived or not, so archiving does not
//     hand the allowance back;
//   - a post counts while it is waiting or once it went out. A post that was
//     skipped, cancelled or failed never reached a group and is not counted;
//   - a model call counts when it was made. An answer from cache and a call
//     that was refused cost nothing;
//   - an account counts while it can be used, and a group while it and its
//     account are connected.
const usageSQL = `
SELECT
    (SELECT count(*) FROM load_ads
      WHERE company_id = @company AND created_at >= @from AND created_at < @to) AS loads,
    (SELECT count(*) FROM posting_jobs
      WHERE company_id = @company AND created_at >= @from AND created_at < @to
        AND status IN ('pending', 'processing', 'sent', 'rate_limited')) AS posts,
    (SELECT count(*) FROM ai_tasks
      WHERE company_id = @company AND created_at >= @from AND created_at < @to
        AND NOT cache_hit AND status <> 'rejected') AS ai_requests,
    (SELECT count(*) FROM telegram_accounts
      WHERE company_id = @company AND deleted_at IS NULL
        AND status IN ('active', 'paused', 'restricted')) AS tg_accounts,
    (SELECT count(*) FROM telegram_groups g
       JOIN telegram_accounts a ON a.id = g.account_id AND a.deleted_at IS NULL
      WHERE g.company_id = @company AND g.deleted_at IS NULL) AS tg_groups`

type usageRow struct {
	Loads      int
	Posts      int
	AIRequests int `gorm:"column:ai_requests"`
	TgAccounts int `gorm:"column:tg_accounts"`
	TgGroups   int `gorm:"column:tg_groups"`
}

func (r *usageRepository) Usage(
	ctx context.Context,
	companyID uuid.UUID,
	from, to time.Time,
) (map[domain.QuotaMetric]int, error) {
	var row usageRow
	err := r.DB(ctx).Raw(usageSQL, map[string]any{
		"company": companyID,
		"from":    from,
		"to":      to,
	}).Scan(&row).Error
	if err != nil {
		return nil, fmt.Errorf("count usage: %w", err)
	}
	return map[domain.QuotaMetric]int{
		domain.QuotaLoadsPerMonth:    row.Loads,
		domain.QuotaPostsPerMonth:    row.Posts,
		domain.QuotaAIRequests:       row.AIRequests,
		domain.QuotaTelegramAccounts: row.TgAccounts,
		domain.QuotaTelegramGroups:   row.TgGroups,
	}, nil
}

type aiTaskRepository struct{ Base }

// NewAITaskRepository builds the model-call ledger.
func NewAITaskRepository(db *gorm.DB) domain.AITaskRepository {
	return &aiTaskRepository{Base: NewBase(db)}
}

func (r *aiTaskRepository) Record(ctx context.Context, task *domain.AITask) error {
	if err := r.DB(ctx).Create(task).Error; err != nil {
		return translate(err, "ai task")
	}
	return nil
}

func (r *aiTaskRepository) SpentSince(ctx context.Context, since time.Time) (int64, error) {
	var spent int64
	err := r.DB(ctx).Model(&domain.AITask{}).
		Where("created_at >= ?", since).
		Select("COALESCE(SUM(cost_micros), 0)").
		Scan(&spent).Error
	if err != nil {
		return 0, fmt.Errorf("sum ai spending: %w", err)
	}
	return spent, nil
}
