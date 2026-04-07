package pgxtrace

import (
	"testing"
)

func TestNewTracer_returnsQueryTracer(t *testing.T) {
	tracer := NewTracer()
	if tracer == nil {
		t.Fatal("expected non-nil QueryTracer")
	}
	// Compile-time interface check via return type of NewTracer.
	_ = tracer
}

func TestNewTracer_withIncludeQueryParameters(t *testing.T) {
	tracer := NewTracer(WithIncludeQueryParameters())
	if tracer == nil {
		t.Fatal("expected non-nil QueryTracer with params option")
	}
}

func TestNewTracer_defaultNoParams(t *testing.T) {
	// Default tracer should be created without panic and without params.
	tracer := NewTracer()
	if tracer == nil {
		t.Fatal("expected non-nil QueryTracer")
	}
}
