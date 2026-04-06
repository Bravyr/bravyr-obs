package obs

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/bravyr/bravyr-obs/health"
)

func TestInit_valid(t *testing.T) {
	o, err := Init(Config{
		ServiceName: "test-svc",
		LogLevel:    "info",
	})
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if o == nil {
		t.Fatal("expected non-nil Obs instance")
	}
	o.Shutdown(context.Background())
}

func TestInit_invalidConfig(t *testing.T) {
	_, err := Init(Config{})
	if err == nil {
		t.Fatal("expected error for empty config")
	}
}

func TestMiddleware_passthrough(t *testing.T) {
	o, err := Init(Config{ServiceName: "test-svc", LogLevel: "info"})
	if err != nil {
		t.Fatalf("Init failed: %v", err)
	}

	mw := o.Middleware()
	called := false
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	})

	handler := mw(inner)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	handler.ServeHTTP(rec, req)

	if !called {
		t.Fatal("middleware did not call inner handler")
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
}

func TestHealthHandler_noChecks(t *testing.T) {
	o, err := Init(Config{ServiceName: "test-svc", LogLevel: "info"})
	if err != nil {
		t.Fatalf("Init failed: %v", err)
	}

	handler := o.HealthHandler(nil)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	handler(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	var resp health.Response
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if resp.Status != "healthy" {
		t.Fatalf("expected status healthy, got %q", resp.Status)
	}
}
