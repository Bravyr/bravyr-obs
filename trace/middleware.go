package trace

import (
	"net/http"

	"github.com/riandyrn/otelchi"
	"go.opentelemetry.io/otel/attribute"
	oteltrace "go.opentelemetry.io/otel/trace"
)

// maxTraceStateLen is the maximum number of bytes accepted in the W3C
// tracestate header. The W3C spec allows up to 512 bytes; we enforce this
// limit to prevent memory pressure from maliciously large headers before the
// OTel propagator processes them.
const maxTraceStateLen = 512

// HTTPMiddleware returns a Chi-compatible middleware that creates an OTel span
// for every HTTP request. The span is named after the matched route pattern,
// so traces group by endpoint rather than by raw URL path (which would create
// unbounded cardinality for routes with path parameters).
//
// serverName is used as the otelchi server name attribute and should match
// Config.ServiceName so traces are correlated with the same service in the
// backend.
func HTTPMiddleware(serverName string) func(http.Handler) http.Handler {
	otelMW := otelchi.Middleware(serverName)
	return func(next http.Handler) http.Handler {
		otelHandler := otelMW(next)
		// Sanitize tracestate BEFORE the OTel propagator reads it.
		// A malformed or excessively large tracestate should not propagate
		// into the span context — dropping it is safer than truncating.
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if ts := r.Header.Get("tracestate"); len(ts) > maxTraceStateLen {
				r.Header.Del("tracestate")
			}
			otelHandler.ServeHTTP(w, r)
		})
	}
}

// UserAttributesMiddleware enriches the active span with user-scoped
// attributes. It must be placed after the auth middleware in the chain so
// that the user identity is already available in the request context.
//
// userIDFunc and workspaceIDFunc are called for every request; return an
// empty string to omit the attribute for that request.
func UserAttributesMiddleware(
	userIDFunc func(r *http.Request) string,
	workspaceIDFunc func(r *http.Request) string,
) func(http.Handler) http.Handler {
	if userIDFunc == nil {
		panic("trace: UserAttributesMiddleware: userIDFunc must not be nil")
	}
	if workspaceIDFunc == nil {
		panic("trace: UserAttributesMiddleware: workspaceIDFunc must not be nil")
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			span := oteltrace.SpanFromContext(r.Context())
			if span.IsRecording() {
				if uid := userIDFunc(r); uid != "" {
					span.SetAttributes(attribute.String("user_id", uid))
				}
				if wid := workspaceIDFunc(r); wid != "" {
					span.SetAttributes(attribute.String("workspace_id", wid))
				}
			}
			next.ServeHTTP(w, r)
		})
	}
}
