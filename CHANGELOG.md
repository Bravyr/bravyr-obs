# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added

- `stack/.env.example` — `GF_SERVER_ROOT_URL` field so Grafana's root URL can be set to the public FQDN behind Coolify/Traefik (defaults to `http://localhost:3000` for local dev)
- Frontend observability ingest (`faro.receiver` in Alloy) for Grafana Faro Web SDK clients; traces fan out to the existing OTel Collector, logs and measurements land in a dedicated Loki pipeline under `{source="faro"}`
- Coolify/Traefik container labels on Alloy for the public ingest endpoint at `https://obs.bravyr.com/collect` with basic-auth, CORS allowlist, 20 r/s rate limit, 1 MB body cap, and HSTS
- Grafana dashboard `Frontend RUM (Faro)` covering page loads, distinct sessions, JS exceptions, Core Web Vitals (LCP/INP/CLS p75 by route), browser/OS/route breakdowns, and an exception table with Tempo trace links
- Defence-in-depth credential scrubbing in the Alloy Faro Loki pipeline (`Bearer …`, `sk-…`, `eyJ…` prefixes redacted); `user_id`, `session_id`, `trace_id`, `release`, `browser`, `os`, `route` attached as Loki structured metadata to avoid index cardinality blow-ups
- Frontend observability plan (`docs/plans/009-frontend-observability.md`)
- Host bind-mount + Grafana 504 diagnostics plan (`docs/plans/010-host-bind-mounts.md`) — next iteration after this PR
- Integration guide addendum documenting the Faro SDK env var contract for the `socialup` React app
- `stack/scripts/smoke-faro.sh` — end-to-end smoke test that posts a synthetic Faro payload to the local Alloy receiver and asserts the span surfaces in Tempo and the log surfaces in Loki
- `alloy-fmt` CI job that runs `alloy fmt --test` against `stack/alloy/config.alloy` on every PR, catching syntax regressions before merge
- Alloy container healthcheck on `/-/ready` (port 12345) so orchestrators surface crashes instead of silently continuing
- 4-agent review fixes for socialup-api#674's pgBackRest alerting. **`stack/docker-compose.yaml`: Prometheus attached to the external `coolify` network** (additive — keeps the existing `monitoring` bridge). Without this, `socialup-postgres-exporter:9187` and `socialup-node-exporter:9100` (a different Coolify project's containers) were unreachable from this stack's Prometheus, and every alert depending on them would sit in permanent NoData — this also retroactively fixes the same latent gap for the pre-existing `socialup-api` (`api:8080`) scrape job, which relied on the same network attachment and was equally affected. New `coolify: external: true` network declaration alongside the existing `monitoring` one. **`stack/grafana/provisioning/alerting/rules-pgbackrest.yaml` and `rules-node-disk.yaml`: every rule (9 total) now sets `noDataState: Alerting` / `execErrState: Alerting`**, overriding Grafana's defaults (`NoData`/`Error`, neither of which pages by default) — for a backup-monitoring stack, a scrape outage or query error means "backups are now unmonitored," which must page rather than sit silently unevaluated. Verified valid provisioning keys against this stack's Grafana version (`grafana/grafana:12.4.2`) and the apiVersion-1 file-provisioning schema in use. `socialup-api/docs/deployment-guide.md` (production step) and this repo's `stack/README.md` gain a post-deploy verification step: confirm `up{job=~"socialup-.*"} == 1` for all three socialup jobs before declaring #674 §10 alerting done — a `0`/missing series means the target is unreachable and its alerts are silently not evaluating regardless of what Grafana's alert list shows.
- Functional Grafana alerting-as-code for socialup-api's pgBackRest PITR (issue #674 §10 release-blocker). New `stack/grafana/provisioning/alerting/` (auto-loaded by Grafana's existing `provisioning:` bind mount, no compose change needed): `contact-points.yaml` (a `pgbackrest-oncall` webhook contact point, URL sourced from the new `ALERT_WEBHOOK_URL` env var via Grafana's built-in `${ENV_VAR}` provisioning expansion — no real webhook URL committed), `notification-policies.yaml` (root org policy routed to that contact point — there was no policy tree before this), and `rules-pgbackrest.yaml` with two alert rules against `pg_stat_archiver_*` metrics (exposed by socialup-api's `socialup-postgres-exporter` via a custom postgres-exporter `queries.yaml`, since postgres-exporter v0.17.0 has no built-in `pg_stat_archiver` collector — verified against the exporter's own source): `PgBackRestArchiveStalled` (`time() - pg_stat_archiver_last_archived_time_seconds > 600` for 5m — no successful WAL archive-push in >10 minutes, abnormal given `archive_timeout=60s`) and `PgBackRestArchiveFailuresOutpacingSuccesses` (`increase(pg_stat_archiver_failed_count[10m]) - increase(pg_stat_archiver_archived_count[10m]) > 0`). `stack/prometheus/prometheus.yml` gains a real `socialup-postgres-exporter` scrape job (`socialup-postgres-exporter:9187` on the shared `coolify` network), replacing the commented template. `stack/.env.example` gains `ALERT_WEBHOOK_URL` (placeholder). Deliberately **not** shipped: backup-freshness/stanza-check alerts (`pgbackrest info`/`check` output) and a WAL/pgdata disk-usage alert — both require a node-exporter with `--collector.textfile.directory`, which is not currently deployed anywhere in the active split-stack Coolify architecture (only the legacy, being-phased-out `socialup-api` monolith defines a node-exporter, and even that one has no textfile-collector flag). Documented as an explicit remaining gap in `socialup-api` `docs/runbooks/postgres-restore.md` "Alerting" rather than shipped as inert config.
- Closed the two gaps deliberately left open above (backup-freshness/stanza-check + disk-usage), now that socialup-api deploys a standalone node-exporter (`coolify/stacks/node-exporter.yml`, container `socialup-node-exporter`) with `--collector.textfile.directory` fed by its own `pgbackrest_last_{full,diff,incr}_backup_timestamp_seconds` / `pgbackrest_check_success` / `pgbackrest_backup_error` / `pgbackrest_textfile_last_run_seconds` textfile-collector script. `stack/prometheus/prometheus.yml` gains a `socialup-node-exporter` scrape job (`socialup-node-exporter:9100`, `service: socialup-host` label) replacing the "not wired here yet" placeholder comment. `stack/grafana/provisioning/alerting/rules-pgbackrest.yaml` gains a second rule group, `pgbackrest-backup-health` (same folder, same `pgbackrest-oncall` contact point as the existing archive-stall rules): `PgBackRestFullBackupStale` (`time() - pgbackrest_last_full_backup_timestamp_seconds > 8d`, matching the weekly-full schedule plus a day of grace), `PgBackRestDiffBackupStale` (`> 30h`, matching the daily-diff schedule plus grace), `PgBackRestStanzaCheckFailed` (`pgbackrest_check_success == 0` for 15m), `PgBackRestBackupErrorReported` (`pgbackrest_backup_error == 1` for 15m), and `PgBackRestTextfileScriptStale` (warning; `time() - pgbackrest_textfile_last_run_seconds > 30m` — a staleness guard for the script/cron itself, needed because node-exporter's textfile collector keeps re-serving the last-written values on every scrape regardless of file age, so metric *absence* can't detect a stopped cron job on its own). New sibling file `rules-node-disk.yaml` (folder "Host Resources", same contact point) with `HostRootDiskUsageWarning` (70%, 10m) and `HostRootDiskUsageCritical` (85%, 5m) against `node_filesystem_avail_bytes` / `node_filesystem_size_bytes` at `mountpoint="/"`, excluding `fstype="rootfs"` (a synthetic duplicate `/` entry some kernels expose — verified node_exporter's default filesystem-type exclude regex does not already filter it). Targets the host root filesystem rather than a pgdata-specific mount, an assumption documented in-file (single-disk VPS, no separate Docker data-root disk). `stack/README.md`'s node-exporter section gains a note on why socialup-api runs it containerized (the textfile-collector requirement) as a documented exception to the "prefer host systemd" guidance, rather than a silent contradiction of it.
- `redistrace` sub-package for Redis command span instrumentation via redisotel (`db.statement` off by default)
- `temporaltrace` sub-package for Temporal workflow/activity span instrumentation via the Temporal OTel contrib interceptor
- Prometheus bearer token auth for `/metrics` scraping via `INTERNAL_API_KEY` env var
- `OTLPInsecure` config field (`OBS_OTLP_INSECURE`) for plaintext gRPC to internal collectors without enabling DevMode
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

- Tempo `block_retention` raised from 72h to 14 days and `max_block_bytes` from 1 MB to 100 MB to absorb frontend trace volume without tiny-block accumulation
- Loki `retention_period` raised from 72h to 14 days; `ingestion_rate_mb` raised from 8 to 32 (burst 64) to accommodate Faro payloads; `allow_structured_metadata: true` added
- `stack/docker-compose.yaml` — Alloy service now depends on `otel-collector` (Faro traces fan out through it), requires `FARO_ALLOWED_ORIGINS` env var, and carries Traefik middleware labels for the public `/collect` route
- `stack/.env.example` — added `FARO_PUBLIC_HOST`, `FARO_ALLOWED_ORIGINS`, `FARO_APP_KEY`, `FARO_BASIC_AUTH_HTPASSWD`
- Non-dev log output now writes to stdout (was stderr); this aligns with the 12-factor app convention and allows Promtail to collect logs via the Docker socket without additional configuration
- `stack/docker-compose.yaml` — replaced Seq service with Loki + Promtail; updated Grafana `depends_on` to require Loki instead of Seq; removed `GF_INSTALL_PLUGINS` env var (no third-party Grafana plugins needed)
- `stack/docker-compose.dev.yaml` — removed Seq service and `seq_data_dev` volume; dev stack now contains only OTel Collector and Prometheus; dev mode uses console output to stdout, no log backend required
- `stack/grafana/provisioning/datasources/datasources.yaml` — replaced Seq datasource with Loki datasource
- `stack/.env.example` — removed `SEQ_API_KEY` field
- `stack/README.md` — updated service list, architecture diagram, and quick-start instructions to reflect Loki + Promtail replacing Seq
- `stack/docker-compose.yaml` — Grafana `GF_SERVER_ROOT_URL` is now env-driven (`${GF_SERVER_ROOT_URL:-http://localhost:3000}`) instead of hardcoded to localhost

### Fixed

- `stack/docker-compose.yaml` — pin `traefik.docker.network: coolify` on the Grafana service. Under Coolify, Grafana attaches to multiple Docker networks; without this label Traefik could resolve Grafana to an unroutable network IP, causing a 504 Gateway Timeout when accessing the dashboard through its public domain

### Removed

- Seq CLEF HTTP sink (`seqWriter`) from the `log` package — logs are now collected from container stdout by Promtail and shipped to Loki; no in-process HTTP shipping is needed
- `SeqURL` and `SeqAPIKey` fields from `config.Config` and `log.Config`
- `stack/docker-compose.yaml` `seq` service and `seq_data` volume
- `stack/docker-compose.dev.yaml` `seq` service and `seq_data_dev` volume
- `datalust-seq-datasource` Grafana plugin requirement

### Added (stack items from previous entry, carried forward)

- `stack/docker-compose.yaml` — full monitoring stack: OTel Collector contrib 0.123.0, Tempo 2.7.2, Prometheus v3.3.1, postgres-exporter v0.17.1, Grafana 11.6.1; all services on a shared `monitoring` bridge network with named volumes for data persistence
- `stack/docker-compose.dev.yaml` — lightweight dev stack (OTel Collector + Prometheus only) for fast local iteration
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
