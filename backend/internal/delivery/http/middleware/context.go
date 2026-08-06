// Package middleware holds the Fiber middleware stack: request identity,
// logging, rate limiting, authentication and authorisation.
package middleware

import (
	"context"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"

	"github.com/loadify/backend/internal/usecase"
	"github.com/loadify/backend/pkg/apperror"
	"github.com/loadify/backend/pkg/jwt"
)

// Locals keys. Everything the middleware stack stores on the request goes
// through these constants.
const (
	KeyClaims    = "auth_claims"
	KeyRequestID = "request_id"
)

// Claims returns the authenticated principal, or nil for anonymous requests.
func Claims(c *fiber.Ctx) *jwt.Claims {
	if v, ok := c.Locals(KeyClaims).(*jwt.Claims); ok {
		return v
	}
	return nil
}

// MustClaims returns the principal or an unauthorised error. Handlers behind
// Authenticate can rely on it being present.
func MustClaims(c *fiber.Ctx) (*jwt.Claims, error) {
	claims := Claims(c)
	if claims == nil {
		return nil, apperror.ErrUnauthorized
	}
	return claims, nil
}

// UserID returns the authenticated user id.
func UserID(c *fiber.Ctx) (uuid.UUID, error) {
	claims, err := MustClaims(c)
	if err != nil {
		return uuid.Nil, err
	}
	return claims.UserID, nil
}

// CompanyID returns the company the principal acts for, if any.
func CompanyID(c *fiber.Ctx) *uuid.UUID {
	claims := Claims(c)
	if claims == nil {
		return nil
	}
	return claims.CompanyID
}

// DriverID returns the driver profile of the principal, if any.
func DriverID(c *fiber.Ctx) *uuid.UUID {
	claims := Claims(c)
	if claims == nil {
		return nil
	}
	return claims.DriverID
}

// RequestID returns the correlation id assigned to this request.
func RequestID(c *fiber.Ctx) string {
	if v, ok := c.Locals(KeyRequestID).(string); ok {
		return v
	}
	return ""
}

// Device builds the device fingerprint recorded on sessions.
func Device(c *fiber.Ctx) usecase.DeviceInfo {
	return usecase.DeviceInfo{
		DeviceID:   c.Get("X-Device-Id"),
		DeviceName: c.Get("X-Device-Name"),
		Platform:   c.Get("X-Platform"),
		AppVersion: c.Get("X-App-Version"),
		IPAddress:  c.IP(),
		UserAgent:  c.Get(fiber.HeaderUserAgent),
	}
}

// RequestContext returns a context carrying the request deadline and the
// correlation id, for passing into usecases.
func RequestContext(c *fiber.Ctx) context.Context {
	return c.UserContext()
}
