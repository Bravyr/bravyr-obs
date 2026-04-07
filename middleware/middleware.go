// Package middleware provides a Chi middleware bundle for request logging,
// trace propagation, and request metrics.
package middleware

import (
	"net/http"
	"regexp"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	obslog "github.com/bravyr/bravyr-obs/log"
	"github.com/bravyr/bravyr-obs/metrics"
	obstrace "github.com/bravyr/bravyr-obs/trace"
)

// BundleConfig controls which middleware components are enabled.
type BundleConfig struct {
	Tracing bool
	Logging bool
	Metrics bool
}

// DefaultBundleConfig returns a config with all components enabled.
func DefaultBundleConfig() BundleConfig {
	return BundleConfig{Tracing: true, Logging: true, Metrics: true}
}

// Bundle returns a single Chi-compatible middleware that composes tracing,
// metrics, and request logging. It uses a single response writer wrapper
// shared by both metrics and logging to avoid double-wrapping.
//
// Chain order: trace (outermost, creates span) → inner handler with shared
// statusWriter → metrics recording + request logging (after handler returns).
func Bundle(
	serviceName string,
	logger *obslog.Logger,
	metricsReg *metrics.Registry,
	cfg BundleConfig,
) func(http.Handler) http.Handler {
	metricsEnabled := cfg.Metrics && metricsReg != nil
	loggingEnabled := cfg.Logging && logger != nil
	tracingEnabled := cfg.Tracing && serviceName != ""

	return func(next http.Handler) http.Handler {
		// inner is the core handler: one statusWriter shared by metrics and logging.
		inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if metricsEnabled {
				metricsReg.IncrementActiveRequests()
				// Dec runs after the handler returns even on panic, keeping the
				// gauge consistent.
				defer metricsReg.DecrementActiveRequests()
			}

			sw := &statusWriter{ResponseWriter: w, status: http.StatusOK}
			start := time.Now()

			// Resolve request ID before delegating so the response header is
			// set regardless of what the inner handler does.
			requestID := resolveRequestID(r)
			sw.Header().Set("X-Request-ID", requestID)

			next.ServeHTTP(sw, r)

			duration := time.Since(start)
			path := routePattern(r)
			method := normalizeMethod(r.Method)

			if metricsEnabled {
				metricsReg.RecordHTTPRequest(method, path, sw.status, duration)
			}

			if loggingEnabled {
				logRequest(logger, r, sw.status, duration, path, requestID, method)
			}
		})

		// Wrap with trace middleware if enabled — tracing is outermost so the
		// span covers the full request lifecycle including metrics recording.
		if tracingEnabled {
			return obstrace.HTTPMiddleware(serviceName)(inner)
		}
		return inner
	}
}

// statusWriter wraps an http.ResponseWriter to capture the HTTP status code
// written by the handler. If the handler never calls WriteHeader, status
// defaults to 200 (the net/http default).
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

// uuidRE matches a canonical UUID string (any version). Only structurally
// valid UUIDs are forwarded as X-Request-ID to prevent log injection.
var uuidRE = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)

// resolveRequestID returns the incoming X-Request-ID if it is a valid UUID,
// otherwise generates a fresh one. This prevents log injection while still
// propagating caller-supplied correlation IDs.
func resolveRequestID(r *http.Request) string {
	if id := r.Header.Get("X-Request-ID"); uuidRE.MatchString(id) {
		return id
	}
	return uuid.NewString()
}

// routePattern returns the matched Chi route pattern for the request, falling
// back to "/unknown" when no Chi route context is present.
func routePattern(r *http.Request) string {
	if rctx := chi.RouteContext(r.Context()); rctx != nil {
		if pattern := rctx.RoutePattern(); pattern != "" {
			return pattern
		}
	}
	return "/unknown"
}

// normalizeMethod maps HTTP methods to a bounded set of known verbs.
// Unknown methods are mapped to "OTHER" to prevent label cardinality explosion.
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
