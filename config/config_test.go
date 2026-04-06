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

func TestString_redactsAPIKey(t *testing.T) {
	cfg := Config{
		ServiceName: "test-svc",
		SeqAPIKey:   "super-secret-key",
	}
	s := cfg.String()
	if strings.Contains(s, "super-secret-key") {
		t.Fatal("String() must not contain the actual API key")
	}
	if !strings.Contains(s, "***") {
		t.Fatal("String() should show redacted API key as ***")
	}
}

func TestString_emptyAPIKey(t *testing.T) {
	cfg := Config{ServiceName: "test-svc"}
	s := cfg.String()
	if strings.Contains(s, "***") {
		t.Fatal("String() should not show *** when API key is empty")
	}
}

func TestMarshalJSON_redactsAPIKey(t *testing.T) {
	cfg := Config{
		ServiceName: "test-svc",
		SeqAPIKey:   "super-secret-key",
	}
	data, err := json.Marshal(cfg)
	if err != nil {
		t.Fatalf("MarshalJSON failed: %v", err)
	}
	if strings.Contains(string(data), "super-secret-key") {
		t.Fatal("MarshalJSON must not contain the actual API key")
	}
	if !strings.Contains(string(data), "***") {
		t.Fatal("MarshalJSON should show redacted API key as ***")
	}
}

func TestMarshalJSON_emptyAPIKey(t *testing.T) {
	cfg := Config{ServiceName: "test-svc"}
	data, err := json.Marshal(cfg)
	if err != nil {
		t.Fatalf("MarshalJSON failed: %v", err)
	}
	if strings.Contains(string(data), "***") {
		t.Fatal("MarshalJSON should not show *** when API key is empty")
	}
}
