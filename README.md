# bravyr-obs

Opinionated observability library for Go services. Structured logging,
distributed tracing, and Prometheus metrics in one `Init()` call.

## Quick Start

```go
package main

import (
	"context"
	"net/http"
	"time"

	obs "github.com/bravyr/bravyr-obs"
	"github.com/bravyr/bravyr-obs/health"
	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"
)

func main() {
	pool, _ := pgxpool.New(context.Background(), "postgres://localhost:5432/mydb")
	rdb := redis.NewClient(&redis.Options{Addr: "localhost:6379"})

	// Build a Checker with a global 5s timeout and per-check overrides.
	checker := health.New(health.WithTimeout(5 * time.Second))
	checker.AddCheck("postgres", health.PgxCheck(pool))
	checker.AddCheck("redis", health.RedisCheck(rdb),
		health.WithCheckTimeout(2*time.Second))

	o, err := obs.Init(obs.Config{
		ServiceName:  "socialup-api",
		Environment:  "production",
		LogLevel:     "info",
		OTLPEndpoint: "otel-collector:4317",
	})
	if err != nil {
		panic(err)
	}
	defer o.Shutdown(context.Background())

	router := chi.NewRouter()
	// Single middleware call composes tracing + metrics + request logging.
	router.Use(o.Middleware())
	router.Get("/api/health", checker.Handler())

	http.ListenAndServe(":8080", router)
}
```

## Logging

`obs.Init()` constructs a zerolog-based structured logger automatically. Access
it via `o.Logger()`:

```go
o, err := obs.Init(obs.Config{
    ServiceName: "my-api",
    Environment: "production",
    LogLevel:    "info",
})
if err != nil {
    panic(err)
}
defer o.Shutdown(context.Background())

log := o.Logger()
log.Info().Str("version", "1.0.0").Msg("service started")
log.Error().Err(err).Str("user_id", uid).Msg("database query failed")

// Derive a request-scoped sub-logger with fixed fields.
reqLog := log.With().Str("request_id", reqID).Logger()
reqLog.Info().Msg("handling request")
```

### Logging configuration

| Behaviour | Config |
|---|---|
| Human-readable console output | `DevMode: true` (development only) |
| Minimum log level | `LogLevel: "debug"` / `"info"` / `"warn"` / `"error"` / `"fatal"` |

In non-dev mode the logger writes structured JSON to stdout. Logs are collected
from container stdout by Promtail and forwarded to Loki for aggregation.

The `log` package can also be used standalone without the root facade:

```go
import obslog "github.com/bravyr/bravyr-obs/log"

logger, err := obslog.New(obslog.Config{
    ServiceName: "worker",
    Level:       "debug",
    DevMode:     true, // omit in production; JSON goes to stdout for Promtail
})
```

## Installation

```bash
go get github.com/bravyr/bravyr-obs
```

## Middleware Bundle

`obs.Middleware()` returns a single Chi-compatible middleware that composes
tracing, metrics, and request logging in one call. The middleware uses a
shared `statusWriter` wrapper so the response code is captured exactly once,
feeding both Prometheus labels and the structured log entry.

```go
router := chi.NewRouter()
router.Use(o.Middleware()) // tracing + metrics + request logging

// Selective enablement — disable tracing for a worker service:
import obsmw "github.com/bravyr/bravyr-obs/middleware"

router.Use(o.MiddlewareWithConfig(obsmw.BundleConfig{
    Tracing: false,
    Logging: true,
    Metrics: true,
}))
```

### Request logging

Every completed request emits a structured log event with the following fields:

| Field | Description |
|---|---|
| `method` | HTTP method (GET, POST, …) |
| `path` | Chi route pattern (`/items/{id}`, not raw URL) |
| `status` | HTTP status code written by the handler |
| `duration` | Elapsed time in nanoseconds (zerolog `Dur`) |
| `request_id` | Forwarded from `X-Request-ID` if a valid UUID; otherwise freshly generated |
| `trace_id` | OTel trace ID — present when tracing is active |
| `span_id` | OTel span ID — present when tracing is active |

Requests with status ≥ 500 are logged at `error` level; all others at `info`.

The bundle also sets the `X-Request-ID` response header so callers can correlate
their own logs with server-side entries.

### X-Request-ID validation

Only well-formed UUIDs (RFC 4122 format) are accepted from incoming requests.
Any other value — including empty strings or freeform text — is replaced with a
freshly generated UUID. This prevents log injection from arbitrary header content.

## Tracing

