package repository

import (
	"context"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/loadify/backend/internal/domain"
	"github.com/loadify/backend/pkg/apperror"
)

type postingRepository struct{ Base }

// NewPostingRepository builds the posting repository.
func NewPostingRepository(db *gorm.DB) domain.PostingRepository {
	return &postingRepository{Base: NewBase(db)}
}

// CreateRun writes the run, its jobs and their first history rows together.
//
// One transaction, and nothing reaches the queue before it commits: the relay
// reads committed rows, so a crash between the two leaves work to be picked up
// rather than messages that were sent without a record of why.
func (r *postingRepository) CreateRun(ctx context.Context, run *domain.PublishRun, jobs []domain.PostingJob) error {
	return r.DB(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(run).Error; err != nil {
			if isUniqueViolation(err) {
				// The idempotency key already ran. The caller reads the earlier
				// run and returns it rather than posting the advert twice.
				return apperror.ErrConflict.
					WithMessage("This publish has already been requested.").Wrap(err)
			}
			return translate(err, "publish run")
		}
		if len(jobs) == 0 {
			return nil
		}

		if err := tx.Create(&jobs).Error; err != nil {
			if isUniqueViolation(err) {
				return apperror.ErrConflict.
					WithMessage("This advert is already queued for one of these groups.").Wrap(err)
			}
			return translate(err, "posting job")
		}

		events := make([]domain.PostingEvent, 0, len(jobs))
		for _, job := range jobs {
			events = append(events, domain.PostingEvent{
				JobID:     job.ID,
				CompanyID: job.CompanyID,
				ToStatus:  job.Status,
				Detail:    skipReasonDetail(job.SkipReason),
			})
		}
		if err := tx.Create(&events).Error; err != nil {
			return translate(err, "posting history")
		}
		return nil
	})
}

func skipReasonDetail(reason *domain.SkipReason) *string {
	if reason == nil {
		return nil
	}
	detail := string(*reason)
	return &detail
}

func (r *postingRepository) FindRunByIdempotencyKey(ctx context.Context, companyID uuid.UUID, key string) (*domain.PublishRun, error) {
	var run domain.PublishRun
	err := r.DB(ctx).Where("company_id = ? AND idempotency_key = ?", companyID, key).First(&run).Error
	if err != nil {
		return nil, translate(err, "publish run")
	}
	return &run, nil
}

func (r *postingRepository) FindJob(ctx context.Context, companyID, id uuid.UUID) (*domain.PostingJob, error) {
	var job domain.PostingJob
	err := r.DB(ctx).Where("id = ? AND company_id = ?", id, companyID).First(&job).Error
	if err != nil {
		return nil, translate(err, "posting job")
	}
	return &job, nil
}

func (r *postingRepository) ListJobs(
	ctx context.Context,
	companyID uuid.UUID,
	filter domain.PostingJobFilter,
	offset, limit int,
) ([]domain.PostingJob, int64, error) {
	q := r.DB(ctx).Model(&domain.PostingJob{}).Where("company_id = ?", companyID)

	if len(filter.Statuses) > 0 {
		q = q.Where("status IN ?", filter.Statuses)
	}
	if filter.AccountID != nil {
		q = q.Where("account_id = ?", *filter.AccountID)
	}
	if filter.GroupID != nil {
		q = q.Where("group_id = ?", *filter.GroupID)
	}
	if filter.LoadAdID != nil {
		q = q.Where("load_ad_id = ?", *filter.LoadAdID)
	}
	if filter.UserID != nil {
		// Whose post it was is a property of the run, not of the job.
		q = q.Where("run_id IN (?)", r.DB(ctx).Model(&domain.PublishRun{}).
			Select("id").Where("requested_by_user_id = ?", *filter.UserID))
	}
	if filter.From != nil {
		q = q.Where("created_at >= ?", *filter.From)
	}
	if filter.To != nil {
		q = q.Where("created_at < ?", *filter.To)
	}

	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, translate(err, "posting job")
	}

	var jobs []domain.PostingJob
	if err := applyPage(q.Order("created_at DESC"), offset, limit).Find(&jobs).Error; err != nil {
		return nil, 0, translate(err, "posting job")
	}
	return jobs, total, nil
}

// dueJobsSQL takes the jobs whose time has come and hands them to the queue.
//
// SKIP LOCKED is what lets several relays run at once without coordinating:
// each takes rows the others are not already holding. Marking enqueued_at in
// the same statement is what stops the same job being queued twice while it
// waits to be claimed.
const dueJobsSQL = `
UPDATE posting_jobs
SET enqueued_at = now()
WHERE id IN (
    SELECT id FROM posting_jobs
    WHERE status IN ('pending', 'rate_limited')
      AND next_attempt_at <= now()
      AND (enqueued_at IS NULL OR enqueued_at < now() - INTERVAL '5 minutes')
    ORDER BY next_attempt_at
    FOR UPDATE SKIP LOCKED
    LIMIT ?
)
RETURNING *`

func (r *postingRepository) DueJobs(ctx context.Context, limit int) ([]domain.PostingJob, error) {
	if limit <= 0 {
		limit = 100
	}
	var jobs []domain.PostingJob
	if err := r.DB(ctx).Raw(dueJobsSQL, limit).Scan(&jobs).Error; err != nil {
		return nil, translate(err, "posting job")
	}
	return jobs, nil
}

