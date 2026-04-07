package config

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestValidate_valid(t *testing.T) {
	cfg := Config{
		ServiceName: "test-svc",
		LogLevel:    "info",
	}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
}

func TestValidate_missingServiceName(t *testing.T) {
	cfg := Config{LogLevel: "info"}
	err := cfg.Validate()
	if err == nil {
		t.Fatal("expected error for missing ServiceName")
	}
	if !strings.Contains(err.Error(), "ServiceName") {
		t.Fatalf("expected error about ServiceName, got: %v", err)
	}
}

func TestValidate_invalidLogLevel(t *testing.T) {
	cfg := Config{ServiceName: "test-svc", LogLevel: "banana"}
	err := cfg.Validate()
	if err == nil {
		t.Fatal("expected error for invalid LogLevel")
	}
	if !strings.Contains(err.Error(), "LogLevel") {
		t.Fatalf("expected error about LogLevel, got: %v", err)
	}
}

func TestValidate_multipleErrors(t *testing.T) {
	cfg := Config{LogLevel: "banana"}
	err := cfg.Validate()
	if err == nil {
		t.Fatal("expected error for multiple violations")
	}
	msg := err.Error()
	if !strings.Contains(msg, "ServiceName") {
		t.Fatalf("expected error about ServiceName, got: %v", err)
	}
	if !strings.Contains(msg, "LogLevel") {
		t.Fatalf("expected error about LogLevel, got: %v", err)
	}
}

func TestValidate_caseInsensitiveLogLevel(t *testing.T) {
	for _, level := range []string{"INFO", "Debug", "WARN"} {
		cfg := Config{ServiceName: "test-svc", LogLevel: level}
		if err := cfg.Validate(); err != nil {
			t.Fatalf("expected %q to be valid, got: %v", level, err)
		}
	}
}

func TestValidate_emptyLogLevel(t *testing.T) {
	cfg := Config{ServiceName: "test-svc", LogLevel: ""}
	err := cfg.Validate()
	if err == nil {
		t.Fatal("expected error for empty LogLevel")
	}
}

func TestString_noSensitiveFields(t *testing.T) {
	cfg := Config{ServiceName: "test-svc", OTLPEndpoint: "otel:4317"}
	s := cfg.String()
	if !strings.Contains(s, "test-svc") {
		t.Fatalf("String() should contain service name, got: %s", s)
	}
}

func TestMarshalJSON_containsExpectedFields(t *testing.T) {
	cfg := Config{ServiceName: "test-svc", LogLevel: "info"}
	data, err := json.Marshal(cfg)
	if err != nil {
		t.Fatalf("MarshalJSON failed: %v", err)
	}
	var out map[string]any
	if err := json.Unmarshal(data, &out); err != nil {
		t.Fatalf("json.Unmarshal failed: %v", err)
	}
	if _, ok := out["service_name"]; !ok {
		t.Fatal("MarshalJSON must include service_name")
	}
	if _, ok := out["log_level"]; !ok {
		t.Fatal("MarshalJSON must include log_level")
	}
}

func TestValidate_devModeProductionError(t *testing.T) {
	cfg := Config{
		ServiceName: "test-svc",
		LogLevel:    "info",
		DevMode:     true,
		Environment: "production",
	}
	err := cfg.Validate()
	if err == nil {
		t.Fatal("expected error for DevMode=true in production environment")
	}
	if !strings.Contains(err.Error(), "DevMode") {
		t.Fatalf("expected error about DevMode, got: %v", err)
	}
}

func TestValidate_devModeProductionCaseInsensitive(t *testing.T) {
	for _, env := range []string{"Production", "PRODUCTION", "production"} {
		cfg := Config{
			ServiceName: "test-svc",
			LogLevel:    "info",
			DevMode:     true,
			Environment: env,
		}
		err := cfg.Validate()
		if err == nil {
			t.Fatalf("expected error for DevMode=true with Environment=%q", env)
		}
	}
}

func TestValidate_devModeDevelopmentOK(t *testing.T) {
	cfg := Config{
		ServiceName: "test-svc",
		LogLevel:    "info",
		DevMode:     true,
		Environment: "development",
	}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("expected no error for DevMode=true in development, got: %v", err)
	}
}

