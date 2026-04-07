// Package temporaltrace provides an OpenTelemetry tracing interceptor for the
// Temporal Go SDK. It wraps go.temporal.io/sdk/contrib/opentelemetry and uses
// the globally registered TracerProvider set by trace.Init.
//
// Usage:
//
//	interceptor, err := temporaltrace.NewInterceptor()
//	if err != nil {
//	    log.Fatal(err)
//	}
//	c, err := client.Dial(client.Options{
//	    Interceptors: []interceptor.ClientInterceptor{interceptor},
//	})
package temporaltrace

import (
	"go.opentelemetry.io/otel"
	"go.temporal.io/sdk/contrib/opentelemetry"
	"go.temporal.io/sdk/interceptor"
)

// Option configures the Temporal OTel tracing interceptor.
type Option func(*options)

type options struct {
	disableSignalTracing bool
	disableQueryTracing  bool
}

// WithoutSignalTracing disables span creation for signal handling.
// Useful when signals are high-frequency and would create excessive spans.
func WithoutSignalTracing() Option {
	return func(o *options) { o.disableSignalTracing = true }
}

// WithoutQueryTracing disables span creation for query handling.
// Queries are read-only and often high-frequency; disabling reduces
// span volume without losing workflow/activity visibility.
func WithoutQueryTracing() Option {
	return func(o *options) { o.disableQueryTracing = true }
}

// NewInterceptor returns a Temporal client interceptor that creates
// OpenTelemetry spans for workflow, activity, signal, and query operations.
// Uses a tracer from the globally registered TracerProvider.
//
// Security: the Temporal OTel interceptor does NOT record workflow or activity
// input/output arguments as span attributes. Workflow payloads can contain PII
// or credentials and must never appear in trace backends.
func NewInterceptor(opts ...Option) (interceptor.ClientInterceptor, error) {
	var cfg options
	for _, o := range opts {
		o(&cfg)
	}

	tracerOpts := opentelemetry.TracerOptions{
		Tracer:               otel.GetTracerProvider().Tracer("github.com/bravyr/bravyr-obs/temporaltrace"),
		DisableSignalTracing: cfg.disableSignalTracing,
		DisableQueryTracing:  cfg.disableQueryTracing,
	}

	return opentelemetry.NewTracingInterceptor(tracerOpts)
}
