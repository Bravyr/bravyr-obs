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
		SeqURL:       "http://seq:5341",
		OTLPEndpoint: "otel-collector:4317",
	})
	if err != nil {
		panic(err)
	}
	defer o.Shutdown(context.Background())

	router := chi.NewRouter()
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
    SeqURL:      "https://seq.internal:5341",
    SeqAPIKey:   os.Getenv("SEQ_API_KEY"),
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
| Ship logs to Seq | Set `SeqURL` (must be `https://` in non-dev mode) |
| Authenticate to Seq | Set `SeqAPIKey` |
| Minimum log level | `LogLevel: "debug"` / `"info"` / `"warn"` / `"error"` / `"fatal"` |

The `log` package can also be used standalone without the root facade:

```go
import obslog "github.com/bravyr/bravyr-obs/log"

logger, err := obslog.New(obslog.Config{
    ServiceName: "worker",
    Level:       "debug",
    DevMode:     true,
})
```

## Installation

```bash
go get github.com/bravyr/bravyr-obs
```

## Features

| Feature | Package | Status |
|---|---|---|
| Structured logging (zerolog + Seq CLEF) | `log` | Available |
| Distributed tracing (OpenTelemetry OTLP) | `trace` | Planned |
| Prometheus metrics | `metrics` | Planned |
| Chi middleware bundle | `middleware` | Planned |
| Health check endpoint | `health` | Available (typed checkers: Postgres, Redis) |
| Environment-based configuration | `config` | Available |
| Local monitoring stack (Docker Compose) | `stack` | Planned |

## Configuration

All configuration fields can be set via struct literal or environment variables:

| Field | Env Var | Default | Description |
|---|---|---|---|
| `ServiceName` | `OBS_SERVICE_NAME` | *(required)* | Name of the service for logs/traces |
| `Environment` | `OBS_ENVIRONMENT` | `development` | Deployment environment |
| `LogLevel` | `OBS_LOG_LEVEL` | `info` | Log level (trace, debug, info, warn, error, fatal, panic) |
| `SeqURL` | `OBS_SEQ_URL` | | Seq server URL for log shipping |
| `SeqAPIKey` | `OBS_SEQ_API_KEY` | | Seq API key (separate from URL for security) |
| `OTLPEndpoint` | `OBS_OTLP_ENDPOINT` | | OpenTelemetry Collector gRPC endpoint |
| `DevMode` | `OBS_DEV_MODE` | `false` | Enable pretty console logging |

## Architecture

```
github.com/bravyr/bravyr-obs
├── obs.go          Root facade: Init() returns *Obs with Middleware(), HealthHandler()
├── config/         Environment-based configuration with validation
├── log/            zerolog wrapper with Seq CLEF shipping
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
