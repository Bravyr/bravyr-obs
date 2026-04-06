# bravyr-obs

Opinionated observability library for Go services. Structured logging,
distributed tracing, and Prometheus metrics in one `Init()` call.

## Quick Start

```go
package main

import (
	"context"
	"net/http"

	obs "github.com/bravyr/bravyr-obs"
	"github.com/bravyr/bravyr-obs/health"
	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

func main() {
	// Initialize your database pool (example).
	pool, _ := pgxpool.New(context.Background(), "postgres://localhost:5432/mydb")

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
	router.Get("/api/health", o.HealthHandler(map[string]health.CheckFunc{
		"db": func(ctx context.Context) error { return pool.Ping(ctx) },
	}))

	http.ListenAndServe(":8080", router)
}
```

## Installation

```bash
go get github.com/bravyr/bravyr-obs
```

## Features

| Feature | Package | Status |
|---|---|---|
| Structured logging (zerolog + Seq CLEF) | `log` | Planned |
| Distributed tracing (OpenTelemetry OTLP) | `trace` | Planned |
| Prometheus metrics | `metrics` | Planned |
| Chi middleware bundle | `middleware` | Planned |
| Health check endpoint | `health` | Available |
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
├── health/         Health check helpers (Postgres, Redis, Temporal)
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
