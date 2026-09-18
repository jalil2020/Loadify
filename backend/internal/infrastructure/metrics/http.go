package metrics

import (
	"strconv"
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

// HTTP counts and times the API's requests.
type HTTP struct {
	requests *prometheus.CounterVec
	duration *prometheus.HistogramVec
}

// NewHTTP registers the request metrics.
func NewHTTP(reg prometheus.Registerer) *HTTP {
	h := &HTTP{
		requests: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: namespace,
			Subsystem: "http",
			Name:      "requests_total",
			Help:      "Requests answered, by method, route pattern and status code.",
		}, []string{"method", "route", "status"}),
		duration: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Namespace: namespace,
			Subsystem: "http",
			Name:      "request_duration_seconds",
			Help:      "Time to answer a request, by method and route pattern.",
			Buckets:   durationBuckets,
		}, []string{"method", "route"}),
	}
	reg.MustRegister(h.requests, h.duration)
	return h
}

// ObserveRequest records one answered request. route is the pattern the
// request matched, such as /api/v1/load-ads/:id, never the path itself.
func (h *HTTP) ObserveRequest(method, route string, status int, took time.Duration) {
	h.requests.WithLabelValues(method, route, strconv.Itoa(status)).Inc()
	h.duration.WithLabelValues(method, route).Observe(took.Seconds())
}
