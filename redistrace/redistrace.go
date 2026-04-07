// Package redistrace provides OpenTelemetry instrumentation for go-redis clients.
// It wraps github.com/redis/go-redis/extra/redisotel/v9 and uses the globally
// registered TracerProvider set by trace.Init.
//
// Usage:
//
//	rdb := redis.NewClient(&redis.Options{Addr: "localhost:6379"})
//	if err := redistrace.Instrument(rdb); err != nil {
//	    log.Fatal(err)
//	}
package redistrace

import (
	"github.com/redis/go-redis/extra/redisotel/v9"
	"github.com/redis/go-redis/v9"
)

// Option configures the Redis OTel instrumentation.
type Option func(*options)

type options struct {
	dbStatement bool
	metrics     bool
}

// WithDBStatement enables capturing the full Redis command string in the
// db.statement span attribute. Off by default because command arguments
// can contain PII, keys, or auth credentials (including AUTH passwords).
// Enable only in development or when command values are confirmed safe.
func WithDBStatement() Option {
	return func(o *options) { o.dbStatement = true }
}

// WithMetrics enables OpenTelemetry metrics collection in addition to
// tracing. Off by default to keep the instrumentation footprint minimal.
func WithMetrics() Option {
	return func(o *options) { o.metrics = true }
}

// Instrument adds OpenTelemetry tracing hooks to an existing Redis client.
// Unlike pgxtrace and temporaltrace which return new values, this function
// modifies rdb in place because go-redis uses an AddHook model.
//
// Instrument must be called exactly once per client. Calling it multiple
// times registers duplicate hooks and doubles all spans.
//
// By default, db.statement is redacted. Call before issuing any commands.
func Instrument(rdb redis.UniversalClient, opts ...Option) error {
	var cfg options
	for _, o := range opts {
		o(&cfg)
	}

	tracingOpts := []redisotel.TracingOption{
		redisotel.WithDBStatement(cfg.dbStatement),
	}

	if err := redisotel.InstrumentTracing(rdb, tracingOpts...); err != nil {
		return err
	}

	if cfg.metrics {
		if err := redisotel.InstrumentMetrics(rdb); err != nil {
			return err
		}
	}

	return nil
}
