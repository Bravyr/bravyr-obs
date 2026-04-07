# Monitoring Stack

Local Docker Compose monitoring stack for bravyr-obs. Provides log aggregation,
distributed tracing, metrics scraping, and dashboards.

## Services

| Service           | Image                                    | Purpose                              | Port  |
|-------------------|------------------------------------------|--------------------------------------|-------|
| Loki              | grafana/loki:3.5.0                       | Log aggregation backend              | 3100  |
| Promtail          | grafana/promtail:3.5.0                   | Log collector (Docker stdout → Loki) | 9080  |
| OTel Collector    | otel/opentelemetry-collector-contrib     | OTLP receiver, trace/metrics fanout  | 4317  |
| Tempo             | grafana/tempo:2.7.2                      | Distributed trace storage            | 3200  |
| Prometheus        | prom/prometheus:v3.3.1                   | Metrics scraping and storage         | 9090  |
| Grafana           | grafana/grafana:11.6.1                   | Dashboards                           | 3000  |

## Architecture

```
Go service stdout (JSON)
        │
        ▼
   [Promtail]  ── scrapes container stdout via Docker socket
        │
        ▼
     [Loki]   ── stores log streams, indexed by labels (service, level)
        │
        ▼
   [Grafana]  ── queries Loki for logs, Tempo for traces, Prometheus for metrics
```

Trace IDs written into log events (`trace_id` field) are extracted by Loki's
pipeline stage and stored as structured metadata. Grafana's Loki derived fields
and Tempo `tracesToLogsV2` wire log-to-trace and trace-to-log navigation.

## Prerequisites

- Docker Desktop 4.x or later (Mac/Windows) or Docker Engine + Compose plugin (Linux)
- On Linux only: add `--add-host=host.docker.internal:host-gateway` to the
  prometheus service in `docker-compose.yaml` if your Go services run on the host
- Promtail requires access to the Docker socket (`/var/run/docker.sock`) and
  container log directory (`/var/lib/docker/containers`)

## Quick Start

### Full stack (all services)

```bash
cd stack
cp .env.example .env
# edit .env and set GRAFANA_ADMIN_PASSWORD
docker compose --env-file .env up -d
```

### Dev stack (OTel Collector + Prometheus only)

```bash
cd stack
docker compose -f docker-compose.dev.yaml up -d
```

In dev mode, the Go service writes human-readable console output to stdout.
No log backend is needed locally — read logs directly from the terminal or
`docker compose logs`.

## Configuring Your Go Service

Set these environment variables when running your Go service locally:

```bash
export OBS_SERVICE_NAME=my-service
export OBS_OTLP_ENDPOINT=localhost:4317    # host:port, no scheme
export OBS_DEV_MODE=true
export OBS_ENVIRONMENT=development
```

The OTel Collector listens on `localhost:4317` for OTLP/gRPC. The Go library uses
insecure gRPC when `OBS_DEV_MODE=true`, which matches the collector's plaintext
receiver config.

Prometheus scrapes `/metrics` from `host.docker.internal:8080` by default. Update
`prometheus/prometheus.yml` with the actual port your service listens on.

In production (non-dev mode), the Go library writes structured JSON to stdout.
Promtail collects those logs via the Docker socket and forwards them to Loki.
No additional configuration is required in the Go service.

## Adding More Go Services

Edit `stack/prometheus/prometheus.yml` and add a new scrape config under the
`scrape_configs` section. A template is included in the file as a comment.

Promtail automatically discovers all running containers via the Docker socket.
Logs from any container are forwarded to Loki with `container` and `service`
labels derived from the container name and `service` label respectively.

## Exporters (postgres-exporter, redis-exporter, node-exporter)

Exporters should live alongside their parent services, not in this stack. This
stack is backend-agnostic — it receives and stores telemetry, not produce it.

Add exporters to your **application's** Docker Compose file:

```yaml
# In your app's docker-compose.yaml:
postgres-exporter:
  image: prometheuscommunity/postgres-exporter:v0.17.1
  environment:
    DATA_SOURCE_NAME: "${POSTGRES_DSN}"
  ports:
    - "127.0.0.1:9187:9187"

redis-exporter:
  image: oliver006/redis_exporter:v1.67.0
  environment:
    REDIS_ADDR: "redis:6379"
  ports:
    - "127.0.0.1:9121:9121"
```

Then add scrape targets to `stack/prometheus/prometheus.yml`:

```yaml
- job_name: "postgres"
  static_configs:
    - targets: ["host.docker.internal:9187"]

- job_name: "redis"
  static_configs:
    - targets: ["host.docker.internal:9121"]
```

### node-exporter

node-exporter monitors **host-level** metrics (CPU, memory, disk, network). It
should run directly on the host machine, not inside Docker — running it in a
container limits visibility to the container's cgroup.

**If you use Coolify**: Coolify provides built-in server monitoring (CPU, memory,
disk) via its dashboard. You likely don't need node-exporter unless you want
those metrics **in Grafana** alongside application metrics for a unified view.

**If you still want it**: Install as a systemd service on the VM:

```bash
# Ubuntu/Debian:
sudo apt install prometheus-node-exporter
# Verify: curl http://localhost:9100/metrics
```

Then add to `stack/prometheus/prometheus.yml`:

```yaml
- job_name: "node"
  static_configs:
    - targets: ["host.docker.internal:9100"]
```

## Resetting State

```bash
docker compose down -v   # removes containers AND named volumes (all data)
```

## Volumes and Data Persistence

| Volume            | Service    | Contents                           |
|-------------------|------------|------------------------------------|
| loki_data         | Loki       | Log streams (72h retention)        |
| prometheus_data   | Prometheus | TSDB blocks (30-day retention)     |
| tempo_data        | Tempo      | Trace blocks (72h retention + WAL) |
| grafana_data      | Grafana    | Dashboards, users, plugin cache    |

Volumes persist across `docker compose restart`. Use `docker compose down -v`
to wipe all data.
