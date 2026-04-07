# Integration Guide

Add full observability to any Go service with one `Init()` call.

## Installation

```bash
go get github.com/bravyr/bravyr-obs
```

## Quick Start

```go
package main

import (
	"context"
	"net/http"

	obs "github.com/bravyr/bravyr-obs"
	"github.com/bravyr/bravyr-obs/health"
	"github.com/go-chi/chi/v5"
)

func main() {
	o, err := obs.Init(obs.Config{
		ServiceName:   "my-api",
		Environment:   "production",
		LogLevel:      "info",
		OTLPEndpoint:  "otel-collector:4317",
		MetricsPrefix: "my_api",
	})
	if err != nil {
		panic(err)
	}
	defer o.Shutdown(context.Background())

	r := chi.NewRouter()
	r.Use(o.Middleware()) // tracing + metrics + request logging

	checker := health.New()
	checker.AddCheck("postgres", health.PgxCheck(pool))
	r.Get("/health", checker.Handler())
	r.Handle("/metrics", o.MetricsHandler())

	o.Logger().Info().Msg("server starting")
	http.ListenAndServe(":8080", r)
}
```

This gives you:

- Structured JSON logging to stdout (collected by Promtail)
- Distributed tracing via OTLP to your collector
- Prometheus metrics on `/metrics`
- Health checks on `/health`
- Request logging with trace_id correlation
- Active connections gauge

## Configuration

All configuration is via environment variables or struct fields:

| Field | Env Var | Default | Description |
|---|---|---|---|
| `ServiceName` | `OBS_SERVICE_NAME` | *(required)* | Name for logs, traces, and metrics |
| `Environment` | `OBS_ENVIRONMENT` | `development` | Deployment environment |
| `LogLevel` | `OBS_LOG_LEVEL` | `info` | Log level: debug, info, warn, error, fatal |
| `OTLPEndpoint` | `OBS_OTLP_ENDPOINT` | *(empty = no tracing)* | OTel Collector gRPC endpoint |
| `SampleRate` | `OBS_SAMPLE_RATE` | `1.0` | Trace sampling rate (0.0-1.0) |
| `DevMode` | `OBS_DEV_MODE` | `false` | Pretty console logs + insecure gRPC |
| `MetricsPrefix` | `OBS_METRICS_PREFIX` | *(empty)* | Prefix for Prometheus metric names |

When `OTLPEndpoint` is empty, tracing is disabled (no-op). The library still works — you just don't get traces.

When `DevMode` is true:
- Logs are pretty-printed to stdout (human-readable)
- Tracing uses `AlwaysSample` regardless of `SampleRate`
- OTLP gRPC uses insecure (plaintext) transport

## Logging

