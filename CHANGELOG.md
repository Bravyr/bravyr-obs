# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added

- Coolify deployment guide (`docs/coolify-deployment.md`)
- `pgxtrace` sub-package for automatic pgx database query span instrumentation via otelpgx
- `CachedCheck()` wrapper that caches healthy results for a configurable TTL to prevent health check amplification
- OTLPEndpoint validation against loopback and link-local IP addresses in non-dev mode
- `TemporalCheck()` health helper with `TemporalChecker` interface
- PII field scrubbing for log output (`ScrubFields` config, `DefaultDenylist`)
- Collector access control documentation (`docs/collector-access-control.md`)
- Integration guide (`docs/integration-guide.md`) — step-by-step setup for any Go service
- socialup-api migration plan (`docs/plans/008-socialup-migration.md`)
- `stack/loki/config.yaml` — single-node Loki 3.5.0 config with filesystem storage, 72h log retention, tsdb schema v13, and compaction
- `stack/promtail/config.yaml` — Promtail 3.5.0 scrape config using Docker service discovery; extracts `level`, `service`, and `trace_id` from JSON log events; promotes `trace_id` to structured metadata for log-to-trace linking
- Loki datasource in Grafana provisioning with a `TraceID` derived field wired to Tempo, enabling one-click navigation from a log line to its trace
- Tempo `lokiSearch` and `tracesToLogsV2` wired to the Loki datasource UID for bidirectional log-to-trace and trace-to-log correlation in Grafana

### Changed

- Non-dev log output now writes to stdout (was stderr); this aligns with the 12-factor app convention and allows Promtail to collect logs via the Docker socket without additional configuration
- `stack/docker-compose.yml` — replaced Seq service with Loki + Promtail; updated Grafana `depends_on` to require Loki instead of Seq; removed `GF_INSTALL_PLUGINS` env var (no third-party Grafana plugins needed)
- `stack/docker-compose.dev.yml` — removed Seq service and `seq_data_dev` volume; dev stack now contains only OTel Collector and Prometheus; dev mode uses console output to stdout, no log backend required
- `stack/grafana/provisioning/datasources/datasources.yaml` — replaced Seq datasource with Loki datasource
- `stack/.env.example` — removed `SEQ_API_KEY` field
- `stack/README.md` — updated service list, architecture diagram, and quick-start instructions to reflect Loki + Promtail replacing Seq

### Removed

- Seq CLEF HTTP sink (`seqWriter`) from the `log` package — logs are now collected from container stdout by Promtail and shipped to Loki; no in-process HTTP shipping is needed
- `SeqURL` and `SeqAPIKey` fields from `config.Config` and `log.Config`
- `stack/docker-compose.yml` `seq` service and `seq_data` volume
- `stack/docker-compose.dev.yml` `seq` service and `seq_data_dev` volume
- `datalust-seq-datasource` Grafana plugin requirement

### Added (stack items from previous entry, carried forward)

- `stack/docker-compose.yml` — full monitoring stack: OTel Collector contrib 0.123.0, Tempo 2.7.2, Prometheus v3.3.1, postgres-exporter v0.17.1, Grafana 11.6.1; all services on a shared `monitoring` bridge network with named volumes for data persistence
- `stack/docker-compose.dev.yml` — lightweight dev stack (OTel Collector + Prometheus only) for fast local iteration
- `stack/prometheus/prometheus.yml` — scrape config targeting Prometheus self, OTel Collector self-metrics, postgres-exporter, and Go services on `host.docker.internal`; includes template for adding additional services
- `stack/otel-collector/config.yaml` — OTLP gRPC/HTTP receivers, memory_limiter + batch processors, OTLP exporter to Tempo, Prometheus exporter on port 8889, debug exporter; self-metrics on port 8888
- `stack/tempo/config.yaml` — single-node Tempo config with local filesystem storage, 72h trace retention, OTLP receiver, remote_write back to Prometheus for span metrics
- `stack/grafana/provisioning/dashboards/dashboards.yaml` — dashboard provider pointing to `/var/lib/grafana/dashboards`
- `stack/grafana/dashboards/http-overview.json` — HTTP overview dashboard with panels: request rate by route, 4xx/5xx error rate %, p50/p95/p99 latency, active connections gauge, summary stat row, p95 latency by route; `$metric_prefix` template variable supports `OBS_METRICS_PREFIX`
- `stack/.env.example` — documents required `POSTGRES_DSN` and Grafana admin credentials
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
