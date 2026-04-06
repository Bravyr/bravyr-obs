# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

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
