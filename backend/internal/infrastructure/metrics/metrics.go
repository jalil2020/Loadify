// Package metrics exposes what the API and the posting worker are doing in the
// Prometheus text format.
//
// Every label here takes its value from a closed set: a route pattern rather
// than a path, a status, a Telegram error kind rather than its message. A label
// fed from user input or an id would grow a new series for every value, and a
// monitoring system that runs out of memory is worse than one with less detail.
//
// Each process serves its own registry on its own address, never through the
// public proxy: the numbers say nothing secret, but how busy a platform is and
// how often Telegram refuses it is nobody else's business.
package metrics

import (
	"context"
	"errors"
	"net"
	"net/http"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
	"github.com/prometheus/client_golang/prometheus/promhttp"

	"github.com/loadify/backend/pkg/logger"
)

// namespace prefixes every metric this project defines.
const namespace = "loadify"

// NewRegistry returns a registry with the Go runtime and process collectors,
// and a build_info series naming the service and its version.
//
// A registry of our own rather than the package default keeps a library that
// registers something globally from appearing in our output unannounced.
func NewRegistry(service, version string) *prometheus.Registry {
	reg := prometheus.NewRegistry()
	reg.MustRegister(
		collectors.NewGoCollector(),
		collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}),
		prometheus.NewGaugeFunc(prometheus.GaugeOpts{
			Namespace:   namespace,
			Name:        "build_info",
			Help:        "Always 1; the labels say which service and version is running.",
			ConstLabels: prometheus.Labels{"service": service, "version": version},
		}, func() float64 { return 1 }),
	)
	return reg
}

// Serve answers GET /metrics on addr until ctx ends. An empty addr turns it
// off. The listener is opened before Serve returns, so an address that is
// already taken is reported at startup rather than lost in a goroutine.
func Serve(ctx context.Context, addr string, reg *prometheus.Registry, log logger.Logger) error {
	if addr == "" {
		return nil
	}

	listener, err := net.Listen("tcp", addr)
	if err != nil {
		return err
	}

	mux := http.NewServeMux()
	mux.Handle("GET /metrics", promhttp.HandlerFor(reg, promhttp.HandlerOpts{
		// A collector that fails is reported in the scrape and logged, and the
		// rest of the metrics are still served.
		ErrorHandling: promhttp.ContinueOnError,
		ErrorLog:      promLogger{log},
		Registry:      reg,
	}))
	server := &http.Server{
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      30 * time.Second,
	}

	go func() {
		log.Info("metrics listening", logger.String("addr", listener.Addr().String()))
		if err := server.Serve(listener); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Error("metrics server stopped", logger.Err(err))
		}
	}()
	go func() {
		<-ctx.Done()
		shutdown, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = server.Shutdown(shutdown)
	}()
	return nil
}

// promLogger routes the handler's complaints into the structured log.
type promLogger struct{ log logger.Logger }

func (l promLogger) Println(v ...any) {
	l.log.Warn("metrics collection error", logger.Any("error", v))
}

// durationBuckets spans a fast local call to a slow remote one, in seconds.
var durationBuckets = []float64{0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10, 30}
