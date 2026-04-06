package health

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestHandler_allHealthy(t *testing.T) {
	checks := map[string]CheckFunc{
		"db": func(ctx context.Context) error { return nil },
	}
	handler := Handler(checks)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	handler(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	var resp Response
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if resp.Status != "healthy" {
		t.Fatalf("expected status healthy, got %q", resp.Status)
	}
	if len(resp.Checks) != 1 {
		t.Fatalf("expected 1 check, got %d", len(resp.Checks))
	}
	if !resp.Checks[0].Healthy {
		t.Fatal("expected check to be healthy")
	}
}

func TestHandler_unhealthy(t *testing.T) {
	checks := map[string]CheckFunc{
		"db": func(ctx context.Context) error { return errors.New("connection refused") },
	}
	handler := Handler(checks)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	handler(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d", rec.Code)
	}

	var resp Response
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if resp.Status != "unhealthy" {
		t.Fatalf("expected status unhealthy, got %q", resp.Status)
	}
	if resp.Checks[0].Error != "check failed" {
		t.Fatalf("expected sanitized error message, got %q", resp.Checks[0].Error)
	}
}

func TestHandler_mixedChecks(t *testing.T) {
	checks := map[string]CheckFunc{
		"db":    func(ctx context.Context) error { return nil },
		"redis": func(ctx context.Context) error { return errors.New("connection refused") },
	}
	handler := Handler(checks)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	handler(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d", rec.Code)
	}

	var resp Response
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if resp.Status != "unhealthy" {
		t.Fatalf("expected status unhealthy, got %q", resp.Status)
	}
	if len(resp.Checks) != 2 {
		t.Fatalf("expected 2 checks, got %d", len(resp.Checks))
	}

	found := false
	for _, c := range resp.Checks {
		if c.Name == "redis" && !c.Healthy && c.Error == "check failed" {
			found = true
		}
	}
	if !found {
		t.Fatal("expected redis check to be unhealthy with sanitized error")
	}
}

func TestHandler_contentType(t *testing.T) {
	handler := Handler(nil)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	handler(rec, req)

	ct := rec.Header().Get("Content-Type")
	if ct != "application/json" {
		t.Fatalf("expected Content-Type application/json, got %q", ct)
	}
}

func TestHandler_methodNotAllowed(t *testing.T) {
	handler := Handler(nil)
	for _, method := range []string{http.MethodPost, http.MethodPut, http.MethodDelete} {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(method, "/health", nil)
		handler(rec, req)

		if rec.Code != http.StatusMethodNotAllowed {
			t.Fatalf("%s: expected 405, got %d", method, rec.Code)
		}
	}
}

func TestHandler_headMethod(t *testing.T) {
	handler := Handler(nil)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodHead, "/health", nil)
	handler(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 for HEAD, got %d", rec.Code)
	}
}

func TestHandler_noChecks(t *testing.T) {
	handler := Handler(nil)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	handler(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	var resp Response
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if resp.Status != "healthy" {
		t.Fatalf("expected healthy when no checks, got %q", resp.Status)
	}
}
