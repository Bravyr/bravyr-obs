# Collector and Backend Access Control

## Local Development

The monitoring stack runs with no authentication by default:

- **OTel Collector** accepts OTLP from any source on port 4317
- **Loki** accepts log pushes without authentication (`auth_enabled: false`)
- **Prometheus** has no authentication and exposes lifecycle API
- **All ports** are bound to `127.0.0.1` (loopback only)

This is acceptable for local development on a personal machine.

## Production Requirements

Before deploying to any shared or remotely accessible environment:

### OTel Collector

- Add TLS to the OTLP gRPC receiver (see comments in `stack/otel-collector/config.yaml`)
- Place behind a network policy that allows only your service pods
- Consider mTLS for service-to-collector authentication
- Remove `pprof` and `zpages` extensions (they expose runtime internals)

### Loki

- Set `auth_enabled: true` in `stack/loki/config.yaml`
- Configure Promtail with `X-Scope-OrgID` header matching your tenant
- Update Grafana Loki datasource with the same org ID
- Do NOT expose Loki's port (3100) outside the Docker network

### Prometheus

- Remove `--web.enable-lifecycle` flag (allows unauthenticated config reload and shutdown)
- Do not expose port 9090 to any external network
- Add basic auth via a reverse proxy if UI access is needed

### Grafana

- Set a strong `GRAFANA_ADMIN_PASSWORD` (the stack fails to start without it)
- Enable SSO/OAuth for team access
- Configure role-based access to limit dashboard and datasource visibility

### Trace and Log Data

- Spans contain tenant-identifying UUIDs (`user_id`, `workspace_id`)
- Log events may contain request metadata
- Restrict Tempo, Loki, and Grafana access to authorized team members
- Configure data retention policies for compliance (default: 72h for traces and logs)
