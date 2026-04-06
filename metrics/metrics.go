// Package metrics provides a Prometheus metrics registry and HTTP handler.
// It uses a custom registry (not the global DefaultRegisterer) to allow
// multiple isolated registries per test and avoid cross-test pollution.
package metrics

import (
	"context"
	"fmt"

	"github.com/prometheus/client_golang/prometheus"
)

// defaultDurationBuckets are the latency buckets used for the built-in HTTP
// request duration histogram. They cover typical web service latencies from
// 5ms to 10s.
var defaultDurationBuckets = []float64{
	.005, .01, .025, .05, .1, .25, .5, 1, 2.5, 5, 10,
}

// Registry wraps a Prometheus registry with helpers for creating and
// registering metrics scoped to a single service instance.
type Registry struct {
	reg                 *prometheus.Registry
	cfg                 Config
	httpRequestDuration *prometheus.HistogramVec
	httpRequestsTotal   *prometheus.CounterVec
	httpActiveRequests  prometheus.Gauge
}

// Init creates a new metrics Registry and registers the built-in HTTP
// instrumentation metrics. It returns an error if any metric cannot be
// registered (which would indicate a programmer error such as a duplicate name).
func Init(cfg Config) (*Registry, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}

	reg := prometheus.NewRegistry()

	durationName := metricName(cfg.Prefix, "http_request_duration_seconds")
	totalName := metricName(cfg.Prefix, "http_requests_total")
	activeName := metricName(cfg.Prefix, "http_active_requests")

	duration := prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Name:    durationName,
		Help:    "Duration of HTTP requests in seconds, partitioned by method, path, and status code.",
		Buckets: defaultDurationBuckets,
	}, []string{"method", "path", "status_code"})

	total := prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: totalName,
		Help: "Total number of HTTP requests, partitioned by method, path, and status code.",
	}, []string{"method", "path", "status_code"})

	active := prometheus.NewGauge(prometheus.GaugeOpts{
		Name: activeName,
		Help: "Number of HTTP requests currently being processed.",
	})

	for _, c := range []prometheus.Collector{duration, total, active} {
		if err := reg.Register(c); err != nil {
			return nil, fmt.Errorf("registering metric: %w", err)
		}
	}

	return &Registry{
		reg:                 reg,
		cfg:                 cfg,
		httpRequestDuration: duration,
		httpRequestsTotal:   total,
		httpActiveRequests:  active,
	}, nil
}

// NewCounter creates and registers a new CounterVec with the given name, help
// text, and label names. The configured prefix is applied to the name. Returns
// an error if a metric with the same name is already registered.
func (r *Registry) NewCounter(name, help string, labels []string) (*prometheus.CounterVec, error) {
	c := prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: metricName(r.cfg.Prefix, name),
		Help: help,
	}, labels)

	if err := r.reg.Register(c); err != nil {
		return nil, fmt.Errorf("registering counter %q: %w", name, err)
	}

	return c, nil
}

// NewHistogram creates and registers a new HistogramVec with the given name,
// help text, label names, and bucket boundaries. The configured prefix is
// applied to the name. Returns an error if a metric with the same name is
// already registered.
func (r *Registry) NewHistogram(name, help string, labels []string, buckets []float64) (*prometheus.HistogramVec, error) {
	h := prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Name:    metricName(r.cfg.Prefix, name),
		Help:    help,
		Buckets: buckets,
	}, labels)

	if err := r.reg.Register(h); err != nil {
		return nil, fmt.Errorf("registering histogram %q: %w", name, err)
	}

	return h, nil
}

// Shutdown is a no-op and exists for API symmetry with the logger and tracer
// providers so callers can treat all observability components uniformly.
func (r *Registry) Shutdown(_ context.Context) error {
	return nil
}

// metricName joins a prefix and a base name with an underscore.
// If prefix is empty the base name is returned unchanged — no leading
// underscore is added.
func metricName(prefix, base string) string {
	if prefix == "" {
		return base
	}

	return prefix + "_" + base
}
