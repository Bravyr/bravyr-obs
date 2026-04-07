package middleware

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/go-chi/chi/v5"
	"go.opentelemetry.io/otel"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"

	obslog "github.com/bravyr/bravyr-obs/log"
	"github.com/bravyr/bravyr-obs/metrics"
)

// withTestProvider installs an in-memory TracerProvider as the global OTel
// provider for the duration of the test and restores the original on cleanup.
// NOT safe for t.Parallel() — mutates the global OTel provider.
func withTestProvider(t *testing.T) *tracetest.SpanRecorder {
	t.Helper()
	sr := tracetest.NewSpanRecorder()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(sr))
	prev := otel.GetTracerProvider()
	otel.SetTracerProvider(tp)
	t.Cleanup(func() {
		_ = tp.Shutdown(t.Context())
		otel.SetTracerProvider(prev)
	})
	return sr
}

// newTestLogger builds a Logger in dev mode so tests can inspect console
// output without requiring a running log backend.
func newTestLogger(t *testing.T) *obslog.Logger {
	t.Helper()
	l, err := obslog.New(obslog.Config{ServiceName: "test", Level: "debug", DevMode: true})
	if err != nil {
		t.Fatalf("obslog.New: %v", err)
	}
	return l
}

// newTestRegistry returns an isolated Prometheus registry.
func newTestRegistry(t *testing.T) *metrics.Registry {
	t.Helper()
	reg, err := metrics.Init(metrics.Config{})
	if err != nil {
		t.Fatalf("metrics.Init: %v", err)
	}
	return reg
}

// scrapeMetrics hits the Prometheus handler and returns the text body.
func scrapeMetrics(t *testing.T, reg *metrics.Registry) string {
	t.Helper()
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	reg.Handler().ServeHTTP(rec, req)
	body, err := io.ReadAll(rec.Body)
	if err != nil {
		t.Fatalf("reading /metrics body: %v", err)
	}
	return string(body)
}

