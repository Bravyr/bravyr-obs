# 010 — Host Bind-Mount Persistence + Grafana 504 Diagnostics

## Context

`bravyr-obs` runs as a self-hosted Coolify stack on a single VPS. Two operational pains drive this work:

1. **Data loss on redeploy.** All five backend volumes (`prometheus_data`, `tempo_data`, `grafana_data`, `loki_data`, `alloy_data`) are unnamed Docker volumes. Coolify recreate operations have wiped the `alloy_data` volume in the past, which triggered a Loki "entry too far behind" log storm (`loki.source.docker` re-read every container log from byte zero, posting timestamps from 8+ days ago into ingesters whose `max_chunk_age` is 2h). The same vector also risks losing Prometheus TSDB, Tempo blocks, Loki chunks, and Grafana dashboards/users.
2. **Recurring Grafana 504 Gateway Timeout — root cause is the Coolify Traefik edge, not the obs stack.** Empirically the 504 cleared the moment the user redeployed an unrelated service in Coolify, without touching the observability stack. Coolify regenerates Traefik labels and reloads its config on any deploy, so any one of: stale router pointing at a dead upstream IP, broken healthcheck, dead persistent TCP connections in Traefik's upstream pool, or Coolify-managed certificate / route reconcile lag would clear on that reload. We do not have Traefik logs from the failure window, so this PR instruments the next occurrence rather than guessing at a fix.

Intended outcome: a Coolify redeploy preserves all five datastores; the next Grafana 504 surfaces as an alert with the diagnostic data needed to root-cause it (instead of a redeploy ritual); the Loki single-replica ring stops blocking Grafana queries on restart.

## Decisions Locked

