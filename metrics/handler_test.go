package metrics

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestHandler_servesMetrics(t *testing.T) {
	reg, err := Init(Config{})
	if err != nil {
		t.Fatalf("Init() error: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	rec := httptest.NewRecorder()
	reg.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}

	contentType := rec.Header().Get("Content-Type")
	if !strings.Contains(contentType, "text/plain") {
		t.Errorf("expected text/plain content type, got %q", contentType)
	}
}

func TestHandler_returnsNonNil(t *testing.T) {
	reg, err := Init(Config{})
	if err != nil {
		t.Fatalf("Init() error: %v", err)
	}

	h := reg.Handler()
	if h == nil {
		t.Fatal("Handler() returned nil")
	}
}
