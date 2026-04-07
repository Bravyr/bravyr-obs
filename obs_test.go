package obs

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/bravyr/bravyr-obs/health"
	obsmw "github.com/bravyr/bravyr-obs/middleware"
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

func TestInit_createsLogger(t *testing.T) {
	o, err := Init(Config{ServiceName: "test-svc", LogLevel: "info"})
	if err != nil {
		t.Fatalf("Init() returned error: %v", err)
	}
	if o.Logger() == nil {
		t.Fatal("Logger() returned nil after Init")
	}
	o.Shutdown(context.Background())
}

func TestShutdown_flushesLogger(t *testing.T) {
	o, err := Init(Config{ServiceName: "test-svc", LogLevel: "info"})
	if err != nil {
		t.Fatalf("Init() returned error: %v", err)
	}
	// Shutdown must complete without panicking or blocking.
	o.Shutdown(context.Background())
}

func TestInit_allValidLevels(t *testing.T) {
	for _, level := range []string{"debug", "info", "warn", "error", "fatal"} {
		t.Run(level, func(t *testing.T) {
			o, err := Init(Config{ServiceName: "test-svc", LogLevel: level})
			if err != nil {
				t.Fatalf("Init(%q) returned error: %v", level, err)
			}
			o.Shutdown(context.Background())
		})
	}
}

func TestInit_createsTracer(t *testing.T) {
	o, err := Init(Config{ServiceName: "test-svc", LogLevel: "info"})
	if err != nil {
		t.Fatalf("Init() returned error: %v", err)
	}
	defer o.Shutdown(context.Background())

	if o.TracerProvider() == nil {
		t.Fatal("TracerProvider() returned nil after Init")
	}
}

func TestShutdown_flushesTracer(t *testing.T) {
	o, err := Init(Config{ServiceName: "test-svc", LogLevel: "info"})
	if err != nil {
		t.Fatalf("Init() returned error: %v", err)
	}
	// Shutdown must complete without panicking, blocking, or returning an error
	// even when no OTLP endpoint is configured (no-op tracer path).
	o.Shutdown(context.Background())
}

func TestInit_createsMetrics(t *testing.T) {
	o, err := Init(Config{ServiceName: "test-svc", LogLevel: "info"})
	if err != nil {
		t.Fatalf("Init() returned error: %v", err)
	}
	defer o.Shutdown(context.Background())

	if o.Metrics() == nil {
		t.Fatal("Metrics() returned nil after Init")
	}
}

func TestMetricsHandler(t *testing.T) {
	o, err := Init(Config{ServiceName: "test-svc", LogLevel: "info"})
	if err != nil {
		t.Fatalf("Init() returned error: %v", err)
	}
	defer o.Shutdown(context.Background())

	h := o.MetricsHandler()
	if h == nil {
		t.Fatal("MetricsHandler() returned nil")
	}

	// Verify the handler serves a valid 200 response.
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 from MetricsHandler(), got %d", rec.Code)
	}
}

func TestMiddleware_composedBundle(t *testing.T) {
	o, err := Init(Config{ServiceName: "test-svc", LogLevel: "info"})
	if err != nil {
		t.Fatalf("Init failed: %v", err)
	}
	defer o.Shutdown(context.Background())

	mw := o.Middleware()
	if mw == nil {
		t.Fatal("Middleware() returned nil")
	}

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

func TestMiddlewareWithConfig_selective(t *testing.T) {
	o, err := Init(Config{ServiceName: "test-svc", LogLevel: "info"})
	if err != nil {
		t.Fatalf("Init failed: %v", err)
	}
	defer o.Shutdown(context.Background())

	// Disable tracing and metrics — only logging enabled.
	mw := o.MiddlewareWithConfig(obsmw.BundleConfig{
		Tracing: false,
		Logging: true,
		Metrics: false,
	})
	if mw == nil {
		t.Fatal("MiddlewareWithConfig() returned nil")
	}

	called := false
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusNoContent)
	})

	handler := mw(inner)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	handler.ServeHTTP(rec, req)

	if !called {
		t.Fatal("MiddlewareWithConfig handler did not call inner handler")
	}
	if rec.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d", rec.Code)
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
