# Issue #7 — Docker Compose Monitoring Stack

## Scope

Design and implement the local development monitoring stack for bravyr-obs.
Provides structured log aggregation (Seq), distributed tracing (Tempo via OTel Collector),
metrics (Prometheus + postgres-exporter), and dashboards (Grafana).

## Architecture Decisions

### Network Strategy

All services share a single `monitoring` bridge network named `monitoring`.
Services reference each other by container service name (e.g. `tempo:4317`, `seq:80`).
Go services running on the host are reached via `host.docker.internal` from inside
containers — this resolves automatically on Docker Desktop (Mac/Windows). On Linux,
`--add-host=host.docker.internal:host-gateway` must be added to the Prometheus service.

A single shared network is appropriate here: this is a monitoring stack for one developer,
not a multi-tenant environment requiring network isolation between components.

### Volume Strategy

Four named volumes provide data persistence across restarts:

| Volume          | Service    | Retention          |
|-----------------|------------|--------------------|
| seq_data        | Seq        | Until manual purge |
| prometheus_data | Prometheus | 30 days (TSDB)     |
| tempo_data      | Tempo      | 72 hours           |
| grafana_data    | Grafana    | Until manual purge |

Named volumes (not bind mounts) are used for service data so that the stack works
identically on Mac, Windows, and Linux without permission issues.
Configuration files (prometheus.yml, otel-collector/config.yaml, etc.) use read-only
bind mounts so changes to config files take effect after a container restart without
rebuilding anything.

### Seq EULA

Seq requires `ACCEPT_EULA=Y` as an environment variable. This is set in both
`docker-compose.yaml` and `docker-compose.dev.yaml`. Both files include a comment
at the top noting the EULA requirement and linking to https://datalust.co/doc/seq-eula.

### OTel Collector Pipeline

```
Go service (OBS_OTLP_ENDPOINT=localhost:4317)
    │
    ▼ OTLP gRPC (insecure, DevMode=true)
OTel Collector :4317
    │
    ├── memory_limiter → batch → otlp/tempo → Tempo :4317 (traces)
    └── memory_limiter → batch → prometheus exporter :8889 (metrics)
                                      │
                                      ▼
                              Prometheus scrapes :8889
```

The `memory_limiter` processor is placed first in every pipeline to act as a
circuit breaker. The `batch` processor reduces outgoing connections. The debug
exporter with `verbosity: detailed` is included for development visibility —
it should be set to `basic` or removed before production use.

### Prometheus Scrape Target Discovery

Static config is used with `host.docker.internal` to reach Go services on the host.
Service discovery (e.g. Docker SD, Consul) is not used — the operational overhead
is not justified for a single developer running a handful of services. Adding a new
service requires a one-line edit to `prometheus/prometheus.yml` and a config reload
(`curl -X POST http://localhost:9090/-/reload`), which is enabled via
`--web.enable-lifecycle`.

### Seq Grafana Data Source

The `datalust-seq-datasource` plugin is required. It is installed via
`GF_INSTALL_PLUGINS=datalust-seq-datasource` on first Grafana startup. The plugin
is cached in the `grafana_data` volume. Seq's own UI at `:5341` is often sufficient
for log searching; Grafana adds value for cross-signal correlation only.

### Grafana Dashboard Variable: $metric_prefix

bravyr-obs supports an optional `OBS_METRICS_PREFIX` that prepends a string to all
metric names (e.g. `myapp_http_requests_total`). The dashboard exposes this as a
free-text template variable `$metric_prefix`. Users set it to `myapp_` (with trailing
underscore) if a prefix was configured, or leave it empty for unprefixed metrics.
This avoids maintaining two sets of PromQL queries.

### Dev Stack vs Full Stack

`docker-compose.dev.yaml` omits Tempo and Grafana to keep startup time under 10 seconds
and memory usage under 512MB. The full stack (`docker-compose.yaml`) is used when
trace visualization or Grafana dashboards are needed. Both stacks use the same
`otel-collector/config.yaml` and `prometheus/prometheus.yml` — no duplication.

## Files Produced

```
stack/
├── .env.example
├── docker-compose.yaml
├── docker-compose.dev.yaml
├── otel-collector/
│   └── config.yaml
├── prometheus/
│   └── prometheus.yml
├── tempo/
│   └── config.yaml
└── grafana/
    ├── provisioning/
    │   ├── datasources/
    │   │   └── datasources.yaml
    │   └── dashboards/
    │       └── dashboards.yaml
    └── dashboards/
        └── http-overview.json
```

## Deferred Work

- Alertmanager integration (Prometheus alert rules + PagerDuty/Slack routing)
- TLS on OTel Collector receiver for production use
- Grafana dashboard for Postgres metrics (pg_stat_activity, replication lag, cache hit rate)
- Loki integration for container log aggregation (currently Seq handles structured app logs only)
- docker-compose.yaml `profiles` to selectively enable postgres-exporter when a DSN is available
