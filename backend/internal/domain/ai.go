package domain

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
)

// AdExtractRequest is what a model is allowed to see: the advert the person
// typed, which fields the rules could not fill, and the words nothing
// understood.
//
// It carries no vocabulary and no identifiers. The model proposes words; the
// server decides whether those words name anything. That is what keeps an
// invented region id out of a search and an invented city out of a post.
type AdExtractRequest struct {
	Text       string
	Missing    []string
	Unresolved []string
}

// AdExtractedValue is one field a model proposed.
//
// Evidence is not decoration: a value whose evidence does not appear in the
// advert is discarded, which is the mechanism that stops a model from filling
// in a tonnage nobody wrote.
type AdExtractedValue struct {
	Value    string
	Evidence string
}

// AdExtractResult is a model's reading, plus what it cost.
type AdExtractResult struct {
	Fields    map[string]AdExtractedValue
	Provider  string
	Model     string
	TokensIn  int
	TokensOut int
}

// AdExtractor reads what the rules could not.
//
// It is optional everywhere: with no extractor configured the platform still
// parses, stores and posts adverts, and the fields the rules missed are simply
// reported as missing for a person to fill in.
type AdExtractor interface {
	Extract(ctx context.Context, req AdExtractRequest) (AdExtractResult, error)
}

// AI task kinds, as the ai_tasks table knows them.
const (
	AITaskParse = "parse"
)

// AI task outcomes.
const (
	AITaskOK       = "ok"
	AITaskRejected = "rejected"
	AITaskFailed   = "failed"
)

// Why a model call was refused before it was made.
const (
	AIRejectQuota  = "quota"
	AIRejectBudget = "budget"
	AIRejectActor  = "no_actor"
)

// AITask is one model call, or one refusal to make it, with what it cost.
//
// The input is kept only as a digest: the table exists to account for spending
// and to spot repeats, and an advert's text already lives with the advert.
type AITask struct {
	ID           uuid.UUID  `gorm:"type:uuid;primaryKey;default:gen_random_uuid()"`
	CompanyID    uuid.UUID  `gorm:"type:uuid;not null"`
	UserID       *uuid.UUID `gorm:"type:uuid"`
	Kind         string     `gorm:"size:16;not null"`
	Provider     string     `gorm:"size:16;not null"`
	Model        string     `gorm:"size:64;not null;default:''"`
	InputHash    string     `gorm:"type:char(64);not null"`
	CacheHit     bool       `gorm:"not null;default:false"`
	Status       string     `gorm:"size:16;not null;default:ok"`
	RejectReason *string    `gorm:"size:48"`
	TokensIn     int        `gorm:"not null;default:0"`
	TokensOut    int        `gorm:"not null;default:0"`
	CostMicros   int64      `gorm:"not null;default:0"`
	LatencyMS    int        `gorm:"column:latency_ms;not null;default:0"`
	CreatedAt    time.Time
}

// TableName pins the table.
func (AITask) TableName() string { return "ai_tasks" }

// AITaskRepository records model calls.
type AITaskRepository interface {
	Record(ctx context.Context, task *AITask) error
	// SpentSince is what every model call across the platform has cost since
	// the given moment, in millionths of a dollar.
	SpentSince(ctx context.Context, since time.Time) (int64, error)
}

// ErrAIQuotaExhausted is returned instead of calling a model when the company
// has used its allowance. The advert is still read by the rules.
var ErrAIQuotaExhausted = errors.New("ai: the plan's model allowance is used up")

// ErrAIBudgetExhausted is returned when the platform has spent its daily model
// budget. It is a cost brake, not a tariff, and lifts at midnight.
var ErrAIBudgetExhausted = errors.New("ai: the daily model budget is spent")

// How a model took part in reading an advert, as the person is told.
const (
	AIStatusOff         = "off"
	AIStatusNotNeeded   = "not_needed"
	AIStatusUsed        = "used"
	AIStatusNoResult    = "no_result"
	AIStatusLimited     = "limited"
	AIStatusUnavailable = "unavailable"
)

// Actor is who a request acts for. It travels in the context only as far as
// adapters that cannot take it as an argument — a model call made deep inside
// the parser still has to be charged to someone.
type Actor struct {
	UserID    uuid.UUID
	CompanyID uuid.UUID
}

type actorKey struct{}

// WithActor attaches the actor to a context.
func WithActor(ctx context.Context, actor Actor) context.Context {
	return context.WithValue(ctx, actorKey{}, actor)
}

// ActorFrom reads the actor back.
func ActorFrom(ctx context.Context) (Actor, bool) {
	actor, ok := ctx.Value(actorKey{}).(Actor)
	return actor, ok && actor.CompanyID != uuid.Nil
}
