package middleware

import (
	"strings"
	"sync"
	"time"

	"github.com/gofiber/fiber/v2"
)

// RequestObserver records answered requests. The metrics package implements it,
// so this package does not depend on any particular monitoring system.
type RequestObserver interface {
	ObserveRequest(method, route string, status int, took time.Duration)
}

// unmatchedRoute labels a request no endpoint took: a typo, or a scanner
// probing paths. Labelling those by their path would let anyone on the
// internet mint a new time series with every request.
const unmatchedRoute = "unmatched"

// knownMethods are labelled as themselves; anything else is "other", for the
// same reason as unmatched paths.
//
// The label is always the constant from this map, never c.Method() itself:
// Fiber's strings point into a request buffer that is reused for the next
// request, and a label kept by the metrics registry would change under it.
var knownMethods = map[string]string{
	fiber.MethodGet: fiber.MethodGet, fiber.MethodHead: fiber.MethodHead,
	fiber.MethodPost: fiber.MethodPost, fiber.MethodPut: fiber.MethodPut,
	fiber.MethodPatch: fiber.MethodPatch, fiber.MethodDelete: fiber.MethodDelete,
	fiber.MethodOptions: fiber.MethodOptions,
}

// Metrics reports every request by the route pattern it matched and the status
// it ended with.
//
// A request an endpoint answered is labelled with that endpoint's pattern, such
// as /api/v1/load-ads/:id. One that a group's middleware stopped (a missing
// token on /api/v1/wallet, say) is labelled with the group, /api/v1/wallet/*,
// and so is a path under a group that no endpoint matched. Both come from the
// routes the application registered, so the set of labels is fixed.
//
// Register it right after the request id, so a request refused by a later
// middleware (the rate limit, authentication) is counted too.
func Metrics(app *fiber.App, observer RequestObserver) fiber.Handler {
	// Fiber only tells an endpoint from a Use prefix through GetRoutes, and
	// every route is registered by the time the first request arrives.
	var (
		once   sync.Once
		labels map[string]string
	)
	labelFor := func(method, path string) string {
		once.Do(func() {
			labels = map[string]string{}
			for _, route := range app.GetRoutes() {
				labels[route.Method+" "+route.Path] = strings.TrimSuffix(route.Path, "/") + "/*"
			}
			for _, route := range app.GetRoutes(true) {
				labels[route.Method+" "+route.Path] = route.Path
			}
		})
		if label, ok := labels[method+" "+path]; ok {
			return label
		}
		return unmatchedRoute
	}

	return func(c *fiber.Ctx) error {
		start := time.Now()
		err := c.Next()

		route := unmatchedRoute
		if r := c.Route(); r != nil {
			route = labelFor(r.Method, r.Path)
		}
		method, known := knownMethods[c.Method()]
		if !known {
			method = "other"
		}
		observer.ObserveRequest(method, route, statusOf(c, err), time.Since(start))
		return err
	}
}