The logger is a thin wrapper around [zerolog](https://github.com/rs/zerolog). All level methods return `*zerolog.Event` for chaining:

```go
logger := o.Logger()

logger.Info().
    Str("user_id", "abc-123").
    Str("action", "login").
    Msg("user logged in")

logger.Error().
    Err(err).
    Str("endpoint", "/api/users").
    Msg("request failed")
```

### Sub-loggers

Create scoped loggers with additional fields:

```go
reqLogger := logger.With().
    Str("request_id", requestID).
    Str("user_id", userID).
    Logger()

reqLogger.Info().Msg("processing request") // includes request_id and user_id
```

Note: sub-loggers are raw `zerolog.Logger` instances. They share the parent's output writer and are flushed when the parent shuts down.

### Trace correlation

The middleware automatically adds `trace_id` and `span_id` to every request log. These fields are extracted from the OTel span context, enabling log-to-trace correlation in Grafana (Loki + Tempo).

## Health Checks

### Basic usage

```go
checker := health.New()
checker.AddCheck("postgres", health.PgxCheck(pool))
checker.AddCheck("redis", health.RedisCheck(redisClient))

r.Get("/health", checker.Handler())
```

Response:
```json
{
  "status": "healthy",
  "checks": [
    {"name": "postgres", "healthy": true, "duration": "2ms"},
    {"name": "redis", "healthy": true, "duration": "1ms"}
  ]
}
```

### Per-check timeouts

```go
checker.AddCheck("slow-service", myCheck, health.WithCheckTimeout(2*time.Second))
```

### Custom checks

Any `func(ctx context.Context) error` works as a `CheckFunc`:

```go
checker.AddCheck("temporal", func(ctx context.Context) error {
    _, err := temporalClient.CheckHealth(ctx, &client.CheckHealthRequest{})
    return err
})
```

### Global timeout

```go
checker := health.New(health.WithTimeout(10 * time.Second))
```

### Interface-based checkers

`PgxCheck` accepts any type with `Ping(ctx context.Context) error` — not just `*pgxpool.Pool`. This means zero driver imports in the health package.

`RedisCheck` accepts any type with `Ping(ctx context.Context) RedisResult` where `RedisResult` has `Err() error`.

## Metrics

### /metrics endpoint

```go
r.Handle("/metrics", o.MetricsHandler())
```

The endpoint serves Prometheus text format. It should be protected by authentication or served on an internal-only port.

### Built-in HTTP metrics

The middleware automatically records:
- `{prefix}_http_request_duration_seconds` — histogram by method, path, status
- `{prefix}_http_requests_total` — counter by method, path, status
- `{prefix}_http_active_requests` — gauge

Path labels use Chi route patterns (`/users/{id}`, not `/users/123`) to prevent label cardinality explosion.

### Custom business metrics

```go
signups, _ := o.Metrics().NewCounter(
    "user_signups_total",
    "Total user signups",
    []string{"plan"},
)
signups.WithLabelValues("pro").Inc()

latency, _ := o.Metrics().NewHistogram(
    "ai_response_seconds",
    "AI response generation time",
    []string{"model"},
    nil, // nil = default buckets
)
latency.WithLabelValues("gpt-4").Observe(1.23)
```

## Middleware

`o.Middleware()` returns a composed Chi middleware chain:

1. **Tracing** (outermost) — creates OTel span with route pattern, method, status
2. **Metrics** — records duration, count, active connections
3. **Logging** — logs method, path, status, duration, request_id, trace_id, span_id

### Selective disable

```go
r.Use(o.MiddlewareWithConfig(middleware.BundleConfig{
    Tracing: true,
    Logging: true,
    Metrics: false, // disable metrics on this router group
}))
```

### User attributes on traces

Place after your auth middleware:

```go
r.Use(trace.UserAttributesMiddleware(
    func(r *http.Request) string {
        return auth.UserIDFromContext(r.Context())
    },
    func(r *http.Request) string {
        return auth.WorkspaceIDFromContext(r.Context())
    },
))
```

## Tracing

Tracing is automatic when `OTLPEndpoint` is set. Spans are exported via OTLP/gRPC to your collector.

- Sampling: `ParentBased(TraceIDRatioBased(SampleRate))` — respects parent decisions
- DevMode: `AlwaysSample` regardless of rate
- Empty endpoint: no-op (safe, no errors)

The global OTel TracerProvider is registered, so any OTel-compatible library (otelhttp, otelpgx, otelgrpc) will automatically use it.

## Monitoring Stack

Start the full LGTM stack:

```bash
cd stack
cp .env.example .env  # edit with your values
docker compose up -d
```

Services:
- **Loki** (log aggregation) — internal only
- **Promtail** (log collection) — tails container stdout
- **Tempo** (trace storage) — internal only
- **Prometheus** (metrics) — internal only
- **OTel Collector** (trace/metrics relay) — localhost:4317
- **Grafana** (dashboards) — localhost:3000

### Exporters

Database and cache exporters (postgres-exporter, redis-exporter, node-exporter) should live alongside their parent services in your **application's** Docker Compose, not in this stack. Add their scrape targets to `stack/prometheus/prometheus.yml`. See `stack/README.md` for examples.

### Docker integration

For Promtail to collect your service's logs, your service must run as a Docker container. Promtail tails all container stdout/stderr automatically via Docker service discovery.

Set these env vars on your Go service container:

```yaml
environment:
  OBS_SERVICE_NAME: my-api
  OBS_LOG_LEVEL: info
  OBS_OTLP_ENDPOINT: otel-collector:4317
  OBS_DEV_MODE: "false"
  OBS_METRICS_PREFIX: my_api
```

## Shutdown

Always defer `o.Shutdown()` to flush telemetry:

```go
o, err := obs.Init(cfg)
if err != nil {
    log.Fatal(err)
}
defer o.Shutdown(context.Background())
```

Shutdown order: tracer (flush spans) → metrics (no-op) → logger (no-op). The tracer flushes first so any span-related logs can still be emitted.

## Middleware Ordering

If you use the middleware components separately (not via `o.Middleware()`), the correct order is:

```go
r.Use(trace.HTTPMiddleware("my-api"))     // 1. creates span context
r.Use(metricsRegistry.HTTPMiddleware())    // 2. records metrics
r.Use(middleware.RequestLogging(logger))   // 3. logs with trace_id
r.Use(authMiddleware)                      // 4. sets user in context
r.Use(trace.UserAttributesMiddleware(...)) // 5. adds user to span
```

`o.Middleware()` handles steps 1-3 automatically.
