// Package trace provides OpenTelemetry tracer setup with OTLP/gRPC export.
package trace

import (
	"context"
	"errors"
	"fmt"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"
)

// Provider wraps the OTel SDK TracerProvider and carries shutdown semantics.
// A nil internal tp indicates a no-op provider (no OTLP endpoint configured).
type Provider struct {
	tp *sdktrace.TracerProvider
}

// Init validates cfg and creates a TracerProvider connected to the OTLP/gRPC
// endpoint specified in cfg.OTLPEndpoint. When OTLPEndpoint is empty, Init
// returns a no-op Provider — no spans are exported and no network connection
// is made. This allows callers to configure tracing at runtime without
// branching on whether an endpoint is set.
//
// On success, Init registers the provider globally via otel.SetTracerProvider
// and installs the W3C TraceContext + Baggage propagators.
func Init(ctx context.Context, cfg Config) (*Provider, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}

	// No endpoint — return a no-op provider. The global OTel provider stays
	// as the default no-op, which is safe: spans are created but immediately
	// dropped. Callers do not need to check for nil TracerProvider().
	if cfg.OTLPEndpoint == "" {
		return &Provider{tp: nil}, nil
	}

	opts := []otlptracegrpc.Option{
		otlptracegrpc.WithEndpoint(cfg.OTLPEndpoint),
	}
	if cfg.DevMode || cfg.OTLPInsecure {
		// Insecure (plaintext) transport for local/internal collectors.
		// DevMode always enables it; OTLPInsecure enables it independently
		// for production services connecting to internal collectors.
		opts = append(opts, otlptracegrpc.WithInsecure())
	}

	exporter, err := otlptracegrpc.New(ctx, opts...)
	if err != nil {
		return nil, fmt.Errorf("trace: create OTLP exporter: %w", err)
	}

	res, err := resource.New(ctx,
		resource.WithAttributes(
			semconv.ServiceName(cfg.ServiceName),
			semconv.DeploymentEnvironment(cfg.Environment),
		),
	)
	if err != nil && !errors.Is(err, resource.ErrPartialResource) {
		return nil, fmt.Errorf("trace: create resource: %w", err)
	}

	tp := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exporter),
		sdktrace.WithResource(res),
		sdktrace.WithSampler(buildSampler(cfg)),
	)

	// Register globally so instrumentation libraries (otelhttp, otelgrpc, etc.)
	// can obtain a tracer without an explicit provider reference.
	otel.SetTracerProvider(tp)
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	))

	return &Provider{tp: tp}, nil
}

// buildSampler returns the appropriate sdktrace.Sampler for cfg.
// DevMode always samples. Rates at the extremes map to the SDK constants
// rather than ratio-based samplers, which avoids floating-point edge cases.
func buildSampler(cfg Config) sdktrace.Sampler {
	if cfg.DevMode {
		return sdktrace.AlwaysSample()
	}

	switch {
	case cfg.SampleRate <= 0:
		return sdktrace.NeverSample()
	case cfg.SampleRate >= 1:
		return sdktrace.AlwaysSample()
	default:
		// ParentBased respects the sampling decision of upstream callers.
		// When a parent span is sampled, the child is also sampled — this
		// avoids broken traces caused by local sub-sampling.
		return sdktrace.ParentBased(sdktrace.TraceIDRatioBased(cfg.SampleRate))
	}
}

// Shutdown flushes all pending spans and releases the exporter connection.
// It is a no-op when the Provider was created without an OTLPEndpoint.
func (p *Provider) Shutdown(ctx context.Context) error {
	if p.tp != nil {
		return p.tp.Shutdown(ctx)
	}
	return nil
}

// TracerProvider returns the underlying SDK TracerProvider. The return value
// is nil when no OTLPEndpoint was configured (no-op mode). Callers that need
// to pass a provider explicitly can use otel.GetTracerProvider() instead,
// which always returns a valid (possibly no-op) provider.
func (p *Provider) TracerProvider() *sdktrace.TracerProvider {
	return p.tp
}
