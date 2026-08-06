// Package route registers every HTTP endpoint. Route files are the map of the
// API: one place to see what exists and what protects it.
package route

import (
	"time"

	"github.com/gofiber/fiber/v2"

	"github.com/loadify/backend/internal/config"
	"github.com/loadify/backend/internal/delivery/http/handler"
	"github.com/loadify/backend/internal/delivery/http/middleware"
	"github.com/loadify/backend/internal/infrastructure/cache"
	"github.com/loadify/backend/pkg/logger"
)

// Handlers bundles every handler the router needs.
type Handlers struct {
	Health *handler.HealthHandler
	Auth   *handler.AuthHandler
}

// Deps is everything route registration needs beyond the handlers.
type Deps struct {
	Cfg   *config.Config
	Auth  *middleware.AuthMiddleware
	Cache cache.Cache
	Log   logger.Logger
}

// Register mounts all routes on the app.
func Register(app *fiber.App, h Handlers, d Deps) {
	app.Get("/health", h.Health.Live)
	app.Get("/health/ready", h.Health.Ready)

	v1 := app.Group("/api/v1")
	registerAuthRoutes(v1, h, d)
}

// registerAuthRoutes mounts /api/v1/auth.
//
// The unauthenticated endpoints carry their own, much tighter rate limits:
// credential and OTP endpoints are the ones worth brute-forcing.
func registerAuthRoutes(v1 fiber.Router, h Handlers, d Deps) {
	auth := v1.Group("/auth")

	strict := middleware.RateLimit(d.Cache, d.Log, "auth_strict", 10, time.Minute)
	otp := middleware.RateLimit(d.Cache, d.Log, "auth_otp", 5, 10*time.Minute)

	auth.Post("/register", otp, h.Auth.Register)
	auth.Post("/verify-otp", strict, h.Auth.VerifyOTP)
	auth.Post("/resend-otp", otp, h.Auth.ResendOTP)
	auth.Post("/login", strict, h.Auth.Login)
	auth.Post("/refresh", strict, h.Auth.Refresh)
	auth.Post("/forgot-password", otp, h.Auth.ForgotPassword)
	auth.Post("/reset-password", strict, h.Auth.ResetPassword)

	protected := auth.Group("", d.Auth.Authenticate())
	protected.Get("/me", h.Auth.Me)
	protected.Post("/logout", h.Auth.Logout)
	protected.Post("/logout-all", h.Auth.LogoutAll)
	protected.Post("/change-password", strict, h.Auth.ChangePassword)
	protected.Get("/sessions", h.Auth.Sessions)
	protected.Delete("/sessions/:id", h.Auth.RevokeSession)
}
