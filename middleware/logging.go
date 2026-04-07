package middleware

import (
	"net/http"
	"time"

	obslog "github.com/bravyr/bravyr-obs/log"
	oteltrace "go.opentelemetry.io/otel/trace"
)

// logRequest logs a completed HTTP request with structured fields.
// It extracts trace_id and span_id from the OTel span context if available,
// enabling log-to-trace correlation in observability backends.
func logRequest(logger *obslog.Logger, r *http.Request, status int, duration time.Duration, path, requestID, method string) {
	evt := logger.Info()
	if status >= 500 {
		evt = logger.Error()
	}

	evt = evt.
		Str("method", method).
		Str("path", path).
		Int("status", status).
		Dur("duration", duration).
		Str("request_id", requestID)

	// Attach trace correlation fields when an active span is present. This
	// links the log event to the distributed trace in backends like Grafana.
	spanCtx := oteltrace.SpanContextFromContext(r.Context())
	if spanCtx.HasTraceID() {
		evt = evt.Str("trace_id", spanCtx.TraceID().String())
	}
	if spanCtx.HasSpanID() {
		evt = evt.Str("span_id", spanCtx.SpanID().String())
	}

	if status >= 500 {
		evt.Msg("request failed")
	} else {
		evt.Msg("request completed")
	}
}
