package middleware

import (
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"

	"github.com/loadify/backend/pkg/apperror"
)

type observed struct {
	method, route string
	status        int
}

type recorder struct {
	mu   sync.Mutex
	seen []observed
}

func (r *recorder) ObserveRequest(method, route string, status int, _ time.Duration) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.seen = append(r.seen, observed{method, route, status})
}

func (r *recorder) last() observed {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.seen[len(r.seen)-1]
}

// The labels must come from a closed set: the pattern an endpoint was
// registered with, or "unmatched" — never the path a client made up.
func TestRequestMetricsAreLabelledByRoutePattern(t *testing.T) {
	rec := &recorder{}
	app := fiber.New(fiber.Config{
		ErrorHandler: func(c *fiber.Ctx, err error) error {
			return c.SendStatus(apperror.As(err).HTTPStatus)
		},
	})
	app.Use(Metrics(app, rec))

	v1 := app.Group("/api/v1")
	v1.Get("/load-ads/:id", func(c *fiber.Ctx) error { return c.SendString("ok") })
	v1.Post("/load-ads", func(*fiber.Ctx) error { return apperror.ErrValidation })

	admin := v1.Group("/admin", func(*fiber.Ctx) error { return apperror.ErrForbidden })
	admin.Get("/users", func(c *fiber.Ctx) error { return c.SendString("never") })

	cases := []struct {
		name, method, path string
		want               observed
	}{
		{"an endpoint with a parameter", "GET", "/api/v1/load-ads/0b8e-anything",
			observed{"GET", "/api/v1/load-ads/:id", 200}},
		{"a handler that fails", "POST", "/api/v1/load-ads",
			observed{"POST", "/api/v1/load-ads", apperror.ErrValidation.HTTPStatus}},
		{"a group middleware that refuses", "GET", "/api/v1/admin/users",
			observed{"GET", "/api/v1/admin/*", apperror.ErrForbidden.HTTPStatus}},
		{"a path under a group nobody registered", "GET", "/api/v1/admin/wp-login.php",
			observed{"GET", "/api/v1/admin/*", apperror.ErrForbidden.HTTPStatus}},
		{"a path outside every group", "GET", "/wp-login.php",
			observed{"GET", "/*", 404}},
		{"a method the table does not list", "TRACE", "/api/v1/load-ads/1",
			observed{"other", "/*", 405}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := app.Test(httptest.NewRequest(tc.method, tc.path, nil)); err != nil {
				t.Fatalf("request: %v", err)
			}
			if got := rec.last(); got != tc.want {
				t.Errorf("observed %+v, want %+v", got, tc.want)
			}
		})
	}

	// Labels are kept by the registry for the life of the process, so they must
	// not alias Fiber's request buffer, which every later request overwrites.
	// The first request's labels have now been through five more.
	rec.mu.Lock()
	first := rec.seen[0]
	rec.mu.Unlock()
	if first != cases[0].want {
		t.Errorf("the first request's labels changed under later requests: %+v", first)
	}
}