- **Data root**: `/srv/bravyr-obs/data` on the Coolify host.
- **Backup**: none in this PR. Bind mounts already survive Coolify redeploys. Dashboards + datasources live in git. 14d of metrics/logs/traces is forensics-only — acceptable to lose if the VPS dies. Off-site backup deferred to a tracked follow-up issue, to be revisited when a customer-facing SLO needs it.
- **Loki "entry too far behind" fix** (today's log storm): folded into open PR #43 (Alloy `stage.drop { older_than = "1h" }`). This plan starts on a fresh branch after PR #43 merges.
- **Grafana admin password rotation**: no. Hash never left the named volume; no realistic exposure path. Keep current password. New `0600` perms on `grafana.db` after cutover lock down the file.
- **Bootstrap fail-fast on UID mismatch**: yes — refuse, do not auto-chown.
- **Pre-cutover tarball snapshot of current named volumes**: yes — one-shot insurance during cutover, stored under `/srv/bravyr-obs/backups/pre-cutover/`. Pruned after 14 days. No ongoing backup beyond this.

## Architecture

Host directory layout under a single base path. Each subdirectory is owned by the container UID that writes it.

```
/srv/bravyr-obs/
├── data/                                       # STACK_DATA_ROOT
│   ├── prometheus/   65534:bravyr-obs   0750   # Prom TSDB → /prometheus
│   ├── tempo/        10001:bravyr-obs   0750   # blocks + WAL → /var/tempo
│   ├── loki/         10001:bravyr-obs   0750   # chunks + tsdb index → /loki
│   ├── grafana/      472:bravyr-obs     0750   # sqlite + plugins → /var/lib/grafana
│   │   └── grafana.db                  0600
│   └── alloy/        473:bravyr-obs     0750   # positions + WAL → /var/lib/alloy/data
└── backups/                            0700    # root:root
    └── pre-cutover/                            # one-time tarballs of named volumes (pruned after 14d)
```

`bravyr-obs` is a host group created by the bootstrap script. Top-level `/srv/bravyr-obs` is `0750 root:bravyr-obs`.

Filesystem requirements: ext4 or xfs with journaling, mounted with `noatime`. SSD strongly preferred for Prom + Loki query latency. Recommend a dedicated 50 GB partition or LV at `/srv/bravyr-obs/data`.

## File Changes

| Path | Purpose |
|------|---------|
| `stack/docker-compose.yaml` | Replace 5 named volumes with bind mounts under `${STACK_DATA_ROOT}/<svc>`. Tighten Grafana healthcheck. Add `blackbox-exporter` service. |
| `stack/.env.example` | Add `STACK_DATA_ROOT=/srv/bravyr-obs/data`, `BLACKBOX_TARGETS` (comma-separated public URLs Prom should probe). |
| `stack/loki/config.yaml` | Pin instance identity to loopback so single-replica ring forms synchronously on restart (sec **Loki ring fix**). |
| `stack/blackbox-exporter/config.yaml` *(new)* | `http_2xx` probe module with 5s timeout. |
| `stack/prometheus/prometheus.yml` | Add `blackbox` scrape job covering Grafana public URL (and Faro `/collect`). Reference `rule_files`. |
| `stack/prometheus/rules/edge-health.yaml` *(new)* | Alerts: `GrafanaEdgeUnreachable`, `GrafanaEdgeLatencyHigh`. |
| `stack/grafana/provisioning/datasources/datasources.yaml` | Add `jsonData.timeout: 60` to Loki and Tempo (defensive hygiene). |
| `stack/scripts/bootstrap-host-dirs.sh` *(new)* | Idempotent first-boot host directory creation. Refuses on UID mismatch or path traversal. |
| `stack/scripts/migrate-volumes-to-bind.sh` *(new)* | One-shot cutover: tarball each named volume into `pre-cutover/`, then `cp -a` into the bind path with correct ownership. |
| `docs/coolify-deployment.md` | Replace named-volume language with bootstrap + bind-mount cutover runbook + Grafana 504 diagnosis runbook. |
| `CHANGELOG.md` | `### Changed` entry. |

## Bind-Mount Cutover (live deployment)

Run from the Coolify host shell. **Take the pre-cutover tarball before anything else.**

1. SSH to the host. Place the new `bootstrap-host-dirs.sh` and run it. It validates `STACK_DATA_ROOT`, creates the `bravyr-obs` group, creates each subdirectory with the right UID and `0750`, refuses if any target already exists with UID 0.
2. `docker compose stop` (not `down` — keeps named volumes for the copy step). Use SIGTERM to flush WAL on Prom, Loki, Tempo.
3. Pre-cutover tarball:
   ```bash
   for v in prometheus tempo loki grafana alloy; do
     docker run --rm \
       -v "bravyr-obs_${v}_data:/src:ro" \
       -v /srv/bravyr-obs/backups/pre-cutover:/dst \
       alpine tar czf "/dst/${v}-$(date +%F).tar.gz" -C /src .
   done
   ```
4. Per-volume copy with ownership (run from `migrate-volumes-to-bind.sh`):
   ```bash
   docker run --rm -v bravyr-obs_prometheus_data:/src \
     -v /srv/bravyr-obs/data/prometheus:/dst alpine sh -c \
     "cp -a /src/. /dst/ && chown -R 65534:65534 /dst"
   # repeat per service: tempo→10001, loki→10001, grafana→472, alloy→473
   ```
5. Merge the PR. Coolify pulls and recreates with the new `docker-compose.yaml`. Bind mounts attach to the populated host paths.
6. Verify (sec **Verification**).
7. After 24h of clean operation: `docker volume rm bravyr-obs_{prometheus,tempo,loki,grafana,alloy}_data`. Tarballs stay in `pre-cutover/` for 14 days then are pruned.

## Grafana 504 — Diagnosis-First, No Speculative Fix

The 504 the user sees is **browser → Coolify Traefik → Grafana** failing at the edge proxy hop. It clears the moment any service in Coolify gets redeployed, which makes Coolify regenerate Traefik labels and reload its config. The observability stack itself is healthy; the underlying datasource queries succeed once a fresh request gets through.

Root cause sits in Coolify's Traefik / proxy state — almost certainly one of:

- Stale router pointing at a dead upstream IP after a previous Grafana restart, never re-resolved.
- Broken healthcheck causing Traefik to mark Grafana unhealthy and fall back to nothing.
- Dead persistent TCP connections in Traefik's upstream pool, kept alive past the container's actual lifetime.
- Coolify-managed certificate or route reconcile lag.

We do not have Traefik logs from the failure window. Without those, fixing this preemptively is guessing. The plan therefore instruments so the next occurrence surfaces with the data needed to root-cause, instead of being a guess-and-redeploy ritual.

Concrete actions in this PR:

1. **Strengthen Grafana's container healthcheck** so Coolify (and any future probe) can distinguish "process up" from "API responsive". `stack/docker-compose.yaml` already has a healthcheck hitting `/api/health`; bump frequency and tighten the failure threshold so Coolify reacts within seconds instead of minutes:
   ```yaml
   healthcheck:
     test: ["CMD-SHELL", "wget -qO- http://localhost:3000/api/health || exit 1"]
     interval: 15s
     timeout: 3s
     retries: 3
     start_period: 30s
   ```
2. **External blackbox probe** of `https://grafana.bravyr.com/api/health` from outside Docker. Add `prom/blackbox-exporter` to the stack and a Prom job that hits the public URL. When Grafana is reachable internally but 504s externally, `probe_success` flips to 0 and an alert fires — that's the exact failure mode the user sees.
   - New file: `stack/blackbox-exporter/config.yaml` with an `http_2xx` module.
   - New file: `stack/prometheus/rules/edge-health.yaml` with `GrafanaEdgeUnreachable` and `GrafanaEdgeLatencyHigh` alerts.
   - Compose service `blackbox-exporter` on the monitoring network.
   - Prom scrape job pointed at the public Grafana URL (and any other public-facing observability endpoint, e.g. `https://obs.bravyr.com/collect`).
3. **Diagnosis runbook** (add to `docs/coolify-deployment.md`):
   - First, **don't redeploy yet** — that destroys the evidence.
   - From the host: `docker logs $(docker ps -q --filter name=coolify-proxy) --tail 200` — capture what Traefik says about the Grafana router right now.
   - `docker exec coolify-proxy wget -qO- http://bravyr_grafana:3000/api/health` — confirms internal health from inside Traefik's container. If this returns 200, the issue is purely in Traefik's edge handling.
   - `curl -v https://grafana.bravyr.com/api/health` from your laptop — capture the actual upstream error Traefik reports.
   - Inspect Traefik's API: `curl -s http://localhost:8080/api/http/routers | jq '.[] | select(.service | contains("grafana"))'` (port 8080 is Traefik's internal API; may need port-forward).
   - Once data is captured, trigger a Coolify "redeploy proxy" (not the whole stack) to force Traefik label reload — this is the smallest intervention that fixes the symptom.
   - File the captured logs as a follow-up issue so the next occurrence has comparison data.