func TestValidate_devModeEmptyEnvironmentOK(t *testing.T) {
	cfg := Config{
		ServiceName: "test-svc",
		LogLevel:    "info",
		DevMode:     true,
		Environment: "",
	}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("expected no error for DevMode=true with empty Environment, got: %v", err)
	}
}

func TestValidate_sampleRateValid(t *testing.T) {
	for _, rate := range []float64{0.0, 0.5, 1.0} {
		cfg := Config{
			ServiceName: "test-svc",
			LogLevel:    "info",
			SampleRate:  rate,
		}
		if err := cfg.Validate(); err != nil {
			t.Fatalf("expected SampleRate %g to be valid, got: %v", rate, err)
		}
	}
}

func TestValidate_sampleRateInvalid(t *testing.T) {
	for _, rate := range []float64{-0.1, 1.1} {
		cfg := Config{
			ServiceName: "test-svc",
			LogLevel:    "info",
			SampleRate:  rate,
		}
		err := cfg.Validate()
		if err == nil {
			t.Fatalf("expected error for SampleRate %g", rate)
		}
		if !strings.Contains(err.Error(), "SampleRate") {
			t.Fatalf("expected error about SampleRate for rate %g, got: %v", rate, err)
		}
	}
}

func TestValidate_metricsPrefix(t *testing.T) {
	cases := []struct {
		name   string
		prefix string
	}{
		{"empty prefix is valid", ""},
		{"non-empty prefix is valid", "myapp"},
		{"prefix with underscores is valid", "my_app"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cfg := Config{
				ServiceName:   "test-svc",
				LogLevel:      "info",
				MetricsPrefix: tc.prefix,
			}
			if err := cfg.Validate(); err != nil {
				t.Fatalf("expected no error for MetricsPrefix %q, got: %v", tc.prefix, err)
			}
		})
	}
}

func TestValidate_otlpLoopbackRejected(t *testing.T) {
	for _, ep := range []string{"127.0.0.1:4317", "127.0.1.1:4317"} {
		cfg := Config{ServiceName: "svc", LogLevel: "info", OTLPEndpoint: ep}
		if err := cfg.Validate(); err == nil {
			t.Fatalf("expected error for loopback endpoint %q", ep)
		}
	}
}

func TestValidate_otlpLinkLocalRejected(t *testing.T) {
	cfg := Config{ServiceName: "svc", LogLevel: "info", OTLPEndpoint: "169.254.169.254:4317"}
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected error for link-local endpoint")
	}
}

func TestValidate_otlpLoopbackAllowedInDev(t *testing.T) {
	cfg := Config{ServiceName: "svc", LogLevel: "info", OTLPEndpoint: "127.0.0.1:4317", DevMode: true}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("expected no error for loopback in DevMode, got: %v", err)
	}
}

func TestValidate_otlpPrivateIPAllowed(t *testing.T) {
	for _, ep := range []string{"10.0.0.1:4317", "192.168.1.1:4317", "172.18.0.5:4317"} {
		cfg := Config{ServiceName: "svc", LogLevel: "info", OTLPEndpoint: ep}
		if err := cfg.Validate(); err != nil {
			t.Fatalf("expected no error for private IP %q, got: %v", ep, err)
		}
	}
}

func TestValidate_otlpHostnameAllowed(t *testing.T) {
	cfg := Config{ServiceName: "svc", LogLevel: "info", OTLPEndpoint: "otel-collector:4317"}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("expected no error for hostname, got: %v", err)
	}
}

func TestValidate_otlpLoopbackNoPortRejected(t *testing.T) {
	cfg := Config{ServiceName: "svc", LogLevel: "info", OTLPEndpoint: "127.0.0.1"}
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected error for loopback without port")
	}
}

func TestValidate_otlpIPv6LoopbackRejected(t *testing.T) {
	cfg := Config{ServiceName: "svc", LogLevel: "info", OTLPEndpoint: "[::1]:4317"}
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected error for IPv6 loopback")
	}
}

func TestValidate_otlpPublicIPAllowed(t *testing.T) {
	cfg := Config{ServiceName: "svc", LogLevel: "info", OTLPEndpoint: "203.0.113.1:4317"}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("expected no error for public IP, got: %v", err)
	}
}
