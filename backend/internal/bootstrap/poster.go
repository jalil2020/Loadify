package bootstrap

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/google/uuid"

	"github.com/loadify/backend/internal/config"
	"github.com/loadify/backend/internal/domain"
	"github.com/loadify/backend/internal/infrastructure/cache"
	"github.com/loadify/backend/internal/infrastructure/crypto"
	"github.com/loadify/backend/internal/infrastructure/database"
	"github.com/loadify/backend/internal/infrastructure/queue"
	"github.com/loadify/backend/internal/infrastructure/ratelimit"
	"github.com/loadify/backend/internal/repository"
	"github.com/loadify/backend/internal/worker"
	"github.com/loadify/backend/pkg/logger"
)

// PosterContainer is the composition root of the posting worker.
//
// It is deliberately much smaller than the API container: the worker has no
// HTTP surface, no storage and no payment providers. What it does have, and the
// API process does not, is the vault that opens Telegram sessions.
type PosterContainer struct {
	Cfg   *config.Config
	Log   logger.Logger
	DB    *database.DB
	Cache cache.Cache
	Vault domain.SessionVault

	// Telegram is the client that actually sends. Until it is configured the
	// worker runs its health loop and nothing else: relaying jobs to a queue
	// nobody can drain would only fill Redis.
	Telegram domain.TelegramSender

	Relay  *worker.Relay
	Sender *worker.Sender
	Queue  *queue.Streams
}

// NewPosterContainer connects everything the worker needs, failing fast.
func NewPosterContainer(cfg *config.Config, log logger.Logger) (*PosterContainer, error) {
	db, err := database.New(cfg.DB, cfg.App.Env, log)
	if err != nil {
		return nil, fmt.Errorf("database: %w", err)
	}

	redisCache, err := cache.New(cfg.Redis, log)
	if err != nil {
		return nil, fmt.Errorf("cache: %w", err)
	}

	container := &PosterContainer{Cfg: cfg, Log: log, DB: db, Cache: redisCache}

	// Without credentials the worker still runs — it simply has nothing to
	// send. That keeps a deployment without Telegram configured from crash
	// looping, and keeps the health loop reporting.
	if !cfg.Telegram.Enabled() {
		return container, nil
	}

	keks, current, err := cfg.Telegram.KEKs()
	if err != nil {
		return nil, fmt.Errorf("session keys: %w", err)
	}
	vault, err := crypto.NewSessionVault(keks, current)
	if err != nil {
		return nil, fmt.Errorf("session vault: %w", err)
	}
	container.Vault = vault

	if container.Telegram == nil {
		return container, nil
	}
	container.wire()
	return container, nil
}

// wire builds the pipeline once a client exists.
func (c *PosterContainer) wire() {
	posting := repository.NewPostingRepository(c.DB.DB)
	guard := ratelimit.NewGuard(c.Cache.Client(), c.Cfg.Telegram)

	host, _ := os.Hostname()
	// The owner names this process in every lease it takes, so a job stuck in
	// processing can be traced to the worker that took it.
	owner := fmt.Sprintf("%s-%s", host, uuid.NewString()[:8])

	c.Queue = queue.NewStreams(c.Cache.Client(), owner,
		2*c.Cfg.Telegram.LeaseTTL, c.Cfg.Telegram.LeaseTTL, c.Log)
	c.Relay = worker.NewRelay(posting, c.Queue, time.Second, 100, c.Log)
	c.Sender = worker.New(
		posting,
		repository.NewTelegramAccountRepository(c.DB.DB),
		repository.NewTelegramGroupRepository(c.DB.DB),
		repository.NewLoadAdRepository(c.DB.DB),
		guard, c.Telegram, c.Vault, c.Cfg.Telegram, owner, c.Log,
	)
}

// Close releases every resource the container owns.
func (c *PosterContainer) Close() {
	if c.Cache != nil {
		if err := c.Cache.Close(); err != nil {
			c.Log.Warn("closing redis failed", logger.Err(err))
		}
	}
	if c.DB != nil {
		if err := c.DB.Close(); err != nil {
			c.Log.Warn("closing postgres failed", logger.Err(err))
		}
	}
}

// healthInterval is how often the worker proves it can still reach its stores.
// A worker that cannot reach Redis must not send, so noticing early matters.
const healthInterval = 30 * time.Second

// RunPoster starts the worker loops and blocks until the process is asked to
// stop, then drains within the configured grace period.
//
// Draining is the point: a message that is already on its way to Telegram must
// finish and be recorded, because a send whose outcome we never wrote down is
// the one case where a retry could post twice.
func RunPoster(c *PosterContainer) error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	c.Log.Info("posting worker started",
		logger.String("version", Version),
		logger.String("env", c.Cfg.App.Env),
		logger.Bool("telegram_configured", c.Cfg.Telegram.Enabled()),
		logger.Bool("client_ready", c.Sender != nil))

	var wg sync.WaitGroup
	wg.Go(func() { c.runHealthLoop(ctx) })

	switch {
	case !c.Cfg.Telegram.Enabled():
		c.Log.Warn("telegram credentials are not configured: nothing will be sent")
	case c.Sender == nil:
		c.Log.Warn("no telegram client is wired into this build: nothing will be sent")
	default:
		wg.Go(func() { c.Relay.Run(ctx) })
		wg.Go(func() {
			if err := c.Queue.Consume(ctx, c.Sender.Process); err != nil {
				c.Log.Error("queue consumer stopped", logger.Err(err))
			}
		})
	}

	<-ctx.Done()
	c.Log.Info("shutdown signal received, draining")

	drained := make(chan struct{})
	go func() {
		wg.Wait()
		close(drained)
	}()

	select {
	case <-drained:
		c.Log.Info("shutdown complete")
	case <-time.After(c.Cfg.App.ShutdownGrace):
		// Whatever is still running holds a lease that will expire. The job
		// that comes back from it is recognised as interrupted, and is not sent
		// a second time unless random_id handling has been confirmed.
		c.Log.Warn("grace period expired with work still in flight",
			logger.Duration("grace", c.Cfg.App.ShutdownGrace))
	}
	return nil
}

// runHealthLoop keeps an eye on the stores the worker cannot work without.
func (c *PosterContainer) runHealthLoop(ctx context.Context) {
	ticker := time.NewTicker(healthInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			checkCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
			if err := c.DB.Health(checkCtx); err != nil {
				c.Log.Error("postgres unreachable", logger.Err(err))
			}
			if err := c.Cache.Health(checkCtx); err != nil {
				// Redis holds the rate limits, so this is not a degraded mode:
				// the guard fails closed and nothing is sent.
				c.Log.Error("redis unreachable, posting is paused", logger.Err(err))
			}
			cancel()
		}
	}
}
