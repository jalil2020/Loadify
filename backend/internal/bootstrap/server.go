package bootstrap

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gofiber/fiber/v2"

	"github.com/loadify/backend/internal/delivery/http/middleware"
	"github.com/loadify/backend/internal/delivery/http/route"
	"github.com/loadify/backend/pkg/logger"
)

// NewFiberApp builds the HTTP server with the full middleware stack applied in
// the order that matters: identity first, recovery second, then transport
// concerns, then authentication.
func NewFiberApp(c *Container) *fiber.App {
	app := fiber.New(fiber.Config{
		AppName:                 c.Cfg.App.Name,
		ErrorHandler:            middleware.ErrorHandler(c.Log, c.Cfg.App.IsProduction()),
		BodyLimit:               c.Cfg.App.BodyLimitMB * 1024 * 1024,
		ReadTimeout:             30 * time.Second,
		WriteTimeout:            60 * time.Second,
		IdleTimeout:             120 * time.Second,
		DisableStartupMessage:   true,
		EnableTrustedProxyCheck: len(c.Cfg.Security.TrustedProxies) > 0,
		TrustedProxies:          c.Cfg.Security.TrustedProxies,
		ProxyHeader:             fiber.HeaderXForwardedFor,
		JSONEncoder:             nil,
		JSONDecoder:             nil,
	})

	app.Use(middleware.AssignRequestID())
	app.Use(recoverPanics(c.Log))
	app.Use(middleware.SecurityHeaders(c.Cfg.App.IsProduction()))
	app.Use(middleware.CORS(c.Cfg.Security))
	app.Use(middleware.Locale(c.Cfg.App.DefaultLocale))
	app.Use(middleware.AccessLog(c.Log))
	app.Use(middleware.RateLimit(c.Cache, c.Log, "global",
		c.Cfg.Security.RateLimitMax, c.Cfg.Security.RateLimitWindow))

	route.Register(app, route.Handlers{
		Health:       c.Handlers.Health,
		Auth:         c.Handlers.Auth,
		Driver:       c.Handlers.Driver,
		Company:      c.Handlers.Company,
		File:         c.Handlers.File,
		Reference:    c.Handlers.Reference,
		Search:       c.Handlers.Search,
		Load:         c.Handlers.Load,
		LoadAd:       c.Handlers.LoadAd,
		Publish:      c.Handlers.Publish,
		Telegram:     c.Handlers.Telegram,
		Booking:      c.Handlers.Booking,
		Trip:         c.Handlers.Trip,
		Chat:         c.Handlers.Chat,
		Notification: c.Handlers.Notification,
		Payment:      c.Handlers.Payment,
		Admin:        c.Handlers.Admin,
		WS:           c.Handlers.WS,
	}, route.Deps{
		Cfg:   c.Cfg,
		Auth:  c.Auth,
		Cache: c.Cache,
		Log:   c.Log,
	})

	return app
}

// recoverPanics turns a panic into a 500 with a logged stack instead of a dead
// worker. Fiber's own recover middleware would work, but this keeps the log
// line in our own format and attaches the request id.
func recoverPanics(log logger.Logger) fiber.Handler {
	return func(c *fiber.Ctx) (err error) {
		defer func() {
			if r := recover(); r != nil {
				log.Error("panic recovered",
					logger.String("request_id", middleware.RequestID(c)),
					logger.String("path", c.Path()),
					logger.Any("panic", r))
				err = fiber.NewError(fiber.StatusInternalServerError)
			}
		}()
		return c.Next()
	}
}

// Run starts the server and blocks until SIGINT/SIGTERM, then shuts down
// gracefully within the configured grace period.
func Run(c *Container, app *fiber.App) error {
	addr := fmt.Sprintf(":%d", c.Cfg.App.Port)

	// The hub's Redis subscription runs for the life of the process. Its context
	// is cancelled before the HTTP shutdown so no new events arrive while
	// connections are draining.
	hubCtx, stopHub := context.WithCancel(context.Background())
	defer stopHub()
	if c.Hub != nil {
		go c.Hub.Run(hubCtx)
	}

	serverErr := make(chan error, 1)
	go func() {
		c.Log.Info("http server listening",
			logger.String("addr", addr),
			logger.String("env", c.Cfg.App.Env),
			logger.String("version", Version))
		if err := app.Listen(addr); err != nil && !errors.Is(err, context.Canceled) {
			serverErr <- err
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, os.Interrupt, syscall.SIGTERM)

	select {
	case err := <-serverErr:
		return fmt.Errorf("http server: %w", err)
	case sig := <-quit:
		c.Log.Info("shutdown signal received", logger.String("signal", sig.String()))
	}

	ctx, cancel := context.WithTimeout(context.Background(), c.Cfg.App.ShutdownGrace)
	defer cancel()

	// Close sockets first: a client that reconnects should find the next node,
	// not a half-shut one.
	if c.Hub != nil {
		stopHub()
		c.Hub.Close()
	}

	if err := app.ShutdownWithContext(ctx); err != nil {
		c.Log.Error("graceful shutdown failed", logger.Err(err))
	}
	c.Close(ctx)
	c.Log.Info("shutdown complete")
	return nil
}
