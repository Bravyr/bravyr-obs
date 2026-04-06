package trace

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/riandyrn/otelchi"
	"go.opentelemetry.io/otel"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
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

// TestHTTPMiddleware_createsSpan verifies that HTTPMiddleware creates at least
// one completed span for a matched route.
func TestHTTPMiddleware_createsSpan(t *testing.T) {
	sr := withTestProvider(t)

	r := chi.NewRouter()
	r.Use(HTTPMiddleware("test-server"))
	r.Get("/ping", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/ping", nil)
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	spans := sr.Ended()
	if len(spans) == 0 {
		t.Fatal("expected at least one span to be recorded")
	}
}

// TestUserAttributesMiddleware_addsAttributes verifies that user_id and
// workspace_id attributes are set on the active span when the funcs return
// non-empty values.
func TestUserAttributesMiddleware_addsAttributes(t *testing.T) {
	sr := withTestProvider(t)

	r := chi.NewRouter()
	// otelchi must come first in the chain to create the span before
	// UserAttributesMiddleware reads it.
	r.Use(otelchi.Middleware("test-server"))
	r.Use(UserAttributesMiddleware(
		func(_ *http.Request) string { return "user-42" },
		func(_ *http.Request) string { return "ws-99" },
	))
	r.Get("/resource", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/resource", nil)
	r.ServeHTTP(rec, req)

	spans := sr.Ended()
	if len(spans) == 0 {
		t.Fatal("expected at least one span")
	}

	attrMap := make(map[string]string)
	for _, a := range spans[0].Attributes() {
		attrMap[string(a.Key)] = a.Value.AsString()
	}

	if got := attrMap["user_id"]; got != "user-42" {
		t.Fatalf("expected user_id=user-42, got %q", got)
	}
	if got := attrMap["workspace_id"]; got != "ws-99" {
		t.Fatalf("expected workspace_id=ws-99, got %q", got)
	}
}

// TestUserAttributesMiddleware_skipsEmpty verifies that returning an empty
// string from userIDFunc or workspaceIDFunc does not add the attribute.
func TestUserAttributesMiddleware_skipsEmpty(t *testing.T) {
	sr := withTestProvider(t)

	r := chi.NewRouter()
	r.Use(otelchi.Middleware("test-server"))
	r.Use(UserAttributesMiddleware(
		func(_ *http.Request) string { return "" },
		func(_ *http.Request) string { return "" },
	))
	r.Get("/resource", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/resource", nil)
	r.ServeHTTP(rec, req)

	spans := sr.Ended()
	if len(spans) == 0 {
		t.Fatal("expected at least one span")
	}

	for _, a := range spans[0].Attributes() {
		key := string(a.Key)
		if key == "user_id" || key == "workspace_id" {
			t.Fatalf("expected no user_id/workspace_id attribute, found %q", key)
		}
	}
}

// TestHTTPMiddleware_capsTracestate verifies that a tracestate header
// exceeding maxTraceStateLen is removed before the inner handler sees it.
func TestHTTPMiddleware_capsTracestate(t *testing.T) {
	var capturedTracestate string
	inner := http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		capturedTracestate = r.Header.Get("tracestate")
	})

	mw := HTTPMiddleware("test-server")
	handler := mw(inner)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("tracestate", strings.Repeat("x", maxTraceStateLen+1))

	handler.ServeHTTP(rec, req)

	if capturedTracestate != "" {
		t.Fatalf("expected tracestate to be removed, got %q (len=%d)", capturedTracestate, len(capturedTracestate))
	}
}

// TestHTTPMiddleware_capsTracestate_integration verifies the ordering invariant:
// the tracestate cap fires BEFORE the otelchi propagator reads the header.
func TestHTTPMiddleware_capsTracestate_integration(t *testing.T) {
	sr := withTestProvider(t)

	r := chi.NewRouter()
	r.Use(HTTPMiddleware("test-server"))
	r.Get("/", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("tracestate", strings.Repeat("x", maxTraceStateLen+1))
	r.ServeHTTP(rec, req)

	spans := sr.Ended()
	if len(spans) == 0 {
		t.Fatal("expected a span to be recorded")
	}
	ts := spans[0].Parent().TraceState().String()
	if ts != "" {
		t.Fatalf("expected empty tracestate on span, got %q", ts)
	}
}

func TestUserAttributesMiddleware_nilUserIDFuncPanics(t *testing.T) {
	defer func() {
		r := recover()
		if r == nil {
			t.Fatal("expected panic for nil userIDFunc")
		}
		msg, ok := r.(string)
		if !ok || !strings.Contains(msg, "userIDFunc") {
			t.Fatalf("expected panic about userIDFunc, got: %v", r)
		}
	}()
	UserAttributesMiddleware(nil, func(*http.Request) string { return "" })
}

func TestUserAttributesMiddleware_nilWorkspaceIDFuncPanics(t *testing.T) {
	defer func() {
		r := recover()
		if r == nil {
			t.Fatal("expected panic for nil workspaceIDFunc")
		}
		msg, ok := r.(string)
		if !ok || !strings.Contains(msg, "workspaceIDFunc") {
			t.Fatalf("expected panic about workspaceIDFunc, got: %v", r)
		}
	}()
	UserAttributesMiddleware(func(*http.Request) string { return "" }, nil)
}
