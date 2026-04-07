# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added

- `stack/docker-compose.yml` — full monitoring stack: Seq 2025.2, OTel Collector contrib 0.123.0, Tempo 2.7.2, Prometheus v3.3.1, postgres-exporter v0.17.1, Grafana 11.6.1; all services on a shared `monitoring` bridge network with named volumes for data persistence
- `stack/docker-compose.dev.yml` — lightweight dev stack (Seq + OTel Collector + Prometheus only) for fast local iteration
- `stack/prometheus/prometheus.yml` — scrape config targeting Prometheus self, OTel Collector self-metrics, postgres-exporter, and Go services on `host.docker.internal`; includes template for adding additional services
- `stack/otel-collector/config.yaml` — OTLP gRPC/HTTP receivers, memory_limiter + batch processors, OTLP exporter to Tempo, Prometheus exporter on port 8889, debug exporter; self-metrics on port 8888
- `stack/tempo/config.yaml` — single-node Tempo config with local filesystem storage, 72h trace retention, OTLP receiver, remote_write back to Prometheus for span metrics
- `stack/grafana/provisioning/datasources/datasources.yaml` — auto-provisioned Prometheus, Tempo, and Seq data sources; Prometheus exemplar linking wired to Tempo UID
- `stack/grafana/provisioning/dashboards/dashboards.yaml` — dashboard provider pointing to `/var/lib/grafana/dashboards`
- `stack/grafana/dashboards/http-overview.json` — HTTP overview dashboard with panels: request rate by route, 4xx/5xx error rate %, p50/p95/p99 latency, active connections gauge, summary stat row, p95 latency by route; `$metric_prefix` template variable supports `OBS_METRICS_PREFIX`
- `stack/.env.example` — documents required `POSTGRES_DSN`, optional `SEQ_API_KEY`, and Grafana admin credentials
- All ports bound to 127.0.0.1 (loopback only) to prevent network exposure
- Prometheus, Tempo, and postgres-exporter ports not exposed to host (internal Docker network only)

## [0.5.0] - 2026-04-07

### Added

- HTTP middleware bundle (`middleware.Bundle`) composing tracing, metrics, and request logging into a single Chi-compatible middleware
- `middleware.BundleConfig` with per-component enable/disable flags (`Tracing`, `Logging`, `Metrics`)
- `middleware.DefaultBundleConfig()` returns a config with all three components enabled
- Request logging with structured fields: `method`, `path`, `status`, `duration`, `request_id`
- Automatic `trace_id` and `span_id` injection into request log events when an OTel span is active, enabling log-to-trace correlation
- `X-Request-ID` response header support: valid incoming UUIDs are forwarded; invalid or absent values are replaced with a freshly generated UUID
- `metrics.Registry.RecordHTTPRequest()` — records duration histogram and request counter from a caller-supplied status code, enabling the bundle to share a single `statusWriter` rather than wrapping the response writer twice
- `metrics.Registry.IncrementActiveRequests()` and `metrics.Registry.DecrementActiveRequests()` — allow the bundle to manage the active-requests gauge without accessing internal fields
- `obs.MiddlewareWithConfig()` for selective component enablement

### Changed

- `obs.Middleware()` now returns the composed tracing + metrics + logging bundle (was trace-only)

## [0.4.0] - 2026-04-06

### Added

- Prometheus metrics registry using a custom (non-global) `prometheus.Registry` for test isolation (`metrics` package)
- Built-in HTTP instrumentation: `http_request_duration_seconds` histogram, `http_requests_total` counter, `http_active_requests` gauge — all partitioned by `method`, `path`, and `status_code`
- `metrics.HTTPMiddleware()` records per-request duration, total count, and active connections; path labels use Chi's `RoutePattern()` to prevent label cardinality explosion from path parameters
- `metrics.NewCounter()` and `metrics.NewHistogram()` helpers for custom business metrics scoped to the same isolated registry
- `/metrics` endpoint handler via `metrics.Handler()` (Prometheus text exposition format)
- `obs.Metrics()` accessor to retrieve the registry from the root facade
- `obs.MetricsHandler()` convenience method returning the Prometheus handler directly
- `MetricsPrefix` field in `config.Config` (env `OBS_METRICS_PREFIX`, default `""`) — prepended to all metric names with an underscore separator

### Changed

- `obs.Shutdown()` now calls `metrics.Shutdown()` between tracer and logger shutdown for consistent ordering across all providers

## [0.3.0] - 2026-04-06

### Added

- OpenTelemetry distributed tracing with OTLP/gRPC export (`trace` package)
- HTTP middleware for automatic span creation (`trace.HTTPMiddleware`) via `otelchi`
- User attributes middleware that enriches spans with `user_id` and `workspace_id` (`trace.UserAttributesMiddleware`)
- Configurable sampling rate: `SampleRate` field in `config.Config` (env `OBS_SAMPLE_RATE`, default `1.0`)
- W3C Trace Context and Baggage propagators registered globally on `Init()`
- `trace.Config` with `Validate()` for standalone trace package usage
- `Obs.TracerProvider()` accessor to expose the trace provider to callers
- tracestate header length cap in `HTTPMiddleware` to prevent memory pressure from malformed headers

### Changed

- `obs.Middleware()` now returns the OTel HTTP trace middleware instead of a pass-through
- `obs.Shutdown()` now flushes the tracer before the logger so final span-related log lines can still be emitted

## [0.2.0] - 2026-04-06

### Added

- Health check builder with `New()`, `AddCheck()`, and per-check timeouts (`WithCheckTimeout`)
- `PgxCheck()` and `RedisCheck()` typed checkers (interface-based, zero driver imports)
- Structured logging with zerolog (`log` package): `Logger` type with `Debug()`, `Info()`, `Warn()`, `Error()`, `Fatal()`, `With()`, and `Shutdown()`
- Seq CLEF HTTP async sink: batching up to 100 events per POST, 500 ms flush interval, channel buffer of 1024, dropped-event and send-failure counters
- Seq TLS enforcement: minimum TLS 1.2 on the HTTP client used for log shipping
- `obs.Obs.Logger()` accessor to expose the structured logger to callers
- `Config.Validate()` rejects `SeqURL` that does not start with `https://` when `DevMode` is false
- `Config.Validate()` rejects `DevMode=true` when `Environment` is `"production"` (case-insensitive)

## [0.1.0] - 2026-04-06

### Added

- Go module initialized as `github.com/bravyr/bravyr-obs`
- Project structure: config, health, log, trace, metrics, middleware, stack
- Root package facade with `Init()`, `Middleware()`, and `HealthHandler()` stubs
- Config package with `Config` struct, `Validate()`, and redacted `String()`
- Health package with `CheckFunc` type and `Handler` function
- GitHub Actions CI pipeline (lint, test, vet, govulncheck)
- Makefile with lint, test, vet, fmt, and check targets
- golangci-lint configuration
- README with module overview, usage example, and configuration reference
- Proprietary LICENSE