4. **Datasource timeouts (defensive only)** — small hygiene unrelated to the 504 root cause but worth doing while we're in this file. Add `jsonData.timeout: 60` to Loki and Tempo in `stack/grafana/provisioning/datasources/datasources.yaml` so legitimately slow queries fail with a Grafana error message rather than a generic stack-trace.

**Out of scope for this PR**: changing Coolify's Traefik config, adding `dataproxy` overrides, or building a custom edge proxy. The user's confirmed evidence shows defaults work; we just need to detect when they stop.

## Loki Single-Replica Ring Fix

Symptom: `empty ring` and `context canceled` warnings during restart, blocking Grafana queries until Loki's ring re-stabilises. Cause: Coolify bridge hostnames are unstable across restarts; the ingester registers under a name it cannot re-resolve, so the ring stays empty.

Add to `stack/loki/config.yaml`:

```yaml
common:
  instance_addr: 127.0.0.1
  ring:
    kvstore:
      store: inmemory
    instance_addr: 127.0.0.1
memberlist:
  bind_addr: ["127.0.0.1"]
  join_members: []
```

With `replication_factor: 1` + inmemory KV + loopback, ring formation completes synchronously at boot. No empty-ring window, no early Grafana queries fanning into the 504 path.

## Tempo Timeouts

Skipped. Tempo defaults are fine; the Grafana 504 was a Coolify-Traefik edge issue, not a Tempo stall. Revisit only if a Tempo-specific stall surfaces after the blackbox probe is in place.

## Bootstrap Script

`stack/scripts/bootstrap-host-dirs.sh`:

```bash
#!/usr/bin/env bash
set -euo pipefail

ROOT="${STACK_DATA_ROOT:?STACK_DATA_ROOT must be set}"

case "$ROOT" in /*) ;; *) echo "STACK_DATA_ROOT must be absolute" >&2; exit 1 ;; esac
[[ "$ROOT" == *..* ]] && { echo "Path traversal detected" >&2; exit 1; }
resolved=$(realpath -m "$ROOT")
[[ "$resolved" == /srv/* ]] || { echo "Must resolve under /srv/" >&2; exit 1; }

groupadd -f bravyr-obs
gid=$(getent group bravyr-obs | cut -d: -f3)

declare -A DIR_UIDS=(
  [prometheus]=65534
  [tempo]=10001
  [loki]=10001
  [grafana]=472
  [alloy]=473
)

for name in "${!DIR_UIDS[@]}"; do
  uid="${DIR_UIDS[$name]}"
  dir="$ROOT/$name"
  mkdir -p "$dir"
  owner=$(stat -c '%u' "$dir")
  if [[ "$owner" -ne 0 && "$owner" -ne "$uid" ]]; then
    echo "$dir is owned by UID $owner — refusing to start (expected $uid)" >&2
    exit 1
  fi
  if [[ "$owner" -eq 0 && -n "$(ls -A "$dir" 2>/dev/null)" ]]; then
    echo "$dir is owned by UID 0 and non-empty — refusing to chown" >&2
    exit 1
  fi
  chown "$uid:$gid" "$dir"
  chmod 0750 "$dir"
done

[[ -f "$ROOT/grafana/grafana.db" ]] && chmod 0600 "$ROOT/grafana/grafana.db"

mkdir -p /srv/bravyr-obs/backups/pre-cutover
chmod 0700 /srv/bravyr-obs/backups
chown root:"$gid" /srv/bravyr-obs/backups

chmod 0750 /srv/bravyr-obs
chown root:"$gid" /srv/bravyr-obs
```

