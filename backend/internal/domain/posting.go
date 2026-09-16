package domain

import (
	"context"
	"time"

	"github.com/google/uuid"
)

// PostingJobStatus is where one message to one group stands.
type PostingJobStatus string

// Job states. The set is deliberately small: every reason a post did not go out
// is either "not yet" (rate_limited), "not at all" (skipped, failed) or "not
// any more" (cancelled).
const (
	PostingPending     PostingJobStatus = "pending"
	PostingProcessing  PostingJobStatus = "processing"
	PostingSent        PostingJobStatus = "sent"
	PostingFailed      PostingJobStatus = "failed"
	PostingSkipped     PostingJobStatus = "skipped"
	PostingRateLimited PostingJobStatus = "rate_limited"
	PostingCancelled   PostingJobStatus = "cancelled"
)

// Terminal reports whether the job has finished for good.
func (s PostingJobStatus) Terminal() bool {
	switch s {
	case PostingSent, PostingFailed, PostingSkipped, PostingCancelled:
		return true
	default:
		return false
	}
}

// SkipReason is why the guard held a post back. The values match the spam_event
// kinds, because a skip and a spam event are the same fact recorded twice: once
// on the job, once in the log an operator reads.
type SkipReason string

// Reasons a post is not sent.
const (
	SkipDuplicate        SkipReason = "duplicate"
	SkipCooldown         SkipReason = "cooldown"
	SkipGroupLimit       SkipReason = "group_limit"
	SkipUserLimit        SkipReason = "user_limit"
	SkipAccountLimit     SkipReason = "account_limit"
	SkipPlanLimit        SkipReason = "plan_limit"
	SkipQuietHours       SkipReason = "quiet_hours"
	SkipStale            SkipReason = "stale"
	SkipGroupUnavailable SkipReason = "group_unavailable"
)

// PublishRun is one press of Publish: the advert, the account it goes out from,
// and what the guard decided at that moment.
type PublishRun struct {
	ID                uuid.UUID `gorm:"type:uuid;primaryKey;default:gen_random_uuid()"`
	CompanyID         uuid.UUID `gorm:"type:uuid;not null;index"`
	LoadAdID          uuid.UUID `gorm:"type:uuid;not null;index"`
	AccountID         uuid.UUID `gorm:"type:uuid;not null"`
	RequestedByUserID uuid.UUID `gorm:"type:uuid;not null"`
	IdempotencyKey    string    `gorm:"size:128;not null"`
	Candidates        int       `gorm:"not null;default:0"`
	Approved          int       `gorm:"not null;default:0"`
	Skipped           int       `gorm:"not null;default:0"`
	CreatedAt         time.Time
}

// TableName pins the run table.
func (PublishRun) TableName() string { return "publish_runs" }

