package trace

import (
	"errors"
	"fmt"
	"strings"
)

// Config holds the settings required to initialize the trace provider.
// It is intentionally separate from config.Config so the trace package
// can be imported and tested in isolation.
type Config struct {
	// ServiceName is the logical name of the service emitting spans.
	// Required — Init returns an error when empty.
	ServiceName string

	// Environment is the deployment environment (e.g. "production", "development").
	Environment string

	// OTLPEndpoint is the host:port of the OTLP/gRPC collector.
	// When empty, Init returns a no-op Provider and does not connect to any backend.
	OTLPEndpoint string

	// SampleRate controls the fraction of traces that are sampled.
	// Valid range is [0.0, 1.0]. DevMode overrides this to AlwaysSample.
	SampleRate float64

	// DevMode enables AlwaysSample and insecure (plaintext) gRPC transport.
	// Must not be true in production.
	DevMode bool
}

// Validate returns an error if the Config is not usable by Init.
func (c Config) Validate() error {
	var errs []error

	if c.ServiceName == "" {
		errs = append(errs, errors.New("trace: ServiceName is required"))
	}

	// SampleRate range mirrors config.Config.Validate — keep both in sync.
	if c.SampleRate < 0 || c.SampleRate > 1 {
		errs = append(errs, fmt.Errorf("trace: SampleRate must be between 0.0 and 1.0, got %f", c.SampleRate))
	}

	if c.DevMode && strings.EqualFold(c.Environment, "production") {
		errs = append(errs, errors.New("trace: DevMode must not be enabled in production environment"))
	}

	return errors.Join(errs...)
}