// Claim moves one job to processing, but only if it is still claimable.
//
// The condition is the whole guarantee: two workers reading the same queue
// entry both run this, and exactly one updates a row. A job whose lease has
// expired is claimable again, because the worker that held it is gone.
func (r *postingRepository) Claim(ctx context.Context, id uuid.UUID, owner string, lease time.Duration) (*domain.PostingJob, error) {
	var job domain.PostingJob

	res := r.DB(ctx).Model(&job).
		Clauses(clause.Returning{}).
		Where("id = ?", id).
		Where("(status IN ? AND next_attempt_at <= now()) OR (status = ? AND lease_until < now())",
			[]domain.PostingJobStatus{domain.PostingPending, domain.PostingRateLimited},
			domain.PostingProcessing).
		Updates(map[string]any{
			"status":      domain.PostingProcessing,
			"attempt":     gorm.Expr("attempt + 1"),
			"lease_owner": owner,
			"lease_until": time.Now().Add(lease),
		})
	if res.Error != nil {
		return nil, translate(res.Error, "posting job")
	}
	if res.RowsAffected == 0 {
		// Someone else has it, or it is already finished. Not an error: the
		// worker acknowledges the queue entry and moves on.
		return nil, nil
	}
	return &job, nil
}

// Finish records how a job ended, and appends the transition to its history.
func (r *postingRepository) Finish(ctx context.Context, id uuid.UUID, outcome domain.JobOutcome) error {
	return r.DB(ctx).Transaction(func(tx *gorm.DB) error {
		var job domain.PostingJob
		if err := tx.Where("id = ?", id).First(&job).Error; err != nil {
			return translate(err, "posting job")
		}

		columns := map[string]any{
			"status":       outcome.Status,
			"skip_reason":  outcome.SkipReason,
			"error_code":   outcome.ErrorCode,
			"error_detail": outcome.Detail,
			"lease_owner":  nil,
			"lease_until":  nil,
		}
		if outcome.TgMessageID != nil {
			columns["tg_message_id"] = *outcome.TgMessageID
		}
		if outcome.Status == domain.PostingSent {
			columns["sent_at"] = time.Now()
		}
		if outcome.NextAttemptAt != nil {
			columns["next_attempt_at"] = *outcome.NextAttemptAt
			// A job going back into the queue has not been handed to it yet.
			columns["enqueued_at"] = nil
		}

		if err := tx.Model(&domain.PostingJob{}).Where("id = ?", id).Updates(columns).Error; err != nil {
			return translate(err, "posting job")
		}

		from := job.Status
		event := domain.PostingEvent{
			JobID:            id,
			CompanyID:        job.CompanyID,
			FromStatus:       &from,
			ToStatus:         outcome.Status,
			Attempt:          job.Attempt,
			ErrorCode:        outcome.ErrorCode,
			Detail:           outcome.Detail,
			TelegramResponse: outcome.Response,
		}
		if err := tx.Create(&event).Error; err != nil {
			return translate(err, "posting history")
		}
		return nil
	})
}

func (r *postingRepository) Cancel(ctx context.Context, companyID, id uuid.UUID) (bool, error) {
	res := r.DB(ctx).Model(&domain.PostingJob{}).
		Where("id = ? AND company_id = ? AND status IN ?", id, companyID,
			[]domain.PostingJobStatus{domain.PostingPending, domain.PostingRateLimited}).
		Updates(map[string]any{"status": domain.PostingCancelled})
	if res.Error != nil {
		return false, translate(res.Error, "posting job")
	}
	return res.RowsAffected > 0, nil
}

// CancelForLoad stops everything still queued for one advert, which is what
// archiving it means: what has gone out has gone out, nothing further will.
func (r *postingRepository) CancelForLoad(ctx context.Context, companyID, loadAdID uuid.UUID, reason string) (int64, error) {
	return r.cancelWhere(ctx, reason, "company_id = ? AND load_ad_id = ?", companyID, loadAdID)
}

// CancelForAccount stops everything queued from one account: its session was
// revoked, or Telegram restricted it, and nothing may leave it now.
func (r *postingRepository) CancelForAccount(ctx context.Context, accountID uuid.UUID, reason string) (int64, error) {
	return r.cancelWhere(ctx, reason, "account_id = ?", accountID)
}

func (r *postingRepository) cancelWhere(ctx context.Context, reason, condition string, args ...any) (int64, error) {
	res := r.DB(ctx).Model(&domain.PostingJob{}).
		Where(condition, args...).
		Where("status IN ?", []domain.PostingJobStatus{domain.PostingPending, domain.PostingRateLimited}).
		Updates(map[string]any{
			"status":       domain.PostingCancelled,
			"error_detail": reason,
		})
	if res.Error != nil {
		return 0, translate(res.Error, "posting job")
	}
	return res.RowsAffected, nil
}

func (r *postingRepository) AppendEvent(ctx context.Context, event *domain.PostingEvent) error {
	if err := r.DB(ctx).Create(event).Error; err != nil {
		return translate(err, "posting history")
	}
	return nil
}

func (r *postingRepository) RecordSpamEvent(ctx context.Context, event *domain.SpamEvent) error {
	if err := r.DB(ctx).Create(event).Error; err != nil {
		return translate(err, "spam event")
	}
	return nil
}

func (r *postingRepository) CountSentBetween(ctx context.Context, companyID uuid.UUID, from, to time.Time) (int64, error) {
	var total int64
	err := r.DB(ctx).Model(&domain.PostingJob{}).
		Where("company_id = ? AND status = ? AND sent_at >= ? AND sent_at < ?",
			companyID, domain.PostingSent, from, to).
		Count(&total).Error
	if err != nil {
		return 0, translate(err, "posting job")
	}
	return total, nil
}
