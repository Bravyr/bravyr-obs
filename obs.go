// Package obs is an opinionated observability library for Go services.
// It provides structured logging, distributed tracing, and Prometheus metrics
// in a single Init() call.
package obs

import (
	"context"
	"fmt"
	"net/http"

	"github.com/bravyr/bravyr-obs/config"
	"github.com/bravyr/bravyr-obs/health"
	obslog "github.com/bravyr/bravyr-obs/log"
	obstrace "github.com/bravyr/bravyr-obs/trace"
)

// Config is the top-level configuration for the observability stack.
type Config = config.Config

// Obs holds the initialized observability providers and exposes
// middleware and health check handlers as methods.
type Obs struct {
	cfg    Config
	logger *obslog.Logger
	tracer *obstrace.Provider
}

// Init initializes logging, tracing, and metrics based on the provided
// configuration. Call Shutdown before process exit to flush all telemetry.
func Init(cfg Config) (*Obs, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}

	logger, err := obslog.New(obslog.Config{
		Level:       cfg.LogLevel,
		SeqURL:      cfg.SeqURL,
		SeqAPIKey:   cfg.SeqAPIKey,
		ServiceName: cfg.ServiceName,
		DevMode:     cfg.DevMode,
	})
	if err != nil {
		return nil, fmt.Errorf("log init: %w", err)
	}

	tp, err := obstrace.Init(context.Background(), obstrace.Config{
		ServiceName:  cfg.ServiceName,
		Environment:  cfg.Environment,
		OTLPEndpoint: cfg.OTLPEndpoint,
		SampleRate:   cfg.SampleRate,
		DevMode:      cfg.DevMode,
	})
	if err != nil {
		// Logger was successfully constructed; shut it down before returning
		// so the caller does not receive a partially-initialized Obs with a
		// leaking Seq writer goroutine.
		_ = logger.Shutdown(context.Background())
		return nil, fmt.Errorf("trace init: %w", err)
	}

	return &Obs{cfg: cfg, logger: logger, tracer: tp}, nil
}

// Logger returns the structured logger. It is safe to use from multiple
// goroutines. The returned pointer is valid until Shutdown is called.
func (o *Obs) Logger() *obslog.Logger { return o.logger }

// TracerProvider returns the trace provider. The internal SDK provider may be
// nil when no OTLPEndpoint was configured; callers that need to instrument
// code directly should use otel.GetTracerProvider() which always returns a
// valid (possibly no-op) provider.
func (o *Obs) TracerProvider() *obstrace.Provider { return o.tracer }

// Shutdown flushes all telemetry pipelines and releases resources.
// The tracer is shut down before the logger so any final span-related log
// lines can still be emitted during the tracer drain.
func (o *Obs) Shutdown(ctx context.Context) {
	if o.tracer != nil {
		_ = o.tracer.Shutdown(ctx)
	}
	if o.logger != nil {
		_ = o.logger.Shutdown(ctx)
	}
}

// Middleware returns an http.Handler middleware that adds trace propagation
// to every request. The span is named after the matched Chi route pattern.
func (o *Obs) Middleware() func(http.Handler) http.Handler {
	return obstrace.HTTPMiddleware(o.cfg.ServiceName)
}

// HealthHandler returns an http.HandlerFunc that executes the given named
// health checks and responds with a JSON health report.
func (o *Obs) HealthHandler(checks map[string]health.CheckFunc) http.HandlerFunc {
	return health.Handler(checks)
}
