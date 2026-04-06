// Package obs is an opinionated observability library for Go services.
// It provides structured logging, distributed tracing, and Prometheus metrics
// in a single Init() call.
package obs

import (
	"context"
	"net/http"

	"github.com/bravyr/bravyr-obs/config"
	"github.com/bravyr/bravyr-obs/health"
)

// Config is the top-level configuration for the observability stack.
type Config = config.Config

// Obs holds the initialized observability providers and exposes
// middleware and health check handlers as methods.
type Obs struct {
	cfg Config
}

// Init initializes logging, tracing, and metrics based on the provided
// configuration. Call Shutdown before process exit to flush all telemetry.
func Init(cfg Config) (*Obs, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}

	return &Obs{cfg: cfg}, nil
}

// Shutdown flushes all telemetry pipelines and releases resources.
func (o *Obs) Shutdown(ctx context.Context) {
	// Phases 1-2: flush log, trace, and metrics pipelines.
}

// Middleware returns an http.Handler middleware that adds request logging,
// trace propagation, and request metrics to every request.
func (o *Obs) Middleware() func(http.Handler) http.Handler {
	// Phase 3: compose log, trace, and metrics middleware.
	return func(next http.Handler) http.Handler { return next }
}

// HealthHandler returns an http.HandlerFunc that executes the given named
// health checks and responds with a JSON health report.
func (o *Obs) HealthHandler(checks map[string]health.CheckFunc) http.HandlerFunc {
	return health.Handler(checks)
}