func TestBundle_allEnabled(t *testing.T) {
	sr := withTestProvider(t)
	logger := newTestLogger(t)
	reg := newTestRegistry(t)

	router := chi.NewRouter()
	router.Use(Bundle("test-svc", logger, reg, DefaultBundleConfig()))
	router.Get("/ping", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/ping", nil)
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	// Metrics recorded.
	body := scrapeMetrics(t, reg)
	if !strings.Contains(body, "http_requests_total") {
		t.Errorf("expected http_requests_total in metrics output:\n%s", body)
	}

	// At least one span was created.
	if len(sr.Ended()) == 0 {
		t.Error("expected at least one span")
	}

	// X-Request-ID header set.
	if rec.Header().Get("X-Request-ID") == "" {
		t.Error("expected X-Request-ID response header to be set")
	}
}

func TestBundle_tracingDisabled(t *testing.T) {
	sr := withTestProvider(t)
	logger := newTestLogger(t)
	reg := newTestRegistry(t)

	router := chi.NewRouter()
	router.Use(Bundle("test-svc", logger, reg, BundleConfig{Tracing: false, Logging: true, Metrics: true}))
	router.Get("/ping", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/ping", nil)
	router.ServeHTTP(rec, req)

	// No spans created when tracing is disabled.
	if len(sr.Ended()) != 0 {
		t.Errorf("expected no spans when tracing disabled, got %d", len(sr.Ended()))
	}

	// Metrics still recorded.
	body := scrapeMetrics(t, reg)
	if !strings.Contains(body, "http_requests_total") {
		t.Errorf("expected http_requests_total even with tracing disabled:\n%s", body)
	}
}

func TestBundle_loggingDisabled(t *testing.T) {
	sr := withTestProvider(t)
	reg := newTestRegistry(t)

	router := chi.NewRouter()
	// nil logger, logging disabled — must not panic.
	router.Use(Bundle("test-svc", nil, reg, BundleConfig{Tracing: true, Logging: false, Metrics: true}))
	router.Get("/ping", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/ping", nil)
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	// Tracing still active.
	if len(sr.Ended()) == 0 {
		t.Error("expected at least one span with logging disabled")
	}

	// Metrics still recorded.
	body := scrapeMetrics(t, reg)
	if !strings.Contains(body, "http_requests_total") {
		t.Errorf("expected http_requests_total even with logging disabled:\n%s", body)
	}
}

func TestBundle_metricsDisabled(t *testing.T) {
	sr := withTestProvider(t)
	logger := newTestLogger(t)

	router := chi.NewRouter()
	// nil registry, metrics disabled — must not panic.
	router.Use(Bundle("test-svc", logger, nil, BundleConfig{Tracing: true, Logging: true, Metrics: false}))
	router.Get("/ping", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/ping", nil)
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	// Tracing still active.
	if len(sr.Ended()) == 0 {
		t.Error("expected at least one span with metrics disabled")
	}
}

func TestBundle_nilDependencies(t *testing.T) {
	// nil logger and nil registry with all components enabled must not panic.
	router := chi.NewRouter()
	router.Use(Bundle("test-svc", nil, nil, DefaultBundleConfig()))
	router.Get("/ping", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/ping", nil)

	// Panicking would cause the test to fail automatically; wrapping with
	// recover makes the failure message clearer.
	func() {
		defer func() {
			if r := recover(); r != nil {
				t.Fatalf("Bundle panicked with nil dependencies: %v", r)
			}
		}()
		router.ServeHTTP(rec, req)
	}()

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
}

func TestBundle_statusCodeShared(t *testing.T) {
	// The same statusWriter must feed both metrics and logging: the status code
	// in the Prometheus label must match what was actually written.
	reg := newTestRegistry(t)
	logger := newTestLogger(t)

	router := chi.NewRouter()
	router.Use(Bundle("svc", logger, reg, BundleConfig{Tracing: false, Logging: true, Metrics: true}))
	router.Get("/err", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	})

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/err", nil)
	router.ServeHTTP(rec, req)

	body := scrapeMetrics(t, reg)
	if !strings.Contains(body, `status_code="500"`) {
		t.Errorf("expected status_code=\"500\" in metrics output:\n%s", body)
	}
}

func TestBundle_activeConnections(t *testing.T) {
	reg := newTestRegistry(t)

	var wg sync.WaitGroup
	release := make(chan struct{})

	router := chi.NewRouter()
	router.Use(Bundle("svc", nil, reg, BundleConfig{Tracing: false, Logging: false, Metrics: true}))
	router.Get("/slow", func(w http.ResponseWriter, r *http.Request) {
		wg.Done()
		<-release
		w.WriteHeader(http.StatusOK)
	})

	wg.Add(1)
	go func() {
		req := httptest.NewRequest(http.MethodGet, "/slow", nil)
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)
	}()

	wg.Wait() // handler is inside the request body — gauge must be 1

	// Scrape via the /metrics HTTP handler (accessible from outside the package).
	body := scrapeMetrics(t, reg)

	close(release) // unblock the handler before asserting so we don't leak goroutines

	if !strings.Contains(body, "http_active_requests 1") {
		t.Errorf("expected http_active_requests=1 while request in flight:\n%s", body)
	}
}

func TestBundle_requestIDPreserved(t *testing.T) {
	// A valid UUID in X-Request-ID must be forwarded unchanged.
	const validUUID = "550e8400-e29b-41d4-a716-446655440000"

	router := chi.NewRouter()
	router.Use(Bundle("svc", nil, nil, BundleConfig{Tracing: false}))
	router.Get("/", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("X-Request-ID", validUUID)
	router.ServeHTTP(rec, req)

	got := rec.Header().Get("X-Request-ID")
	if got != validUUID {
		t.Errorf("expected X-Request-ID=%q, got %q", validUUID, got)
	}
}

func TestBundle_requestIDReplacedWhenInvalid(t *testing.T) {
	// An invalid X-Request-ID must be replaced with a fresh UUID.
	router := chi.NewRouter()
	router.Use(Bundle("svc", nil, nil, BundleConfig{Tracing: false}))
	router.Get("/", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("X-Request-ID", "not-a-uuid")
	router.ServeHTTP(rec, req)

	got := rec.Header().Get("X-Request-ID")
	if got == "not-a-uuid" {
		t.Error("invalid UUID must be replaced, but original was forwarded")
	}
	if got == "" {
		t.Error("X-Request-ID must be set even when input is invalid")
	}
}
