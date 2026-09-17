package dto

import (
	"time"

	"github.com/google/uuid"
)

// PublishPreviewQuery names the account to post from, when a person has more
// than one connected.
type PublishPreviewQuery struct {
	AccountID *uuid.UUID `query:"account_id"`
}

// GroupDecision is one group and what the platform decided about it.
//
// Approved and skipped groups share a shape because the interface shows them in
// one list: the point of the preview is that a person sees the whole reasoning,
// not only the part that went their way.
type GroupDecision struct {
	GroupID uuid.UUID `json:"group_id"`
	Title   string    `json:"title"`
	// Score and Reasons are why this group ranked where it did.
	Score   int      `json:"score,omitempty"`
	Reasons []string `json:"reasons,omitempty"`
	// Reason is why it was held back, when it was.
	Reason string `json:"reason,omitempty"`
	// RetryAfterSeconds turns "no" into "not yet" wherever that is the truth.
	RetryAfterSeconds int `json:"retry_after_seconds,omitempty"`
}

// PublishPreviewResponse is the answer a person reads before anything is sent.
type PublishPreviewResponse struct {
	AccountID uuid.UUID `json:"account_id"`
	// Candidates is how many groups the filter found before the guard ran, so
	// the funnel is visible: 100 groups, 32 candidates, 18 approved.
	Candidates int             `json:"candidates"`
	Approved   []GroupDecision `json:"approved"`
	Skipped    []GroupDecision `json:"skipped"`
	// MessagePreview is exactly what a group will receive.
	MessagePreview string `json:"message_preview"`
	// PreviewToken ties a publish to the list that was actually reviewed.
	PreviewToken string    `json:"preview_token"`
	ExpiresAt    time.Time `json:"expires_at"`
	// PostsLeft is how many posts the plan still allows this month, or null
	// when it sets no limit. Publishing more approved groups than this is
	// refused, so the person can narrow the list first.
	PostsLeft *int `json:"posts_left"`
}

// PublishRequest sends the advert to the groups the person just reviewed.
type PublishRequest struct {
	PreviewToken string `json:"preview_token" validate:"required"`
	// GroupIDs narrows the approved list. Omitted means all of it; a group that
	// was not in the preview is never added by this field.
	GroupIDs []uuid.UUID `json:"group_ids" validate:"omitempty,max=200,dive,required"`
}

// PublishResponse is what was queued.
type PublishResponse struct {
	RunID     uuid.UUID           `json:"run_id"`
	LoadAdID  uuid.UUID           `json:"load_ad_id"`
	AccountID uuid.UUID           `json:"account_id"`
	Queued    int                 `json:"queued"`
	Skipped   int                 `json:"skipped"`
	Jobs      []PostingJobSummary `json:"jobs"`
}

// PostingJobSummary is one message on its way to one group.
type PostingJobSummary struct {
	ID         uuid.UUID  `json:"id"`
	GroupID    uuid.UUID  `json:"group_id"`
	GroupTitle string     `json:"group_title,omitempty"`
	Status     string     `json:"status"`
	SkipReason string     `json:"skip_reason,omitempty"`
	Attempt    int        `json:"attempt"`
	ErrorCode  string     `json:"error_code,omitempty"`
	NextTryAt  *time.Time `json:"next_try_at,omitempty"`
	SentAt     *time.Time `json:"sent_at,omitempty"`
	CreatedAt  time.Time  `json:"created_at"`
}

// PostingHistoryQuery filters the posting history.
type PostingHistoryQuery struct {
	Status   string     `query:"status" validate:"omitempty,oneof=pending processing sent failed skipped rate_limited cancelled"`
	GroupID  *uuid.UUID `query:"group_id"`
	LoadAdID *uuid.UUID `query:"load_ad_id"`
	Mine     bool       `query:"mine"`
}
