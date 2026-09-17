package domain

import (
	"context"
	"time"

	"github.com/google/uuid"
)

// DistributionScope is whose numbers are read and for which day and month.
type DistributionScope struct {
	CompanyID uuid.UUID
	// UserID narrows everything to one person: their adverts, the posts sent
	// from their own accounts, their model calls. Nil is the whole company.
	UserID     *uuid.UUID
	DayStart   time.Time
	DayEnd     time.Time
	MonthStart time.Time
	MonthEnd   time.Time
}

// DistributionProblem is one recent post that did not go out.
type DistributionProblem struct {
	JobID      uuid.UUID
	GroupID    uuid.UUID
	GroupTitle string
	Status     PostingJobStatus
	SkipReason *string
	ErrorCode  *string
	At         time.Time
}

// DistributionStats is the distribution picture for a day and its month.
//
// Posts are counted by when they were queued, so "today" is what was published
// today and how it went, rather than a mix of today's sends and yesterday's
// stragglers.
type DistributionStats struct {
	LoadsToday int
	LoadsMonth int

	PostsToday map[PostingJobStatus]int
	PostsMonth map[PostingJobStatus]int
	LastSentAt *time.Time

	// GuardToday is why the anti-spam guard held posts back today, by kind.
	GuardToday map[string]int

	Accounts        map[TelegramAccountStatus]int
	GroupsConnected int
	GroupsPostable  int

	AIRequestsToday   int
	AIRequestsMonth   int
	AICacheHitsMonth  int
	AICostMonthMicros int64

	RecentProblems []DistributionProblem
}

// DistributionStatsRepository reads the aggregates.
type DistributionStatsRepository interface {
	Stats(ctx context.Context, scope DistributionScope) (*DistributionStats, error)
}
