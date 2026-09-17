package repository

import (
	"context"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

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

		// The run's id exists only now, so the jobs are tied to it here.
		for i := range jobs {
			jobs[i].RunID = run.ID
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

// QueuedGroups reads the same statuses the one-copy-per-group index covers, so
// what it reports is exactly what a second publish would collide with.
func (r *postingRepository) QueuedGroups(ctx context.Context, companyID, loadAdID uuid.UUID) (map[uuid.UUID]bool, error) {
	var ids []uuid.UUID
	err := r.DB(ctx).Model(&domain.PostingJob{}).
		Where("company_id = ? AND load_ad_id = ? AND status IN ?", companyID, loadAdID, []domain.PostingJobStatus{
			domain.PostingPending, domain.PostingProcessing, domain.PostingRateLimited,
		}).
		Pluck("group_id", &ids).Error
	if err != nil {
		return nil, translate(err, "posting job")
	}
	queued := make(map[uuid.UUID]bool, len(ids))
	for _, id := range ids {
		queued[id] = true
	}
	return queued, nil
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
	var jobs []domain.PostingJob

	err := r.DB(ctx).Raw(claimSQL, map[string]any{
		"id":    id,
		"owner": owner,
		"until": time.Now().Add(lease),
	}).Scan(&jobs).Error
	if err != nil {
		return nil, translate(err, "posting job")
	}
	if len(jobs) == 0 {
		// Someone else has it, or it is already finished. Not an error: the
		// worker acknowledges the queue entry and moves on.
		return nil, nil
	}
	return &jobs[0], nil
}

// claimSQL reports what the job was before it was claimed, alongside the row.
//
// Postgres cannot return an old value from an UPDATE directly, so the row is
// read and locked first. The previous status is the one fact the worker cannot
// reconstruct afterwards: "processing" means another worker died mid-send, and
// whether sending again is safe depends on it.
const claimSQL = `
WITH previous AS (
    SELECT id, status FROM posting_jobs WHERE id = @id FOR UPDATE
)
UPDATE posting_jobs j
SET status      = 'processing',
    attempt     = j.attempt + 1,
    lease_owner = @owner,
    lease_until = @until,
    updated_at  = now()
FROM previous p
WHERE j.id = p.id
  AND (
    (j.status IN ('pending', 'rate_limited') AND j.next_attempt_at <= now())
    OR (j.status = 'processing' AND j.lease_until < now())
  )
RETURNING j.*, p.status AS previous_status`

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
