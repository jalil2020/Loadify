package handler

import (
	"context"
	"time"

	"github.com/gofiber/fiber/v2"

	"github.com/loadify/backend/internal/infrastructure/cache"
	"github.com/loadify/backend/internal/infrastructure/database"
	"github.com/loadify/backend/pkg/response"
)

// HealthHandler exposes liveness and readiness probes.
type HealthHandler struct {
	db        *database.DB
	cache     cache.Cache
	version   string
	env       string
	startedAt time.Time
}

// NewHealthHandler builds the health handler.
func NewHealthHandler(db *database.DB, c cache.Cache, version, env string) *HealthHandler {
	return &HealthHandler{db: db, cache: c, version: version, env: env, startedAt: time.Now()}
}

// Live answers as long as the process is running.
// GET /health
func (h *HealthHandler) Live(c *fiber.Ctx) error {
	return response.OK(c, fiber.Map{
		"status":         "ok",
		"version":        h.version,
		"environment":    h.env,
		"uptime_seconds": int(time.Since(h.startedAt).Seconds()),
		"time":           time.Now().UTC(),
	})
}

// Ready answers only when the dependencies the API cannot serve without are up.
// Kubernetes and the load balancer use this to decide whether to route traffic.
// GET /health/ready
func (h *HealthHandler) Ready(c *fiber.Ctx) error {
	ctx, cancel := context.WithTimeout(c.UserContext(), 3*time.Second)
	defer cancel()

	checks := fiber.Map{}
	healthy := true

	if err := h.db.Health(ctx); err != nil {
		checks["postgres"] = fiber.Map{"status": "down", "error": err.Error()}
		healthy = false
	} else {
		checks["postgres"] = fiber.Map{"status": "up", "pool": h.db.Stats()}
	}

	if err := h.cache.Health(ctx); err != nil {
		checks["redis"] = fiber.Map{"status": "down", "error": err.Error()}
		healthy = false
	} else {
		checks["redis"] = fiber.Map{"status": "up"}
	}

	status := fiber.StatusOK
	overall := "ok"
	if !healthy {
		status = fiber.StatusServiceUnavailable
		overall = "degraded"
	}

	return c.Status(status).JSON(response.Envelope{
		Success: healthy,
		Data: fiber.Map{
			"status":  overall,
			"version": h.version,
			"checks":  checks,
		},
	})
}
