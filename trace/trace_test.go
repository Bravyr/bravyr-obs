package trace

import (
	"context"
	"strings"
	"testing"

	sdktrace "go.opentelemetry.io/otel/sdk/trace"
)

// TestInit_noEndpoint verifies that an empty OTLPEndpoint results in a valid
// no-op Provider that can be shut down safely without any network activity.
func TestInit_noEndpoint(t *testing.T) {
	p, err := Init(context.Background(), Config{
		ServiceName: "test-svc",
		SampleRate:  1.0,
	})
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if p == nil {
		t.Fatal("expected non-nil Provider")
	}
	if p.TracerProvider() != nil {
		t.Fatal("expected nil TracerProvider for no-op provider")
	}
	if err := p.Shutdown(context.Background()); err != nil {
		t.Fatalf("Shutdown returned error: %v", err)
	}
}

// TestInit_missingServiceName verifies that a missing ServiceName returns an
// error before any network connections are attempted.
func TestInit_missingServiceName(t *testing.T) {
	_, err := Init(context.Background(), Config{
		SampleRate: 1.0,
	})
	if err == nil {
		t.Fatal("expected error for missing ServiceName")
	}
	if !strings.Contains(err.Error(), "ServiceName") {
		t.Fatalf("expected error mentioning ServiceName, got: %v", err)
	}
}

// TestInit_invalidSampleRate verifies that out-of-range SampleRate values are
// rejected at validation time.
func TestInit_invalidSampleRate(t *testing.T) {
	cases := []struct {
		name string
		rate float64
	}{
		{"negative", -0.1},
		{"above one", 1.1},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := Init(context.Background(), Config{
				ServiceName: "test-svc",
				SampleRate:  tc.rate,
			})
			if err == nil {
				t.Fatalf("expected error for SampleRate %f", tc.rate)
			}
		})
	}
}

// TestBuildSampler_devMode verifies that DevMode produces an AlwaysSample sampler
// regardless of the SampleRate field.
func TestBuildSampler_devMode(t *testing.T) {
	s := buildSampler(Config{DevMode: true, SampleRate: 0.1})
	if s != sdktrace.AlwaysSample() {
		t.Fatalf("expected AlwaysSample in DevMode, got %T", s)
	}
}

// TestBuildSampler_zeroRate verifies that SampleRate 0 maps to NeverSample.
func TestBuildSampler_zeroRate(t *testing.T) {
	s := buildSampler(Config{SampleRate: 0})
	if s != sdktrace.NeverSample() {
		t.Fatalf("expected NeverSample for rate=0, got %T", s)
	}
}

// TestBuildSampler_fullRate verifies that SampleRate 1 maps to AlwaysSample.
func TestBuildSampler_fullRate(t *testing.T) {
	s := buildSampler(Config{SampleRate: 1.0})
	if s != sdktrace.AlwaysSample() {
		t.Fatalf("expected AlwaysSample for rate=1.0, got %T", s)
	}
}

// TestBuildSampler_ratioRate verifies that a fractional SampleRate produces a
// ParentBased sampler (the concrete type is not exported, so we inspect the
// description string which the SDK documents as stable for this purpose).
func TestBuildSampler_ratioRate(t *testing.T) {
	s := buildSampler(Config{SampleRate: 0.5})
	desc := s.Description()
	if !strings.HasPrefix(desc, "ParentBased") {
		t.Fatalf("expected ParentBased description for rate=0.5, got %q", desc)
	}
}

// TestShutdown_nilProvider verifies that calling Shutdown on a no-op Provider
// (OTLPEndpoint was empty) does not panic.
func TestShutdown_nilProvider(t *testing.T) {
	p := &Provider{tp: nil}
	if err := p.Shutdown(context.Background()); err != nil {
		t.Fatalf("expected nil error from no-op Shutdown, got: %v", err)
	}
}

// TestConfigValidate covers all Config.Validate() branches exhaustively.
func TestConfigValidate(t *testing.T) {
	cases := []struct {
		name    string
		cfg     Config
		wantErr bool
		errText string
	}{
		{
			name:    "valid minimal",
			cfg:     Config{ServiceName: "svc", SampleRate: 1.0},
			wantErr: false,
		},
		{
			name:    "valid zero rate",
			cfg:     Config{ServiceName: "svc", SampleRate: 0},
			wantErr: false,
		},
		{
			name:    "valid half rate",
			cfg:     Config{ServiceName: "svc", SampleRate: 0.5},
			wantErr: false,
		},
		{
			name:    "missing service name",
			cfg:     Config{SampleRate: 1.0},
			wantErr: true,
			errText: "ServiceName",
		},
		{
			name:    "negative sample rate",
			cfg:     Config{ServiceName: "svc", SampleRate: -0.1},
			wantErr: true,
			errText: "SampleRate",
		},
		{
			name:    "sample rate above one",
			cfg:     Config{ServiceName: "svc", SampleRate: 1.1},
			wantErr: true,
			errText: "SampleRate",
		},
		{
			name:    "multiple errors",
			cfg:     Config{SampleRate: -1.0},
			wantErr: true,
			errText: "ServiceName",
		},
		{
			name:    "devmode in production",
			cfg:     Config{ServiceName: "svc", SampleRate: 1.0, DevMode: true, Environment: "production"},
			wantErr: true,
			errText: "DevMode",
		},
		{
			name:    "devmode in production case insensitive",
			cfg:     Config{ServiceName: "svc", SampleRate: 1.0, DevMode: true, Environment: "Production"},
			wantErr: true,
			errText: "DevMode",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.cfg.Validate()
			if tc.wantErr && err == nil {
				t.Fatal("expected error, got nil")
			}
			if !tc.wantErr && err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if tc.wantErr && tc.errText != "" && !strings.Contains(err.Error(), tc.errText) {
				t.Fatalf("expected error containing %q, got: %v", tc.errText, err)
			}
		})
	}
}
