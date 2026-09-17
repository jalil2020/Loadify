package bootstrap

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"sync"
	"syscall"
	"time"

	"github.com/google/uuid"

	"github.com/loadify/backend/internal/config"
	"github.com/loadify/backend/internal/delivery/ws"
	"github.com/loadify/backend/internal/domain"
	"github.com/loadify/backend/internal/infrastructure/cache"
	"github.com/loadify/backend/internal/infrastructure/crypto"
	"github.com/loadify/backend/internal/infrastructure/database"
	"github.com/loadify/backend/internal/infrastructure/push"
	"github.com/loadify/backend/internal/infrastructure/queue"
	"github.com/loadify/backend/internal/infrastructure/ratelimit"
	telegramclient "github.com/loadify/backend/internal/infrastructure/telegram"
	"github.com/loadify/backend/internal/repository"
	"github.com/loadify/backend/internal/usecase"
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

	// Telegram is the client that actually sends. Without credentials it is
	// nil and the worker runs its health loop and nothing else: relaying jobs
	// to a queue nobody can drain would only fill Redis.
	Telegram domain.TelegramSender

	// Alerts tells account owners, their managers and platform admins when
	// Telegram stops an account or a group stops accepting it.
	Alerts domain.DistributionAlerter

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
	container.Telegram = telegramclient.New(cfg.Telegram, log)
	container.Alerts = container.alerts()
	container.wire()
	return container, nil
}

// alerts builds the notification path the worker reports through. It is the
// same one the API uses: the in-app row is written here, push goes out from
// here, and the realtime event reaches open sockets through Redis, since this
// process holds none of its own.
func (c *PosterContainer) alerts() domain.DistributionAlerter {
	users := repository.NewUserRepository(c.DB.DB)
	companies := repository.NewCompanyRepository(c.DB.DB)

	pushSender, err := push.New(context.Background(), c.Cfg.FCM, c.Log)
	if err != nil {
		c.Log.Warn("push notifications unavailable to the worker", logger.Err(err))
		pushSender = nil
	}
	events := ws.NewPublisher(ws.NewHub(c.Cache, companies, c.Log))
	notifier := usecase.NewNotificationUsecase(
		repository.NewNotificationRepository(c.DB.DB), users, pushSender, events, c.Log,
	)
	return usecase.NewDistributionAlertUsecase(
		notifier, users, companies, c.Cache, appLocation(c.Cfg.App.Timezone, c.Log), c.Log,
	)
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
		guard, c.Telegram, c.Vault, c.Alerts, c.Cfg.Telegram, owner, c.Log,
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
//
// A consumer that stops on its own takes the process down with it, and the
// error is returned: a worker that looks alive but reads nothing is worse than
// one that restarts.
func RunPoster(c *PosterContainer) error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	failed := make(chan error, 1)

	c.Log.Info("posting worker started",
		logger.String("version", Version),
		logger.String("env", c.Cfg.App.Env),
		logger.Bool("telegram_configured", c.Cfg.Telegram.Enabled()),
		logger.Bool("client_ready", c.Sender != nil))

	var wg sync.WaitGroup
	wg.Go(func() { c.runHealthLoop(ctx) })

	switch {
	case c.Sender == nil:
		c.Log.Warn("telegram credentials are not configured: nothing will be sent")
	default:
		wg.Go(func() { c.Relay.Run(ctx) })
		wg.Go(func() {
			if err := c.Queue.Consume(ctx, c.Sender.Process); err != nil && ctx.Err() == nil {
				c.Log.Error("queue consumer stopped", logger.Err(err))
				failed <- fmt.Errorf("queue consumer: %w", err)
				stop()
			}
		})
	}

	<-ctx.Done()
	c.Log.Info("shutting down, draining")

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

	select {
	case err := <-failed:
		return err
	default:
		return nil
	}
}

// HeartbeatFile is refreshed while both stores answer. The worker has no HTTP
// port, so this file's age is what the container health check looks at.
var HeartbeatFile = filepath.Join(os.TempDir(), "loadify-poster.alive")

// runHealthLoop keeps an eye on the stores the worker cannot work without.
func (c *PosterContainer) runHealthLoop(ctx context.Context) {
	ticker := time.NewTicker(healthInterval)
	defer ticker.Stop()

	c.checkHealth(ctx)
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			c.checkHealth(ctx)
		}
	}
}

func (c *PosterContainer) checkHealth(ctx context.Context) {
	checkCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	healthy := true
	if err := c.DB.Health(checkCtx); err != nil {
		c.Log.Error("postgres unreachable", logger.Err(err))
		healthy = false
	}
	if err := c.Cache.Health(checkCtx); err != nil {
		// Redis holds the rate limits, so this is not a degraded mode: the
		// guard fails closed and nothing is sent.
		c.Log.Error("redis unreachable, posting is paused", logger.Err(err))
		healthy = false
	}
	if !healthy {
		return
	}
	stamp := []byte(time.Now().UTC().Format(time.RFC3339))
	if err := os.WriteFile(HeartbeatFile, stamp, 0o600); err != nil {
		c.Log.Warn("heartbeat not written", logger.String("path", HeartbeatFile), logger.Err(err))
	}
}
