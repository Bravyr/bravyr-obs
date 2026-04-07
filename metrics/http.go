package metrics

import (
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
)

// statusWriter wraps an http.ResponseWriter to capture the HTTP status code
// written by the handler. It intercepts WriteHeader to record the code before
// delegating to the underlying writer. If the handler never calls WriteHeader,
// status defaults to 200 (the net/http default).
type statusWriter struct {
	http.ResponseWriter
	status      int
	wroteHeader bool
}

func (w *statusWriter) WriteHeader(code int) {
	if !w.wroteHeader {
		w.status = code
		w.wroteHeader = true
	}

	w.ResponseWriter.WriteHeader(code)
}

func (w *statusWriter) Write(b []byte) (int, error) {
	if !w.wroteHeader {
		w.status = http.StatusOK
		w.wroteHeader = true
	}
	return w.ResponseWriter.Write(b)
}

// Unwrap returns the underlying ResponseWriter, enabling http.ResponseController
// (Go 1.20+) to correctly delegate Flush, Hijack, and other optional interfaces.
func (w *statusWriter) Unwrap() http.ResponseWriter {
	return w.ResponseWriter
}

// HTTPMiddleware returns a standalone Chi-compatible middleware that records
// HTTP request metrics. For most use cases, prefer middleware.Bundle which
// composes tracing, metrics, and logging with a single response writer wrapper.
// Do NOT use HTTPMiddleware and Bundle in the same middleware chain — they
// will double-count metrics and double-wrap the response writer.
//
// Path labels use chi.RouteContext().RoutePattern() rather than r.URL.Path to
// avoid unbounded label cardinality from path parameters.
//
// Method labels are normalized to standard HTTP verbs to prevent cardinality
// explosion from non-standard methods.
func (r *Registry) HTTPMiddleware() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			r.httpActiveRequests.Inc()
			// Dec runs after the handler returns even if it panics, ensuring
			// the gauge does not drift upward on panicking handlers.
			defer r.httpActiveRequests.Dec()

			sw := &statusWriter{ResponseWriter: w, status: http.StatusOK}
			start := time.Now()

			next.ServeHTTP(sw, req)

			duration := time.Since(start).Seconds()
			path := routePattern(req)
			statusCode := strconv.Itoa(sw.status)
			method := normalizeMethod(req.Method)

			r.httpRequestDuration.WithLabelValues(method, path, statusCode).Observe(duration)
			r.httpRequestsTotal.WithLabelValues(method, path, statusCode).Inc()
		})
	}
}

// normalizeMethod maps HTTP methods to a bounded set of known verbs.
// Unknown methods are mapped to "OTHER" to prevent unbounded label cardinality.
func normalizeMethod(m string) string {
	switch m {
	case http.MethodGet, http.MethodPost, http.MethodPut, http.MethodPatch,
		http.MethodDelete, http.MethodHead, http.MethodOptions,
		http.MethodConnect, http.MethodTrace:
		return m
	default:
		return "OTHER"
	}
}

// routePattern returns the matched Chi route pattern for the request, falling
// back to "/unknown" when no Chi route context is present. Using the pattern
// rather than the raw URL prevents label cardinality explosion from
// parameterized paths.
func routePattern(r *http.Request) string {
	if rctx := chi.RouteContext(r.Context()); rctx != nil {
		if pattern := rctx.RoutePattern(); pattern != "" {
			return pattern
		}
	}

	return "/unknown"
}