// PostingJob is one message to one group.
type PostingJob struct {
	ID            uuid.UUID `gorm:"type:uuid;primaryKey;default:gen_random_uuid()"`
	CompanyID     uuid.UUID `gorm:"type:uuid;not null;index"`
	RunID         uuid.UUID `gorm:"type:uuid;not null;index"`
	LoadAdID      uuid.UUID `gorm:"type:uuid;not null;index"`
	AccountID     uuid.UUID `gorm:"type:uuid;not null;index"`
	GroupID       uuid.UUID `gorm:"type:uuid;not null;index"`
	Fingerprint   string    `gorm:"type:char(64);not null"`
	MessageText   string    `gorm:"not null"`
	Status        PostingJobStatus
	SkipReason    *SkipReason `gorm:"size:32"`
	RandomID      int64       `gorm:"not null"`
	Attempt       int16       `gorm:"not null;default:0"`
	NextAttemptAt time.Time   `gorm:"not null;default:now()"`
	EnqueuedAt    *time.Time
	LeaseOwner    *string `gorm:"size:64"`
	LeaseUntil    *time.Time
	TgMessageID   *int64
	ErrorCode     *string `gorm:"size:48"`
	ErrorDetail   *string
	SentAt        *time.Time
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

// TableName pins the job table.
func (PostingJob) TableName() string { return "posting_jobs" }

// Claimable reports whether a worker may take this job at now.
func (j *PostingJob) Claimable(now time.Time) bool {
	switch j.Status {
	case PostingPending, PostingRateLimited:
		return !now.Before(j.NextAttemptAt)
	case PostingProcessing:
		// A lease that has run out means the worker holding it died mid-flight.
		return j.LeaseUntil != nil && now.After(*j.LeaseUntil)
	default:
		return false
	}
}

// Backoff is how long to wait before attempt n+1 of a transient failure. It
// grows geometrically from five seconds and stops at ten minutes; the caller
// adds jitter so a Telegram hiccup does not resynchronise every worker.
func Backoff(attempt int16) time.Duration {
	const base, ceiling = 5 * time.Second, 10 * time.Minute
	wait := base
	for range attempt {
		wait *= 2
		if wait >= ceiling {
			return ceiling
		}
	}
	return wait
}

// PostingEvent is one transition, appended and never edited. It is the answer
// to "what happened to this post", so it carries who, where, when and why.
type PostingEvent struct {
	ID         int64             `gorm:"primaryKey"`
	JobID      uuid.UUID         `gorm:"type:uuid;not null;index"`
	CompanyID  uuid.UUID         `gorm:"type:uuid;not null;index"`
	FromStatus *PostingJobStatus `gorm:"size:16"`
	ToStatus   PostingJobStatus  `gorm:"size:16;not null"`
	Attempt    int16             `gorm:"not null;default:0"`
	ErrorCode  *string           `gorm:"size:48"`
	Detail     *string
	// TelegramResponse is stored without access hashes, phone numbers or
	// anything from the session.
	TelegramResponse JSONMap `gorm:"type:jsonb"`
	CreatedAt        time.Time
}

// TableName pins the history table.
func (PostingEvent) TableName() string { return "posting_history" }

// SpamEvent records a post the platform held back, or a limit Telegram imposed.
type SpamEvent struct {
	ID          int64      `gorm:"primaryKey"`
	CompanyID   uuid.UUID  `gorm:"type:uuid;not null;index"`
	AccountID   *uuid.UUID `gorm:"type:uuid"`
	GroupID     *uuid.UUID `gorm:"type:uuid"`
	JobID       *uuid.UUID `gorm:"type:uuid"`
	Kind        string     `gorm:"size:24;not null"`
	WaitSeconds *int
	Detail      *string
	CreatedAt   time.Time
}

// TableName pins the spam event table.
func (SpamEvent) TableName() string { return "spam_events" }

// ─── the guard ──────────────────────────────────────────────────────

// GuardTarget is everything the guard needs about one intended post. The limits
// travel with the target rather than being looked up inside the guard, so the
// same rules can be evaluated for a preview and for a send without the guard
// needing a database.
type GuardTarget struct {
	CompanyID   uuid.UUID
	UserID      uuid.UUID
	AccountID   uuid.UUID
	GroupID     uuid.UUID
	PeerID      int64
	Fingerprint string

	Cooldown    time.Duration
	GroupHourly int
	GroupDaily  int
}

// GuardDecision is the answer for one group.
type GuardDecision struct {
	GroupID uuid.UUID
	Allowed bool
	Reason  SkipReason
	// RetryAfter is set when the answer is "not yet" rather than "no".
	RetryAfter time.Duration
}

// PostingGuard is the anti-spam gate. It runs twice: once when planning, where
// it only reads and explains, and once immediately before sending, where it
// consumes every limit atomically or none of them.
//
// The second run is the one that decides, because a job can sit in the queue
// long enough for the answer to change.
type PostingGuard interface {
	Plan(ctx context.Context, targets []GuardTarget, now time.Time) ([]GuardDecision, error)
	Reserve(ctx context.Context, target GuardTarget, now time.Time) (GuardDecision, error)
	// Release returns a reservation when Telegram proved the message never
	// went out. It is never called on an ambiguous failure: counting a post
	// that may have been sent is the safer error.
	Release(ctx context.Context, target GuardTarget) error
	PauseAccount(ctx context.Context, accountID uuid.UUID, until time.Time) error
	AccountPausedUntil(ctx context.Context, accountID uuid.UUID) (time.Time, bool, error)
}

// ─── ports ──────────────────────────────────────────────────────────

// JobOutcome is how a job ended, as the worker reports it.
type JobOutcome struct {
	Status        PostingJobStatus
	SkipReason    *SkipReason
	ErrorCode     *string
	Detail        *string
	TgMessageID   *int64
	NextAttemptAt *time.Time
	Response      JSONMap
}

// PostingJobFilter narrows the history listing.
type PostingJobFilter struct {
	Statuses  []PostingJobStatus
	AccountID *uuid.UUID
	GroupID   *uuid.UUID
	LoadAdID  *uuid.UUID
	UserID    *uuid.UUID
	From      *time.Time
	To        *time.Time
}

// PostingRepository persists runs, jobs and their history.
type PostingRepository interface {
	// CreateRun writes the run, its jobs and their first history rows in one
	// transaction. Nothing reaches the queue before that commit.
	CreateRun(ctx context.Context, run *PublishRun, jobs []PostingJob) error
	FindRunByIdempotencyKey(ctx context.Context, companyID uuid.UUID, key string) (*PublishRun, error)
	FindJob(ctx context.Context, companyID, id uuid.UUID) (*PostingJob, error)
	ListJobs(ctx context.Context, companyID uuid.UUID, filter PostingJobFilter, offset, limit int) ([]PostingJob, int64, error)

	// DueJobs returns jobs whose time has come and whose lease is free. The
	// relay hands them to the queue; the claim is what actually reserves one.
	DueJobs(ctx context.Context, limit int) ([]PostingJob, error)
	MarkEnqueued(ctx context.Context, ids []uuid.UUID, at time.Time) error
	// Claim moves one job to processing, but only if it is still claimable, so
	// two workers reading the same queue entry cannot both send.
	Claim(ctx context.Context, id uuid.UUID, owner string, lease time.Duration) (*PostingJob, error)
	Finish(ctx context.Context, id uuid.UUID, outcome JobOutcome) error

	Cancel(ctx context.Context, companyID, id uuid.UUID) (bool, error)
	CancelForLoad(ctx context.Context, companyID, loadAdID uuid.UUID, reason string) (int64, error)
	CancelForAccount(ctx context.Context, accountID uuid.UUID, reason string) (int64, error)

	AppendEvent(ctx context.Context, event *PostingEvent) error
	RecordSpamEvent(ctx context.Context, event *SpamEvent) error
	CountSentBetween(ctx context.Context, companyID uuid.UUID, from, to time.Time) (int64, error)
}

// JobQueue carries job ids from the relay to the workers. Postgres remains the
// record; this is only delivery, so losing the queue costs latency, not work.
type JobQueue interface {
	Publish(ctx context.Context, jobID uuid.UUID) error
	// Consume blocks until a job is available, the context ends, or the queue
	// hands back a job an earlier worker never acknowledged.
	Consume(ctx context.Context, handler func(context.Context, uuid.UUID) error) error
}
