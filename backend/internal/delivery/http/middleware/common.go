package middleware

import (
	"errors"
	"strconv"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"

	"github.com/loadify/backend/internal/config"
	"github.com/loadify/backend/internal/infrastructure/cache"
	"github.com/loadify/backend/pkg/apperror"
	"github.com/loadify/backend/pkg/logger"
	"github.com/loadify/backend/pkg/response"
)

// RequestIDHeader is the header used to correlate a request across services and logs.
const RequestIDHeader = "X-Request-Id"

// AssignRequestID assigns a correlation id, honouring one supplied by the caller.
func AssignRequestID() fiber.Handler {
	return func(c *fiber.Ctx) error {
		id := c.Get(RequestIDHeader)
		if id == "" || len(id) > 64 {
			id = uuid.New().String()
		}
		c.Locals(KeyRequestID, id)
		c.Set(RequestIDHeader, id)
		return c.Next()
	}
}

// SecurityHeaders sets the response headers that cost nothing and close off
// whole classes of browser-side attacks.
func SecurityHeaders(isProduction bool) fiber.Handler {
	return func(c *fiber.Ctx) error {
		c.Set("X-Content-Type-Options", "nosniff")
		c.Set("X-Frame-Options", "DENY")
		c.Set("Referrer-Policy", "strict-origin-when-cross-origin")
		c.Set("Permissions-Policy", "geolocation=(self), microphone=(), camera=()")
		c.Set("Cross-Origin-Resource-Policy", "same-site")
		// The API returns JSON only; a restrictive CSP is safe and blocks any
		// attempt to render a response as HTML.
		c.Set("Content-Security-Policy", "default-src 'none'; frame-ancestors 'none'; sandbox")
		if isProduction {
			c.Set("Strict-Transport-Security", "max-age=31536000; includeSubDomains")
		}
		return c.Next()
	}
}

// AccessLog records one structured line per request.
func AccessLog(log logger.Logger) fiber.Handler {
	return func(c *fiber.Ctx) error {
		start := time.Now()
		err := c.Next()

		// A handler that fails returns the error up to Fiber's central
		// ErrorHandler, which only writes the status after every middleware has
		// unwound. Reading the response here would therefore report 200 for
		// every rejection, so the status is taken from the error instead.
		status := c.Response().StatusCode()
		if err != nil {
			status = apperror.As(err).HTTPStatus
		}

		fields := []logger.Field{
			logger.String("method", c.Method()),
			logger.String("path", c.Path()),
			logger.Int("status", status),
			logger.Duration("latency", time.Since(start)),
			logger.String("ip", c.IP()),
			logger.String("request_id", RequestID(c)),
		}
		if claims := Claims(c); claims != nil {
			fields = append(fields, logger.String("user_id", claims.UserID.String()))
		}
		if err != nil {
			// The machine code is what an operator greps for; the wrapped cause
			// is the only place the underlying failure is recorded.
			appErr := apperror.As(err)
			fields = append(fields,
				logger.String("error_code", appErr.Code),
				logger.Err(err))
		}

		switch {
		case status >= 500:
			log.Error("request failed", fields...)
		case status >= 400:
			log.Warn("request rejected", fields...)
		default:
			log.Info("request", fields...)
		}
		return err
	}
}

// RateLimit throttles by authenticated user when possible and by IP otherwise,
// using Redis so the limit holds across API nodes.
func RateLimit(c cache.Cache, log logger.Logger, scope string, max int, window time.Duration) fiber.Handler {
	return func(ctx *fiber.Ctx) error {
		identity := ctx.IP()
		if claims := Claims(ctx); claims != nil {
			identity = claims.UserID.String()
		}

		key := cache.RateLimitKey(scope, identity)
		count, err := c.Incr(ctx.UserContext(), key, window)
		if err != nil {
			// Redis being down must not take the API down with it.
			log.Warn("rate limit backend unavailable", logger.Err(err))
			return ctx.Next()
		}

		remaining := max - int(count)
		if remaining < 0 {
			remaining = 0
		}
		ctx.Set("X-RateLimit-Limit", itoa(max))
		ctx.Set("X-RateLimit-Remaining", itoa(remaining))

		if int(count) > max {
			ttl, _ := c.TTL(ctx.UserContext(), key)
			ctx.Set("Retry-After", itoa(int(ttl.Seconds())+1))
			return apperror.ErrRateLimited
		}
		return ctx.Next()
	}
}

