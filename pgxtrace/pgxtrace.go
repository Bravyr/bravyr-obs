// Package pgxtrace provides an OpenTelemetry-instrumented pgx query tracer.
// It wraps github.com/exaring/otelpgx and uses the globally registered
// TracerProvider set by trace.Init.
//
// Usage:
//
//	connConfig, _ := pgx.ParseConfig(databaseURL)
//	connConfig.Tracer = pgxtrace.NewTracer()
//	pool, _ := pgxpool.NewWithConfig(ctx, poolConfig)
package pgxtrace

import (
	"github.com/exaring/otelpgx"
	"github.com/jackc/pgx/v5"
)

// Option configures the pgx query tracer.
type Option func(*options)

type options struct {
	includeQueryParameters bool
}

// WithIncludeQueryParameters enables capturing SQL bind-parameter values
// in span attributes. Off by default because query parameters can contain
// PII or credentials. Enable only in development or when you have confirmed
// that parameter values are safe to record in your trace backend.
func WithIncludeQueryParameters() Option {
	return func(o *options) {
		o.includeQueryParameters = true
	}
}

// NewTracer returns a pgx.QueryTracer that creates OpenTelemetry spans for
// every query, batch, and copy operation. It uses the globally registered
// TracerProvider (set by trace.Init or otel.SetTracerProvider).
//
// SQL query text is captured in the db.statement span attribute with
// parameterized placeholders ($1, $2). Bind-parameter values are redacted
// by default. Span names are trimmed to the SQL operation (e.g., "SELECT",
// "INSERT") to prevent cardinality explosion.
func NewTracer(opts ...Option) pgx.QueryTracer {
	var cfg options
	for _, o := range opts {
		o(&cfg)
	}

	var otelOpts []otelpgx.Option
	otelOpts = append(otelOpts, otelpgx.WithTrimSQLInSpanName())
	if cfg.includeQueryParameters {
		otelOpts = append(otelOpts, otelpgx.WithIncludeQueryParameters())
	}

	return otelpgx.NewTracer(otelOpts...)
}
