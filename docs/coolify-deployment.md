# Deploying the Monitoring Stack on Coolify

This guide covers deploying the bravyr-obs LGTM monitoring stack
(Loki, Grafana, Tempo, Prometheus) on Coolify.

## Prerequisites

- Coolify instance with Docker support
- The bravyr-obs repository connected to Coolify via GitHub

## Step 1: Create the Monitoring Service

1. Go to **Projects** → select your project (or create one)
2. Click **+ New** → **Service** → **Docker Compose**
3. Select the **bravyr-obs** repository
4. Set **Docker Compose file path**: `stack/docker-compose.yml`
5. Click **Deploy**

## Step 2: Configure Environment Variables

In the service's **Environment Variables** tab, add:

```
GRAFANA_ADMIN_USER=admin
GRAFANA_ADMIN_PASSWORD=<your-strong-password>
```

No other variables are required. The stack is self-contained — exporters
(postgres-exporter, redis-exporter) live in your application's compose,
not here.

## Step 3: Configure Networking

### Same server (recommended)

If your Go services and the monitoring stack run on the same Coolify server:

1. Deploy both services in the **same Coolify project** so they share a
   Docker network
2. Your Go service connects to the collector by container name:
   ```
   OBS_OTLP_ENDPOINT=bravyr_otel_collector:4317
   ```
3. Prometheus scrapes your Go service by container name — update
   `stack/prometheus/prometheus.yml`:
   ```yaml
   - job_name: "socialup-api"
     static_configs:
       - targets: ["socialup-api:8080"]
         labels:
           service: "socialup-api"
   ```

### Different servers

If the monitoring stack is on a separate server from your application:

1. Expose the OTel Collector port (4317) via Coolify's proxy with TLS
2. Your Go service connects via the public endpoint:
   ```
   OBS_OTLP_ENDPOINT=otel.yourdomain.com:443
   OBS_DEV_MODE=false
   ```
3. Prometheus scrapes via the public `/metrics` endpoint — add TLS and
   auth to the scrape config

## Step 4: Expose Grafana

Grafana is the only service that needs browser access:

1. In the monitoring service settings, configure a **domain** for Grafana
   (e.g., `grafana.yourdomain.com`)
2. Map it to container port `3000`
3. Coolify handles TLS automatically via Let's Encrypt

All other services (Prometheus, Loki, Tempo, OTel Collector, Promtail)
stay internal — they communicate over the Docker network.

| Service | Expose to internet? | Reason |
|---|---|---|
| Grafana (3000) | Yes | Dashboard access |
| OTel Collector (4317) | Only if cross-server | Trace/metric ingestion |
| Prometheus (9090) | No | Grafana queries internally |
| Loki (3100) | No | Promtail pushes internally |
| Tempo (3200) | No | Grafana queries internally |
| Promtail (9080) | No | Scrapes Docker socket locally |

## Step 5: Configure Your Go Service

In your application's Coolify service, add these environment variables:

```
OBS_SERVICE_NAME=socialup-api
OBS_ENVIRONMENT=staging
OBS_LOG_LEVEL=info
OBS_OTLP_ENDPOINT=bravyr_otel_collector:4317
OBS_SAMPLE_RATE=1.0
OBS_METRICS_PREFIX=socialup_api
OBS_DEV_MODE=false
```

For the worker service, use a separate service name:

```
OBS_SERVICE_NAME=socialup-worker
OBS_METRICS_PREFIX=socialup_worker
```

## Step 6: Add Exporters to Your Application Compose

Exporters live alongside their parent services in your **application's**
Docker Compose, not in the monitoring stack:

```yaml
# In socialup-api's docker-compose.yml:
postgres-exporter:
  image: prometheuscommunity/postgres-exporter:v0.17.1
  environment:
    DATA_SOURCE_NAME: "${POSTGRES_DSN}"

redis-exporter:
  image: oliver006/redis_exporter:v1.67.0
  environment:
    REDIS_ADDR: "redis:6379"
```

Then add scrape targets to `stack/prometheus/prometheus.yml`:

```yaml
- job_name: "postgres"
  static_configs:
    - targets: ["postgres-exporter:9187"]

- job_name: "redis"
  static_configs:
    - targets: ["redis-exporter:9121"]
```

## Step 7: Verify

After both services are deployed:

1. Open `grafana.yourdomain.com` → log in
2. **Explore → Prometheus** → query `up` → verify scrape targets are healthy
3. **Explore → Loki** → query `{service="socialup-api"}` → verify logs appear
4. **Explore → Tempo** → search by service name → verify traces appear
5. **Dashboards → HTTP Overview** → verify request rate, latency, error rate panels

### Troubleshooting

**No logs in Loki?**
- Verify Promtail is running: check its container logs in Coolify
- Verify your Go service writes JSON to stdout (`OBS_DEV_MODE=false`)
- Verify both services are on the same Docker network

**No traces in Tempo?**
- Verify `OBS_OTLP_ENDPOINT` points to the correct collector container name
- Verify the OTel Collector is running and healthy
- Check collector logs for connection errors to Tempo

**No metrics in Prometheus?**
- Verify your Go service exposes `/metrics` and Prometheus can reach it
- Check `prometheus/prometheus.yml` targets match actual container names
- Verify Prometheus is healthy: check its container logs

**Grafana shows "No data"?**
- Verify data sources are provisioned: **Settings → Data Sources** should
  show Prometheus, Loki, and Tempo
- Check the time range selector (default may be too narrow)

## Coolify-Specific Notes

- **Volumes**: Coolify manages Docker volumes — they persist across deploys
  and redeploys. Use Coolify's volume management UI to inspect or clear data.
- **Networks**: Coolify auto-creates networks per project. Services in the
  same project can reference each other by container name.
- **Environment variables**: Set in Coolify UI, not `.env` files. Coolify
  injects them at container start.
- **Health checks**: The compose file's `healthcheck` directives work as
  expected in Coolify. Coolify also runs its own health checks on top.
- **Updates**: To update the stack, push changes to the bravyr-obs repo
  and redeploy in Coolify. Volumes are preserved.

## Staging vs Production

| Setting | Staging | Production |
|---|---|---|
| `OBS_SAMPLE_RATE` | `1.0` (all traces) | `0.1` - `0.5` (10-50%) |
| `OBS_LOG_LEVEL` | `debug` | `info` |
| Prometheus retention | 30 days (default) | 30-90 days |
| Loki retention | 72 hours (default) | 7-30 days |
| Tempo retention | 72 hours (default) | 7 days |
| Grafana password | Strong | Strong + SSO/OAuth |

To change retention, edit the config files in `stack/` and redeploy.
