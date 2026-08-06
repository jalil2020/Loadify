package middleware

import (
	"errors"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"

	"github.com/loadify/backend/internal/domain"
	"github.com/loadify/backend/internal/infrastructure/cache"
	"github.com/loadify/backend/pkg/apperror"
	"github.com/loadify/backend/pkg/jwt"
	"github.com/loadify/backend/pkg/logger"
)

// AuthMiddleware verifies access tokens and enforces permissions.
type AuthMiddleware struct {
	tokens jwt.Manager
	users  domain.UserRepository
	cache  cache.Cache
	log    logger.Logger
}

// NewAuthMiddleware builds the authentication middleware.
func NewAuthMiddleware(tokens jwt.Manager, users domain.UserRepository, c cache.Cache, log logger.Logger) *AuthMiddleware {
	return &AuthMiddleware{tokens: tokens, users: users, cache: c, log: log}
}

// Authenticate rejects requests without a valid, non-revoked access token.
func (m *AuthMiddleware) Authenticate() fiber.Handler {
	return func(c *fiber.Ctx) error {
		claims, err := m.resolve(c)
		if err != nil {
			return err
		}
		c.Locals(KeyClaims, claims)
		return c.Next()
	}
}

// Optional attaches the principal when a token is present but lets anonymous
// requests through. Used by public endpoints that reveal more to signed-in users.
func (m *AuthMiddleware) Optional() fiber.Handler {
	return func(c *fiber.Ctx) error {
		if extractToken(c) == "" {
			return c.Next()
		}
		claims, err := m.resolve(c)
		if err != nil {
			// A bad token on an optional route is treated as anonymous rather
			// than as a failure, so an expired session never breaks browsing.
			return c.Next()
		}
		c.Locals(KeyClaims, claims)
		return c.Next()
	}
}

// RequirePermission blocks principals lacking the given permission code.
func (m *AuthMiddleware) RequirePermission(codes ...string) fiber.Handler {
	return func(c *fiber.Ctx) error {
		claims, err := MustClaims(c)
		if err != nil {
			return err
		}
		if claims.HasRole(domain.RoleSuperAdmin) {
			return c.Next()
		}
		for _, code := range codes {
			if claims.HasPermission(code) {
				return c.Next()
			}
		}
		return apperror.ErrForbidden.WithMessage(
			"This action requires the " + strings.Join(codes, " or ") + " permission")
	}
}

// RequireRole blocks principals not holding one of the given roles.
func (m *AuthMiddleware) RequireRole(roles ...string) fiber.Handler {
	return func(c *fiber.Ctx) error {
		claims, err := MustClaims(c)
		if err != nil {
			return err
		}
		for _, role := range roles {
			if claims.HasRole(role) {
				return c.Next()
			}
		}
		return apperror.ErrForbidden
	}
}

// RequireCompany blocks principals that are not acting for a company.
func (m *AuthMiddleware) RequireCompany() fiber.Handler {
	return func(c *fiber.Ctx) error {
		claims, err := MustClaims(c)
		if err != nil {
			return err
		}
		if claims.CompanyID == nil {
			return apperror.ErrForbidden.WithMessage("Create or join a company first")
		}
		return c.Next()
	}
}

// RequireDriver blocks principals without a driver profile.
func (m *AuthMiddleware) RequireDriver() fiber.Handler {
	return func(c *fiber.Ctx) error {
		claims, err := MustClaims(c)
		if err != nil {
			return err
		}
		if claims.DriverID == nil {
			return apperror.ErrForbidden.WithMessage("Complete your driver profile first")
		}
		return c.Next()
	}
}

// resolve parses the bearer token, checks the denylist and confirms the account
// is still active.
func (m *AuthMiddleware) resolve(c *fiber.Ctx) (*jwt.Claims, error) {
	raw := extractToken(c)
	if raw == "" {
		return nil, apperror.ErrUnauthorized
	}

	claims, err := m.tokens.ParseAccess(raw)
	if err != nil {
		return nil, err
	}

	jti, err := claims.JTI()
	if err != nil {
		return nil, apperror.ErrTokenInvalid.Wrap(err)
	}
	denied, err := m.cache.Exists(c.UserContext(), cache.DenylistKey(jti))
	if err != nil {
		// A cache outage must not let revoked tokens through silently, but it
		// also must not take the API down: log loudly and fail closed only for
		// the denylist check by refusing the request.
		m.log.Error("token denylist check failed", logger.Err(err))
		return nil, apperror.ErrUnavailable.Wrap(err)
	}
	if denied {
		return nil, apperror.ErrTokenRevoked
	}

	if err := m.assertUserActive(c, claims.UserID); err != nil {
		return nil, err
	}
	return claims, nil
}

// assertUserActive checks the account status, caching the answer so a blocked
// account is rejected within the cache TTL without a query per request.
func (m *AuthMiddleware) assertUserActive(c *fiber.Ctx, userID uuid.UUID) error {
	ctx := c.UserContext()
	key := cache.UserKey(userID)

	var status string
	if err := m.cache.Get(ctx, key, &status); err == nil {
		return statusToError(status)
	} else if !errors.Is(err, cache.ErrNotFound) {
		m.log.Warn("user status cache read failed", logger.Err(err))
	}

	user, err := m.users.FindByID(ctx, userID)
	if err != nil {
		if apperror.Is(err, apperror.ErrNotFound.Code) {
			return apperror.ErrUnauthorized
		}
		return err
	}
	status = string(user.Status)
	if err := m.cache.Set(ctx, key, status, userStatusCacheTTL); err != nil {
		m.log.Warn("user status cache write failed", logger.Err(err))
	}
	return statusToError(status)
}

// userStatusCacheTTL bounds how long a block can take to become effective.
const userStatusCacheTTL = 60 * time.Second

func statusToError(status string) error {
	switch domain.UserStatus(status) {
	case domain.UserStatusActive:
		return nil
	case domain.UserStatusBlocked:
		return apperror.ErrAccountBlocked
	case domain.UserStatusPendingVerification:
		return apperror.ErrPhoneNotVerified
	default:
		return apperror.ErrUnauthorized
	}
}

func extractToken(c *fiber.Ctx) string {
	header := c.Get(fiber.HeaderAuthorization)
	if after, found := strings.CutPrefix(header, "Bearer "); found {
		return strings.TrimSpace(after)
	}
	// WebSocket clients cannot set headers during the handshake in the browser,
	// so the token may arrive as a query parameter on the upgrade request only.
	if strings.HasPrefix(c.Path(), "/api/v1/ws") {
		return c.Query("token")
	}
	return ""
}
