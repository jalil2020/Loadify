package ai

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"math"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/loadify/backend/internal/config"
	"github.com/loadify/backend/internal/domain"
	"github.com/loadify/backend/pkg/logger"
)

// errNoActor keeps a model call nobody can be charged for from being made.
var errNoActor = errors.New("ai: no company to charge the call to")

// ResultCache is the part of the platform cache the meter needs.
type ResultCache interface {
	Get(ctx context.Context, key string, dest any) error
	Set(ctx context.Context, key string, value any, ttl time.Duration) error
}

// Metered is the accounting in front of a model.
//
// Every call is charged to the company that caused it and written down with
// what it cost. A call the company's plan or the platform's daily budget
// cannot cover is refused before it is made, and an advert that was already
// read is answered from cache for nothing. None of this changes what the
// parser does with an answer; it only decides whether there is one.
type Metered struct {
	inner    domain.AdExtractor
	quota    domain.QuotaGuard
	tasks    domain.AITaskRepository
	cache    ResultCache
	cfg      config.AIConfig
	location *time.Location
	log      logger.Logger
	now      func() time.Time
}

// NewMetered wraps an extractor. cache may be nil.
func NewMetered(
	inner domain.AdExtractor,
	quota domain.QuotaGuard,
	tasks domain.AITaskRepository,
	cache ResultCache,
	cfg config.AIConfig,
	location *time.Location,
	log logger.Logger,
) *Metered {
	if location == nil {
		location = time.UTC
	}
	return &Metered{
		inner: inner, quota: quota, tasks: tasks, cache: cache,
		cfg: cfg, location: location, log: log, now: time.Now,
	}
}

// Extract charges, refuses or answers from cache, and otherwise asks the model.
func (m *Metered) Extract(ctx context.Context, req domain.AdExtractRequest) (domain.AdExtractResult, error) {
	actor, ok := domain.ActorFrom(ctx)
	if !ok {
		m.log.Error("model call without a company in context; not calling")
		return domain.AdExtractResult{}, errNoActor
	}

	task := &domain.AITask{
		CompanyID: actor.CompanyID,
		Kind:      domain.AITaskParse,
		Provider:  m.cfg.Provider,
		Model:     m.cfg.Model,
		InputHash: m.inputHash(req),
		Status:    domain.AITaskOK,
	}
	if actor.UserID != uuid.Nil {
		userID := actor.UserID
		task.UserID = &userID
	}

	if cached, hit := m.cached(ctx, task.InputHash); hit {
		task.CacheHit = true
		if cached.Model != "" {
			task.Model = cached.Model
		}
		m.record(ctx, task)
		return cached, nil
	}

	status, err := m.quota.Status(ctx, actor.CompanyID, domain.QuotaAIRequests)
	if err != nil {
		return domain.AdExtractResult{}, err
	}
	if status.Exceeded(1) {
		m.reject(ctx, task, domain.AIRejectQuota)
		return domain.AdExtractResult{}, domain.ErrAIQuotaExhausted
	}
	over, err := m.overBudget(ctx)
	if err != nil {
		return domain.AdExtractResult{}, err
	}
	if over {
		m.reject(ctx, task, domain.AIRejectBudget)
		return domain.AdExtractResult{}, domain.ErrAIBudgetExhausted
	}

	started := m.now()
	result, err := m.inner.Extract(ctx, req)
	task.LatencyMS = int(m.now().Sub(started).Milliseconds())
	if err != nil {
		// A failed call is recorded and counted: a timeout may still have been
		// billed, and a provider that keeps failing should show up somewhere.
		task.Status = domain.AITaskFailed
		m.record(ctx, task)
		return result, err
	}

	if result.Model != "" {
		task.Model = result.Model
	}
	task.TokensIn = result.TokensIn
	task.TokensOut = result.TokensOut
	task.CostMicros = m.cost(result.TokensIn, result.TokensOut)
	m.record(ctx, task)
	m.remember(ctx, task.InputHash, result)
	return result, nil
}

// cost is in millionths of a dollar. Prices are per million tokens, so a token
// costs exactly its price in micro-dollars.
func (m *Metered) cost(in, out int) int64 {
	return int64(math.Round(float64(in)*m.cfg.PriceInPerMTok + float64(out)*m.cfg.PriceOutPerMTok))
}

// overBudget reports whether today's spending across the platform has reached
// the configured brake.
func (m *Metered) overBudget(ctx context.Context) (bool, error) {
	if m.cfg.DailyBudgetUSD <= 0 {
		return false, nil
	}
	local := m.now().In(m.location)
	midnight := time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, m.location)
	spent, err := m.tasks.SpentSince(ctx, midnight)
	if err != nil {
		return false, err
	}
	return spent >= int64(math.Round(m.cfg.DailyBudgetUSD*1e6)), nil
}

func (m *Metered) cached(ctx context.Context, hash string) (domain.AdExtractResult, bool) {
	var result domain.AdExtractResult
	if m.cache == nil || m.cfg.CacheTTL <= 0 {
		return result, false
	}
	if err := m.cache.Get(ctx, cacheKey(hash), &result); err != nil {
		return result, false
	}
	// What was cached is the reading, not the bill.
	result.TokensIn, result.TokensOut = 0, 0
	return result, true
}

func (m *Metered) remember(ctx context.Context, hash string, result domain.AdExtractResult) {
	if m.cache == nil || m.cfg.CacheTTL <= 0 {
		return
	}
	if err := m.cache.Set(ctx, cacheKey(hash), result, m.cfg.CacheTTL); err != nil {
		m.log.Warn("model answer not cached", logger.Err(err))
	}
}

func (m *Metered) reject(ctx context.Context, task *domain.AITask, reason string) {
	task.Status = domain.AITaskRejected
	task.RejectReason = &reason
	m.record(ctx, task)
}

// record never fails the call: the answer is already paid for, and throwing it
// away because the ledger hiccuped would only cost a second call.
func (m *Metered) record(ctx context.Context, task *domain.AITask) {
	if err := m.tasks.Record(ctx, task); err != nil {
		m.log.Error("model call not recorded",
			logger.String("company_id", task.CompanyID.String()),
			logger.String("status", task.Status),
			logger.Err(err))
	}
}

// inputHash identifies a question. It covers the instructions and the model,
// so changing either stops old answers from being served.
func (m *Metered) inputHash(req domain.AdExtractRequest) string {
	h := sha256.New()
	for _, part := range []string{
		systemPrompt,
		m.cfg.Provider,
		m.cfg.Model,
		req.Text,
		strings.Join(req.Missing, ","),
		strings.Join(req.Unresolved, "\x1f"),
	} {
		h.Write([]byte(part))
		h.Write([]byte{0})
	}
	return hex.EncodeToString(h.Sum(nil))
}

func cacheKey(hash string) string { return "ai:extract:" + hash }
