package metrics

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/go-chi/chi/v5"
)

// scrapeMetrics sends a GET request to the /metrics endpoint of the given
// registry and returns the Prometheus text body.
func scrapeMetrics(t *testing.T, reg *Registry) string {
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

func TestHTTPMiddleware_recordsMetrics(t *testing.T) {
	reg, err := Init(Config{Prefix: "test", })
	if err != nil {
		t.Fatalf("Init() error: %v", err)
	}

	router := chi.NewRouter()
	router.Use(reg.HTTPMiddleware())
	router.Get("/ping", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodGet, "/ping", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	body := scrapeMetrics(t, reg)

	if !strings.Contains(body, "test_http_requests_total") {
		t.Errorf("expected test_http_requests_total in metrics output, got:\n%s", body)
	}
	if !strings.Contains(body, "test_http_request_duration_seconds") {
		t.Errorf("expected test_http_request_duration_seconds in metrics output, got:\n%s", body)
	}
}

func TestHTTPMiddleware_routePatternLabels(t *testing.T) {
	// This is the cardinality guard test. Requests to /items/1, /items/2, etc.
	// must all record under the pattern label "/items/{id}", not raw paths.
	reg, err := Init(Config{Prefix: "card", })
	if err != nil {
		t.Fatalf("Init() error: %v", err)
	}

	router := chi.NewRouter()
	router.Use(reg.HTTPMiddleware())
	router.Get("/items/{id}", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	for _, id := range []string{"1", "2", "3", "abc", "xyz"} {
		req := httptest.NewRequest(http.MethodGet, "/items/"+id, nil)
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)
	}

	body := scrapeMetrics(t, reg)

	// There must be exactly one path label value: /items/{id}
	const pattern = `path="/items/{id}"`
	count := strings.Count(body, pattern)
	if count == 0 {
		t.Errorf("expected label %q in metrics output, got:\n%s", pattern, body)
	}

	// Verify that raw paths did NOT become labels.
	for _, rawPath := range []string{`path="/items/1"`, `path="/items/abc"`} {
		if strings.Contains(body, rawPath) {
			t.Errorf("raw path %q must not appear as a label; cardinality guard failed.\nOutput:\n%s", rawPath, body)
		}
	}
}

func TestHTTPMiddleware_activeConnections(t *testing.T) {
	reg, err := Init(Config{Prefix: "active", })
	if err != nil {
		t.Fatalf("Init() error: %v", err)
	}

	// Use a WaitGroup and channel to hold the handler mid-flight so we can
	// assert the gauge is >0 while the request is in progress.
	var wg sync.WaitGroup
	release := make(chan struct{})

	router := chi.NewRouter()
	router.Use(reg.HTTPMiddleware())
	router.Get("/slow", func(w http.ResponseWriter, r *http.Request) {
		wg.Done()       // signal that the handler is running
		<-release       // block until the test releases it
		w.WriteHeader(http.StatusOK)
	})

	// Launch request in background.
	wg.Add(1)
	go func() {
		req := httptest.NewRequest(http.MethodGet, "/slow", nil)
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)
	}()

	// Wait for the handler goroutine to be inside the handler body.
	wg.Wait()

	// Gather gauge value while request is active.
	mfs, err := reg.reg.Gather()
	if err != nil {
		t.Fatalf("Gather() error: %v", err)
	}

	activeVal := float64(0)
	for _, mf := range mfs {
		if mf.GetName() == "active_http_active_requests" {
			for _, m := range mf.GetMetric() {
				activeVal = m.GetGauge().GetValue()
			}
		}
	}

	close(release) // unblock the handler

	if activeVal != 1 {
		t.Errorf("expected active_http_active_requests=1 while request in flight, got %g", activeVal)
	}
}

func TestHTTPMiddleware_statusCode(t *testing.T) {
	cases := []struct {
		name       string
		statusCode int
		wantLabel  string
	}{
		{"200 OK", http.StatusOK, `status_code="200"`},
		{"201 Created", http.StatusCreated, `status_code="201"`},
		{"404 Not Found", http.StatusNotFound, `status_code="404"`},
		{"500 Internal", http.StatusInternalServerError, `status_code="500"`},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// Each sub-test gets its own isolated registry.
			reg, err := Init(Config{})
			if err != nil {
				t.Fatalf("Init() error: %v", err)
			}

			router := chi.NewRouter()
			router.Use(reg.HTTPMiddleware())
			router.Get("/check", func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tc.statusCode)
			})

			req := httptest.NewRequest(http.MethodGet, "/check", nil)
			rec := httptest.NewRecorder()
			router.ServeHTTP(rec, req)

			body := scrapeMetrics(t, reg)
			if !strings.Contains(body, tc.wantLabel) {
				t.Errorf("expected label %q in metrics output for status %d, got:\n%s",
					tc.wantLabel, tc.statusCode, body)
			}
		})
	}
}

func TestHTTPMiddleware_unknownRoute(t *testing.T) {
	// When there is no Chi route context (e.g. middleware mounted outside Chi),
	// the path label must fall back to /unknown.
	reg, err := Init(Config{})
	if err != nil {
		t.Fatalf("Init() error: %v", err)
	}

	// Wrap a plain handler directly — no Chi router involved.
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	handler := reg.HTTPMiddleware()(inner)

	req := httptest.NewRequest(http.MethodGet, "/anything", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	body := scrapeMetrics(t, reg)
	if !strings.Contains(body, `path="/unknown"`) {
		t.Errorf("expected path=\"/unknown\" in metrics output, got:\n%s", body)
	}
}
