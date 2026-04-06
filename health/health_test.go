package health

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
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

func TestHandler_allowHeader(t *testing.T) {
	handler := Handler(nil)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/health", nil)
	handler(rec, req)

	allow := rec.Header().Get("Allow")
	if allow != "GET, HEAD" {
		t.Fatalf("expected Allow: GET, HEAD, got %q", allow)
	}
}

// --- Checker tests ---

func TestChecker_basic(t *testing.T) {
	c := New()
	c.AddCheck("db", func(ctx context.Context) error { return nil })

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	c.Handler()(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	var resp Response
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if resp.Status != "healthy" {
		t.Fatalf("expected healthy, got %q", resp.Status)
	}
	if len(resp.Checks) != 1 {
		t.Fatalf("expected 1 check, got %d", len(resp.Checks))
	}
	if !resp.Checks[0].Healthy {
		t.Fatal("expected check to be healthy")
	}
}

func TestChecker_unhealthy(t *testing.T) {
	c := New()
	c.AddCheck("db", func(ctx context.Context) error { return errors.New("connection refused") })

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	c.Handler()(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d", rec.Code)
	}

	var resp Response
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if resp.Status != "unhealthy" {
		t.Fatalf("expected unhealthy, got %q", resp.Status)
	}
	if resp.Checks[0].Error != "check failed" {
		t.Fatalf("expected sanitized error message, got %q", resp.Checks[0].Error)
	}
}

func TestChecker_perCheckTimeout(t *testing.T) {
	c := New()
	c.AddCheck("slow", func(ctx context.Context) error {
		select {
		case <-time.After(200 * time.Millisecond):
			return nil
		case <-ctx.Done():
			return ctx.Err()
		}
	}, WithCheckTimeout(50*time.Millisecond))

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	c.Handler()(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503 from timed-out check, got %d", rec.Code)
	}

	var resp Response
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if resp.Status != "unhealthy" {
		t.Fatalf("expected unhealthy, got %q", resp.Status)
	}
}

func TestChecker_globalTimeout(t *testing.T) {
	c := New(WithTimeout(100 * time.Millisecond))
	c.AddCheck("slow", func(ctx context.Context) error {
		select {
		case <-time.After(500 * time.Millisecond):
			return nil
		case <-ctx.Done():
			return ctx.Err()
		}
	})

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	c.Handler()(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503 from global timeout, got %d", rec.Code)
	}

	var resp Response
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if resp.Status != "unhealthy" {
		t.Fatalf("expected unhealthy, got %q", resp.Status)
	}
}

func TestChecker_mixedWithTimeout(t *testing.T) {
	c := New(WithTimeout(5 * time.Second))
	c.AddCheck("fast", func(ctx context.Context) error { return nil })
	c.AddCheck("slow", func(ctx context.Context) error {
		select {
		case <-time.After(500 * time.Millisecond):
			return nil
		case <-ctx.Done():
			return ctx.Err()
		}
	}, WithCheckTimeout(50*time.Millisecond))

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	c.Handler()(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d", rec.Code)
	}

	var resp Response
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if resp.Status != "unhealthy" {
		t.Fatalf("expected unhealthy, got %q", resp.Status)
	}
	if len(resp.Checks) != 2 {
		t.Fatalf("expected 2 results, got %d", len(resp.Checks))
	}

	healthy := 0
	unhealthy := 0
	for _, ch := range resp.Checks {
		if ch.Healthy {
			healthy++
		} else {
			unhealthy++
		}
	}
	if healthy != 1 || unhealthy != 1 {
		t.Fatalf("expected 1 healthy and 1 unhealthy, got %d healthy and %d unhealthy", healthy, unhealthy)
	}
}

func TestChecker_methodNotAllowed(t *testing.T) {
	c := New()
	handler := c.Handler()
	for _, method := range []string{http.MethodPost, http.MethodPut, http.MethodDelete} {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(method, "/health", nil)
		handler(rec, req)

		if rec.Code != http.StatusMethodNotAllowed {
			t.Fatalf("%s: expected 405, got %d", method, rec.Code)
		}
		allow := rec.Header().Get("Allow")
		if allow != "GET, HEAD" {
			t.Fatalf("%s: expected Allow: GET, HEAD, got %q", method, allow)
		}
	}
}

func TestChecker_headMethod(t *testing.T) {
	c := New()
	c.AddCheck("db", func(ctx context.Context) error { return nil })
	handler := c.Handler()

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodHead, "/health", nil)
	handler(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 for HEAD, got %d", rec.Code)
	}
}

func TestChecker_noChecks(t *testing.T) {
	c := New()
	handler := c.Handler()

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
