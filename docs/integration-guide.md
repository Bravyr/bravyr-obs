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

## Database Tracing (pgxtrace)

Add automatic OTel spans to every pgx database query:

```go
import "github.com/bravyr/bravyr-obs/pgxtrace"

connConfig, _ := pgx.ParseConfig(databaseURL)
connConfig.Tracer = pgxtrace.NewTracer()
pool, _ := pgxpool.NewWithConfig(ctx, poolConfig)
```

Each query creates a child span with:
- `db.statement` — parameterized SQL (placeholders only, no values)
- `db.system` — `postgresql`
- `db.operation` — `SELECT`, `INSERT`, etc. (used as span name)

### Include query parameters (dev only)

```go
// Only in development — query params can contain PII
connConfig.Tracer = pgxtrace.NewTracer(pgxtrace.WithIncludeQueryParameters())
```

## Redis Tracing (redistrace)

Add automatic OTel spans to every Redis command:

```go
import "github.com/bravyr/bravyr-obs/redistrace"

rdb := redis.NewClient(&redis.Options{Addr: "localhost:6379"})
if err := redistrace.Instrument(rdb); err != nil {
    log.Fatal(err)
}
// All Redis commands now produce OTel spans.
```

Command text is redacted by default. Use `redistrace.WithDBStatement()` in dev only — command arguments can contain PII, auth credentials, or session tokens.

```go
// Development only:
redistrace.Instrument(rdb, redistrace.WithDBStatement())

// Enable OTel metrics collection in addition to tracing:
redistrace.Instrument(rdb, redistrace.WithMetrics())
```

## Temporal Tracing (temporaltrace)

Add OTel spans to every Temporal workflow, activity, signal, and query operation:

```go
import "github.com/bravyr/bravyr-obs/temporaltrace"

i, err := temporaltrace.NewInterceptor()
if err != nil {
    log.Fatal(err)
}
c, err := client.Dial(client.Options{
    HostPort:     "temporal:7233",
    Interceptors: []interceptor.ClientInterceptor{i},
})
```

To reduce span volume for high-frequency operations:

```go
i, err := temporaltrace.NewInterceptor(
    temporaltrace.WithoutSignalTracing(), // skip spans for signal handlers
    temporaltrace.WithoutQueryTracing(),  // skip spans for query handlers
)
```

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

## Frontend Observability (Grafana Faro)

The stack ingests browser telemetry (RUM) from any Grafana Faro Web SDK client through Alloy's `faro.receiver`. Traces reuse the backend's OTel Collector so a single Tempo trace spans the browser span and its downstream Go handler span.

### Backend requirements

No code changes are required in Go services — the existing middleware already accepts the W3C `traceparent` header propagated by Faro's fetch/XHR instrumentation. Verify the client app sends requests to the same API host (or CORS exposes `traceparent`).

### Ingest endpoint

Public route, terminated by Coolify/Traefik:

```
POST https://obs.bravyr.com/collect
Authorization: Basic base64("faro:${FARO_APP_KEY}")
Origin: https://<configured-origin>
```

Traefik enforces basic-auth, the origin allowlist, a 20 r/s rate limit (100 r/s burst), a 1 MB body cap, and HSTS. Alloy additionally enforces the CORS allowlist at the application layer.

### Stack environment variables

See `stack/.env.example`:

| Variable | Purpose |
|----------|---------|
| `FARO_PUBLIC_HOST` | FQDN Traefik routes `/collect` for (default `obs.bravyr.com`). |
| `FARO_ALLOWED_ORIGINS` | Comma-separated exact origins allowed to POST telemetry. |
| `FARO_APP_KEY` | Shared write-only token shipped to browsers as the basic-auth password. Per-env, rotatable. |
| `FARO_BASIC_AUTH_HTPASSWD` | Bcrypt htpasswd line for the Traefik `basicauth` middleware. Generate with `htpasswd -nbB faro "$FARO_APP_KEY"` and double every `$` to `$$` before pasting. |

### Frontend SDK integration

In the React (or any JS) client, pin the SDK versions and initialize before the app mounts:

```ts
import { initializeFaro, getWebInstrumentations } from "@grafana/faro-web-sdk";
import { TracingInstrumentation } from "@grafana/faro-web-tracing";

initializeFaro({
  url: import.meta.env.VITE_FARO_URL,
  apiKey: import.meta.env.VITE_FARO_APP_KEY,
  app: {
    name: import.meta.env.VITE_FARO_APP_NAME,
    version: import.meta.env.VITE_FARO_APP_VERSION,
    environment: import.meta.env.VITE_FARO_ENVIRONMENT,
  },
  sessionTracking: {
    enabled: true,
    samplingRate: Number(import.meta.env.VITE_FARO_SAMPLE_RATE ?? 1.0),
  },
  instrumentations: [
    ...getWebInstrumentations({ captureConsole: true }),
    new TracingInstrumentation(),
  ],
  beforeSend: (item) => {
    // Strip query strings, redact auth headers, drop request/response bodies,
    // truncate stacks at 4096 chars, swap user email for opaque user ID.
    return scrubFaroItem(item);
  },
});
```

The SDK sends `Authorization: Basic base64("faro:$VITE_FARO_APP_KEY")` via its fetch transport.

### Client env var contract

| Variable | Example |
|----------|---------|
| `VITE_FARO_URL` | `https://obs.bravyr.com/collect` |
| `VITE_FARO_APP_NAME` | `socialup-web` |
| `VITE_FARO_APP_VERSION` | `0.12.3+abc1234` (git SHA suffix) |
| `VITE_FARO_ENVIRONMENT` | `development` / `staging` / `production` |
| `VITE_FARO_SAMPLE_RATE` | `1.0` non-prod, `0.1` prod |
| `VITE_FARO_APP_KEY` | basic-auth password — per-env |

### Sampling defaults

| Environment | Errors | Page loads | Web-vitals |
|-------------|--------|-----------|------------|
| Production | 100% | 10% | 100% |
| Staging | 100% | 100% | 100% |
| Development | 100% | 100% | 100% |

### Route labelling

The Grafana dashboard aggregates Core Web Vitals and exceptions by `route`. For `route` to be useful, the SDK must emit a **normalized** path (e.g. `/user/:id`, not `/user/12345`). Call `faro.api.setView({ name: "/user/:id" })` on every navigation — typically inside your router's location-change hook. The Alloy pipeline runs a regex backstop that replaces numeric and UUID path segments, but SDK-side normalization is the source of truth.

### PII and GDPR

- Always implement `beforeSend`. Strip query strings, drop request/response bodies, redact `Authorization`/`Cookie`/`X-Api-Key` headers, truncate stacks.
- Never serve `.map` files publicly. Upload source maps to a private symbolicator keyed by release ID.
- The Alloy Loki pipeline scrubs client IP, remote-address, and `X-Forwarded-For` shaped fields from the stored log line. User-Agent is retained as structured metadata.
- Default retention for Tempo and Loki is 14 days.

### Dashboard

Grafana auto-provisions **Frontend RUM (Faro)** — page loads, distinct sessions, JS exceptions, Core Web Vitals (LCP, INP, CLS) p75 by route, browser/OS/route breakdowns, and an exception table with one-click Tempo trace links.