// CORS answers preflight requests and echoes only allowed origins.
func CORS(cfg config.SecurityConfig) fiber.Handler {
	allowed := make(map[string]bool, len(cfg.CORSAllowedOrigins))
	allowAny := false
	for _, o := range cfg.CORSAllowedOrigins {
		if o == "*" {
			allowAny = true
		}
		allowed[o] = true
	}

	return func(c *fiber.Ctx) error {
		origin := c.Get(fiber.HeaderOrigin)
		if origin != "" && (allowAny || allowed[origin]) {
			c.Set(fiber.HeaderAccessControlAllowOrigin, origin)
			c.Set(fiber.HeaderAccessControlAllowCredentials, "true")
			c.Set(fiber.HeaderVary, "Origin")
		}
		c.Set(fiber.HeaderAccessControlAllowMethods, "GET,POST,PUT,PATCH,DELETE,OPTIONS")
		c.Set(fiber.HeaderAccessControlAllowHeaders,
			"Origin,Content-Type,Accept,Authorization,X-Request-Id,X-Device-Id,X-Device-Name,X-Platform,X-App-Version,Idempotency-Key,Accept-Language")
		c.Set(fiber.HeaderAccessControlExposeHeaders,
			"X-Request-Id,X-RateLimit-Limit,X-RateLimit-Remaining,Retry-After")
		c.Set(fiber.HeaderAccessControlMaxAge, "600")

		if c.Method() == fiber.MethodOptions {
			return c.SendStatus(fiber.StatusNoContent)
		}
		return c.Next()
	}
}

// Locale resolves the response language from the client, falling back to the
// account language and then the platform default.
func Locale(defaultLocale string) fiber.Handler {
	supported := map[string]bool{"uz": true, "ru": true, "en": true}
	return func(c *fiber.Ctx) error {
		locale := strings.ToLower(strings.TrimSpace(c.Get("X-Locale")))
		if locale == "" {
			header := c.Get(fiber.HeaderAcceptLanguage)
			if primary, _, _ := strings.Cut(header, ","); primary != "" {
				lang, _, _ := strings.Cut(strings.TrimSpace(primary), "-")
				locale = strings.ToLower(lang)
			}
		}
		if !supported[locale] {
			locale = defaultLocale
		}
		c.Locals("locale", locale)
		c.Set("Content-Language", locale)
		return c.Next()
	}
}

// ErrorHandler is the single place where an error becomes an HTTP response.
func ErrorHandler(log logger.Logger, isProduction bool) fiber.ErrorHandler {
	return func(c *fiber.Ctx, err error) error {
		requestID := RequestID(c)

		var appErr *apperror.Error
		var fiberErr *fiber.Error
		if errors.As(err, &fiberErr) {
			appErr = mapFiberError(fiberErr)
		} else {
			appErr = apperror.As(err)
		}

		if appErr.HTTPStatus >= 500 {
			log.Error("unhandled error",
				logger.String("request_id", requestID),
				logger.String("path", c.Path()),
				logger.String("method", c.Method()),
				logger.String("code", appErr.Code),
				logger.Err(err))
			if isProduction {
				// Never leak internals; the request id is enough to find the log line.
				appErr = apperror.ErrInternal
			}
		}

		return response.Fail(c, appErr, requestID)
	}
}

func mapFiberError(err *fiber.Error) *apperror.Error {
	switch err.Code {
	case fiber.StatusNotFound:
		return apperror.ErrNotFound.WithMessage("Endpoint not found")
	case fiber.StatusMethodNotAllowed:
		return apperror.New("METHOD_NOT_ALLOWED", "Method not allowed for this endpoint", err.Code)
	case fiber.StatusRequestEntityTooLarge:
		return apperror.ErrPayload
	case fiber.StatusUnsupportedMediaType:
		return apperror.New("UNSUPPORTED_MEDIA_TYPE", "Unsupported content type", err.Code)
	default:
		if err.Code >= 500 {
			return apperror.ErrInternal.Wrap(err)
		}
		return apperror.New("REQUEST_ERROR", err.Message, err.Code)
	}
}

func itoa(n int) string { return strconv.Itoa(n) }