`obs.Init()` wires an OpenTelemetry tracer automatically. `obs.Middleware()` wraps
[otelchi](https://github.com/riandyrn/otelchi) to create a span per request, named
after the matched Chi route pattern.

```go
o, err := obs.Init(obs.Config{
    ServiceName:  "my-api",
    Environment:  "production",
    OTLPEndpoint: "otel-collector:4317",
    SampleRate:   0.1, // sample 10% of traces in production
})
if err != nil {
    panic(err)
}
defer o.Shutdown(context.Background())

router := chi.NewRouter()
router.Use(o.Middleware()) // tracing + metrics + request logging in one call

// Optionally enrich spans with user and workspace identity.
// Place after your auth middleware so the user is already resolved.
router.Use(trace.UserAttributesMiddleware(
    func(r *http.Request) string { return userIDFromContext(r.Context()) },
    func(r *http.Request) string { return workspaceIDFromContext(r.Context()) },
))
```

When `OTLPEndpoint` is empty, `Init()` installs a no-op tracer — no spans are
exported and no network connection is made. This makes it safe to omit the
endpoint in local development without code changes.

### Tracing configuration

| Behaviour | Config |
|---|---|
| Export spans to a collector | Set `OTLPEndpoint` (e.g. `"otel-collector:4317"`) |
| Always sample (local dev) | `DevMode: true` |
| Never sample | `SampleRate: 0.0` |
| Sample a fraction | `SampleRate: 0.1` (10%) |
| Insecure gRPC transport | `DevMode: true` (development only) |

## Metrics

`obs.Init()` creates an isolated Prometheus registry automatically. Access the
`/metrics` handler via `o.MetricsHandler()` and create custom business metrics
via `o.Metrics()`:

```go
o, err := obs.Init(obs.Config{
    ServiceName:   "my-api",
    Environment:   "production",
    MetricsPrefix: "myapi", // all metrics become "myapi_http_request_duration_seconds" etc.
})
if err != nil {
    panic(err)
}
defer o.Shutdown(context.Background())

router := chi.NewRouter()
// obs.Middleware() already records duration, count, and active connections.
// Mount the Prometheus text format at /metrics separately:
router.Get("/metrics", o.MetricsHandler().ServeHTTP)

// Create custom business metrics scoped to the same isolated registry.
ordersTotal, err := o.Metrics().NewCounter(
    "orders_total",
    "total orders placed",
    []string{"status"},
)
if err != nil {
    panic(err)
}
ordersTotal.WithLabelValues("success").Inc()

dbLatency, err := o.Metrics().NewHistogram(
    "db_query_duration_seconds",
    "database query latency",
    []string{"table"},
    prometheus.DefBuckets,
)
if err != nil {
    panic(err)
}
dbLatency.WithLabelValues("orders").Observe(0.012)
```

The built-in HTTP metrics are:

| Metric | Type | Labels |
|---|---|---|
| `{prefix}_http_request_duration_seconds` | Histogram | `method`, `path`, `status_code` |
| `{prefix}_http_requests_total` | Counter | `method`, `path`, `status_code` |
| `{prefix}_http_active_requests` | Gauge | *(none)* |

## Features

| Feature | Package | Status |
|---|---|---|
| Structured logging (zerolog, stdout JSON) | `log` | Available |
| Distributed tracing (OpenTelemetry OTLP) | `trace` | Available |
| Prometheus metrics | `metrics` | Available |
| Chi middleware bundle | `middleware` | Available (tracing + metrics + logging, trace-ID correlation) |
| Health check endpoint | `health` | Available (typed checkers: Postgres, Redis) |
| Environment-based configuration | `config` | Available |
| Local monitoring stack (Docker Compose) | `stack` | Planned |

## Configuration

All configuration fields can be set via struct literal or environment variables:

| Field | Env Var | Default | Description |
|---|---|---|---|
| `ServiceName` | `OBS_SERVICE_NAME` | *(required)* | Name of the service for logs/traces |
| `Environment` | `OBS_ENVIRONMENT` | `development` | Deployment environment |
| `LogLevel` | `OBS_LOG_LEVEL` | `info` | Log level (debug, info, warn, error, fatal) |
| `OTLPEndpoint` | `OBS_OTLP_ENDPOINT` | | OpenTelemetry Collector gRPC endpoint |
| `SampleRate` | `OBS_SAMPLE_RATE` | `1.0` | Fraction of traces to sample (0.0–1.0); overridden to 1.0 when `DevMode` is true |
| `DevMode` | `OBS_DEV_MODE` | `false` | Enable pretty console logging and always-sample tracing |
| `MetricsPrefix` | `OBS_METRICS_PREFIX` | | Prefix prepended to all metric names (e.g. `"myapi"` → `"myapi_http_requests_total"`); empty means no prefix |

## Architecture

```
github.com/bravyr/bravyr-obs
├── obs.go          Root facade: Init() returns *Obs with Middleware(), HealthHandler()
├── config/         Environment-based configuration with validation
├── log/            zerolog wrapper (JSON to stdout, collected by Promtail → Loki)
├── trace/          OpenTelemetry tracer provider setup
├── metrics/        Prometheus metrics registry and handler
├── middleware/     Chi middleware bundle (logging, tracing, metrics)
├── health/         Health check helpers (Checker builder, PgxCheck, RedisCheck)
└── stack/          Docker Compose monitoring stack
```

The root package `obs` re-exports types and functions from sub-packages for a
clean consumer API. Sub-packages can also be used independently.

## Development

```bash
make check    # fmt + vet + lint + test
make test     # run tests
make lint     # run golangci-lint
make vet      # run go vet
```

## License

Proprietary. See [LICENSE](LICENSE) for details.
