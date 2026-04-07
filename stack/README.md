# Monitoring Stack

Local Docker Compose monitoring stack for bravyr-obs. Provides structured log
aggregation, distributed tracing, metrics scraping, and dashboards.

## Services

| Service           | Image                                    | Purpose                            | Port  |
|-------------------|------------------------------------------|------------------------------------|-------|
| Seq               | datalust/seq:2025.2                      | Structured log aggregation (CLEF)  | 5341  |
| OTel Collector    | otel/opentelemetry-collector-contrib     | OTLP receiver, trace/metrics fanout| 4317  |
| Tempo             | grafana/tempo:2.7.2                      | Distributed trace storage          | 3200  |
| Prometheus        | prom/prometheus:v3.3.1                   | Metrics scraping and storage       | 9090  |
| postgres-exporter | prometheuscommunity/postgres-exporter    | Postgres metrics for Prometheus    | 9187  |
| Grafana           | grafana/grafana:11.6.1                   | Dashboards                         | 3000  |

## EULA Requirement

Seq requires accepting its End-User License Agreement. The environment variable
`ACCEPT_EULA=Y` is set in `docker-compose.yml`. By starting the stack you accept
the Seq EULA: https://datalust.co/doc/seq-eula

## Prerequisites

- Docker Desktop 4.x or later (Mac/Windows) or Docker Engine + Compose plugin (Linux)
- On Linux only: add `--add-host=host.docker.internal:host-gateway` to the
  prometheus service in `docker-compose.yml` if your Go services run on the host

## Quick Start

### Full stack (all services)

```bash
cd stack
cp .env.example .env
# edit .env and set POSTGRES_DSN
docker compose --env-file .env up -d
```

### Dev stack (Seq + OTel Collector + Prometheus only)

```bash
cd stack
docker compose -f docker-compose.dev.yml up -d
```

## Configuring Your Go Service

Set these environment variables when running your Go service locally:

```bash
export OBS_SERVICE_NAME=my-service
export OBS_SEQ_URL=http://localhost:5341
export OBS_OTLP_ENDPOINT=localhost:4317    # host:port, no scheme
export OBS_DEV_MODE=true
export OBS_ENVIRONMENT=development
```

The OTel Collector listens on `localhost:4317` for OTLP/gRPC. The Go library uses
insecure gRPC when `OBS_DEV_MODE=true`, which matches the collector's plaintext
receiver config.

Prometheus scrapes `/metrics` from `host.docker.internal:8080` by default. Update
`prometheus/prometheus.yml` with the actual port your service listens on.

## Seq Grafana Plugin

The Seq data source in Grafana requires the `datalust-seq-datasource` plugin.
`GF_INSTALL_PLUGINS=datalust-seq-datasource` is set in `docker-compose.yml`.
Grafana downloads the plugin on first startup. Subsequent starts use the cached
version from the `grafana_data` volume.

Note: Seq has a capable built-in UI at http://localhost:5341 that is often
sufficient for log searching without Grafana. Use Grafana for cross-signal
correlation (logs + traces + metrics).

## Adding More Go Services

Edit `stack/prometheus/prometheus.yml` and add a new scrape config under the
`scrape_configs` section. A template is included in the file as a comment.

## Resetting State

```bash
docker compose down -v   # removes containers AND named volumes (all data)
```

## Volumes and Data Persistence

| Volume            | Service    | Contents                           |
|-------------------|------------|------------------------------------|
| seq_data          | Seq        | Log events and Seq config          |
| prometheus_data   | Prometheus | TSDB blocks (30-day retention)     |
| tempo_data        | Tempo      | Trace blocks (72h retention + WAL) |
| grafana_data      | Grafana    | Dashboards, users, plugin cache    |

Volumes persist across `docker compose restart`. Use `docker compose down -v`
to wipe all data.
