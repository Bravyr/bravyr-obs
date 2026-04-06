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
)

// Config is the top-level configuration for the observability stack.
type Config = config.Config

// Obs holds the initialized observability providers and exposes
// middleware and health check handlers as methods.
type Obs struct {
	cfg    Config
	logger *obslog.Logger
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

	return &Obs{cfg: cfg, logger: logger}, nil
}

// Logger returns the structured logger. It is safe to use from multiple
// goroutines. The returned pointer is valid until Shutdown is called.
func (o *Obs) Logger() *obslog.Logger { return o.logger }

// Shutdown flushes all telemetry pipelines and releases resources.
func (o *Obs) Shutdown(ctx context.Context) {
	if o.logger != nil {
		_ = o.logger.Shutdown(ctx)
	}
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