## Build Sequence

1. **Branch off `main` after PR #43 merges**: `feature/010-host-bind-mounts`. PR #43 carries the Alloy `stage.drop { older_than = "1h" }` patch separately.
2. Implement compose, Loki ring config, datasource timeouts, blackbox-exporter + edge-health alert rules, bootstrap + migration scripts, docs, CHANGELOG.
3. Run pre-commit checks: `make swagger`, `make lint`, `make test`.
4. Run the 5 pre-commit review agents (code-reviewer, qa, security, software-architect, dba) per CLAUDE.md. Fix all findings.
5. Open PR with `--assignee brunosansigolo` and labels copied from the linked issue (or `enhancement` if none).
6. SSH to Coolify host. Run `bootstrap-host-dirs.sh`. Confirm dirs exist with right UIDs.
7. Add Coolify secret: `STACK_DATA_ROOT=/srv/bravyr-obs/data`, plus `BLACKBOX_TARGETS` for the public probe URL(s).
8. Merge PR. Coolify redeploys.
9. Run cutover script: tarball each named volume into `pre-cutover/`, then `cp -a` into the bind path.
10. Coolify reconciles → bind mounts attach to populated dirs.
11. Smoke test (sec **Verification**).
12. After 24h clean: drop old named volumes.
13. Create follow-up GitHub issues for: disk-usage alert at 70% / 85% on `/srv/bravyr-obs/data`, off-site backup (revisit when SLO requires), optional LUKS at-rest encryption decision.

## Verification

End-to-end checks after the cutover deploys:

- **Persistence sanity**: stop the stack with `docker compose stop`, confirm `ls -la /srv/bravyr-obs/data/{prometheus,tempo,loki,grafana,alloy}` is non-empty with the expected UID owner. `docker compose start`. Grafana, Tempo, Loki, Prom resume serving without re-ingesting from zero.
- **Prometheus continuity**: in Grafana **Explore → Prometheus**, query `up{}` over the last 7 days. The line is continuous across the cutover timestamp with no gap longer than the cutover window.
- **Loki continuity**: `{service=~".+"} | __error__=""` over the last 7 days returns logs from before the cutover. No new "entry too far behind" lines after Alloy restart (PR #43 fix).
- **Tempo continuity**: open a trace ID generated yesterday in Grafana **Explore → Tempo**. Spans render.
- **Grafana state**: dashboards, datasources, users, API keys all present. Existing admin password still works (no rotation).
- **Alloy positions**: `docker compose stop alloy && docker compose start alloy`, then check Loki for any lines older than 5 minutes from any container — there should be none. Positions resumed cleanly.
- **Grafana 504 instrumentation**: blackbox-exporter scrapes successfully against the Grafana public URL. `probe_success` returns 1. Manually flip Grafana off (`docker compose stop grafana`) and confirm `GrafanaEdgeUnreachable` fires within 2 minutes. Restart Grafana, alert resolves.
- **Loki ring**: `curl -s http://loki:3100/ring` from inside the network returns a populated ring within 5s of container start, never stays empty.
- **Permissions**: `stat -c '%U:%G %a' /srv/bravyr-obs/data/grafana/grafana.db` returns `<grafana_uid>:bravyr-obs 600`.
- **Pre-cutover tarballs**: `ls -la /srv/bravyr-obs/backups/pre-cutover/` shows one tarball per service from cutover day. Manual `tar tzf` smoke check on one of them confirms readability.
- **Fail-fast**: as a separate test, `chown root /srv/bravyr-obs/data/loki && bootstrap-host-dirs.sh` should exit non-zero with a clear message. Restore correct ownership after.

## Out of Scope (follow-ups)

- Multi-node / HA Loki / Tempo (distributor + ingester + querier split).
- S3/GCS object-store backend for Loki / Tempo (currently filesystem).
- Hot/cold tiering and long-term retention beyond 14d.
- Loki multi-tenancy (`auth_enabled: true`) and per-tenant rate limits.
- Off-site backup of the data root (e.g. restic + R2/B2). Defer until a customer-facing SLO requires retained obs data after a VPS-loss event. Today the only irreplaceable bits are dashboards/datasources, which already live in git.
- Automated DR rehearsal as a scheduled job (no DR plan to rehearse until backup ships).
- LUKS at-rest encryption on the data partition (defer until customer PII lands in Loki).
- Disk-usage Prometheus alert at 70% / 85% on the `/srv/bravyr-obs/data` mount — already tracked as a deferred-work issue from PR #43; tighten to also watch the new mount.

Each of the above gets a tracked GitHub issue per the deferred-work policy if it isn't already covered by the existing #44–#49 set.
