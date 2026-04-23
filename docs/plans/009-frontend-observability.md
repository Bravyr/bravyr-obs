# 009 — Frontend Observability (Grafana Faro)

Add frontend RUM + distributed tracing (browser → Go backend) to the stack.
Platform-agnostic SDK choice (Grafana Faro) keeps the path open for
post-React frameworks without reshaping the backend. Target consumer
repo: `../socialup` (React, bun). Target domains:
`https://socialup-staging.bravyr.com`, `https://socialup.bravyr.com`.

Ingest endpoint: `https://obs.bravyr.com/collect` (Traefik via Coolify → Alloy `faro.receiver`).

---

## 1. Product Owner

### Goal & user value

- **Founders** answer "is the app fast and stable for real users?" with per-route web-vitals percentiles and JS error rates, no second SaaS vendor.
- **Solo developer** jumps from a browser error straight to the exact Go handler span in one Tempo trace — cuts MTTR from guesswork to minutes.
- **Design/marketing** see which routes degrade INP/LCP after design changes, tied to release version.

### In scope

- Web-vitals capture: CLS, LCP, FCP, INP, TTFB per route.
- JS errors with stack traces + source-map resolution.
- Unhandled promise rejections.
- Console capture at `warn` + `error`.
- Browser→Go trace context propagation via W3C `traceparent` header (fetch + XHR instrumentation).
- Custom events API (`faro.api.pushEvent`) for product milestones.
- Opaque session ID + user ID tagging.
- Release/version tagging from build-time env var.
- Configurable sampling (default: 100% errors, 10% page loads in prod, 100% everything in staging/dev).
- Alloy `faro.receiver` exposed via Coolify/Traefik with basic-auth + CORS allowlist.
- New Grafana dashboard for frontend RUM.
- Integration guide addendum.

### Out of scope

- Session replay (PostHog owns this, already self-hosted).
- Product analytics funnels, retention cohorts, A/B flags — PostHog.
- Heatmaps.
- Mobile RUM.
- Feature flag evaluation pre-merge.
- Synthetic monitoring.

### Acceptance criteria

- [ ] Faro endpoint reachable at `https://obs.bravyr.com/collect`.
- [ ] CORS restricts `Origin` to staging + prod domains; other origins receive 403.
- [ ] Basic-auth enforced at Traefik layer (password = `FARO_APP_KEY`); unauthenticated requests rejected.
- [ ] A browser click triggering a backend call appears as a single Tempo trace spanning browser span → Go handler span with matching `trace_id`.
- [ ] Grafana dashboard renders p50/p75/p95 for each web-vital, grouped by route and release.
- [ ] Sampling rate is runtime-configurable via SDK init (no rebuild).
- [ ] No PII in payloads: no request bodies, no query strings with `token|key|auth|email`, IPs dropped before Loki write.
- [ ] Source maps uploaded to private symbolicator; `.map` files never served publicly.
- [ ] Tempo + Loki retention bumped to 14 days; disk growth alert in place.
- [ ] `README.md` + integration guide updated; CHANGELOG entry added.

### Risks / non-goals

- **Cardinality explosion** if route/user/session promoted to Loki labels.
- **GDPR** — IP + UA count as personal data in Loki; strip/hash at Alloy.
- **Vendor lock** — Faro SDK is Grafana-flavoured but OTel-compatible. Non-React apps can swap to `@opentelemetry/sdk-trace-web` without backend changes.

### Gap analysis

Go library side: nothing missing. Existing OTel setup accepts W3C `traceparent` already. Stack-side gaps:

1. Alloy config lacks `faro.receiver` component.
2. No public ingress for browser traffic (stack is internal-only).
3. No auth/CORS layer in front of Alloy.
4. No RUM dashboard JSON.
5. Tempo/Loki retention too short (72h) for frontend debugging workflows.
6. No source-map resolution path.

---

## 2. Software Architect

### Architecture

```
         [ Browser — socialup React ]
                    |
                    | HTTPS POST /collect
                    | Authorization: Basic base64("faro:$FARO_APP_KEY")
                    | Origin: https://socialup{-staging}.bravyr.com
                    v
         [ Coolify Traefik :443 ]
         (TLS · basic-auth · CORS · rate-limit · 1 MB body cap)
                    |
                    | http://bravyr_alloy:12347/collect
                    v
      +-------- [ Alloy faro.receiver "frontend" ] --------+
      |                 |                                  |
 otelcol.exporter.otlp  loki.process.faro             (monitoring
  → otel-collector:4317 → loki:3100                     network)
      |                 |
      v                 v
  [ Tempo ]         [ Loki ]        [ Prometheus ]  [ Grafana ]
                                      (unchanged)     (+ new RUM dash)
```

Alloy's `faro.receiver` has no native auth — Traefik terminates TLS and enforces basic-auth + CORS via Coolify labels. Alloy port `12347` stays on the internal `monitoring` network; Traefik is the only public entrypoint.

### File changes

- `stack/alloy/config.alloy` — extend with `faro.receiver "frontend"`, `loki.process "faro"`, `otelcol.exporter.otlp "tempo"`.
- `stack/docker-compose.yaml` — add Coolify/Traefik labels on `alloy` service; add env vars for allowed origins + app key; depend on `otel-collector`.
- `stack/tempo/config.yaml` — `block_retention` 72h → 14d; `max_block_bytes` 1 MB → 100 MB.
- `stack/loki/config.yaml` — `retention_period` 72h → 14d; `ingestion_rate_mb` 8 → 32; `ingestion_burst_size_mb` 16 → 64.
- `stack/.env.example` — add `FARO_ALLOWED_ORIGINS`, `FARO_APP_KEY`, `FARO_BASIC_AUTH_HTPASSWD`, `FARO_PUBLIC_HOST`.
- `stack/grafana/dashboards/frontend-rum.json` (new) — web-vitals percentiles, JS errors/min, session counts, error table with Tempo trace links.
- `docs/integration-guide.md` — frontend section with Faro SDK init + env-var contract.
- `README.md` — features bullet list.
- `CHANGELOG.md` — `## [Unreleased] → ### Added`.

### Alloy config shape (sketch)

```
faro.receiver "frontend" {
  server { listen_address = "0.0.0.0" listen_port = 12347
           cors_allowed_origins = string.split(env("FARO_ALLOWED_ORIGINS"), ",") }
  output { traces = [otelcol.exporter.otlp.tempo.input]
           logs   = [loki.process.faro.receiver] }
}
otelcol.exporter.otlp "tempo" { client { endpoint = "otel-collector:4317"
                                         tls { insecure = true } } }
loki.process "faro" { /* drop query strings, strip IP, relabel route, structured_metadata for user_id/session_id */
                      forward_to = [loki.write.loki.receiver] }
```

### Port & network topology

| Component | Port | Binding |
|-----------|------|---------|
| Alloy Faro receiver | `12347` | `monitoring` network, internal-only |
| Traefik (Coolify) | `443`, `80` | public, terminates TLS for `obs.bravyr.com` |
| Tempo, Loki, Prom, OTel Collector | (unchanged) | internal |

Dev overlay (`docker-compose.dev.yaml`) optionally publishes `127.0.0.1:12347:12347` for local testing without Coolify.

### Env var contract (socialup)

Naming follows `VITE_*` convention used by socialup:

- `VITE_FARO_URL` — `https://obs.bravyr.com/collect`
- `VITE_FARO_APP_NAME` — `socialup-web`
- `VITE_FARO_APP_VERSION` — git SHA injected at build time
- `VITE_FARO_ENVIRONMENT` — `development` | `staging` | `production`
- `VITE_FARO_SAMPLE_RATE` — float 0.0–1.0 (default 0.1 prod, 1.0 otherwise)
- `VITE_FARO_APP_KEY` — basic-auth password for ingest (per-env, rotatable)

### Build sequence

1. Stack changes (Alloy + compose + retention bumps) validated locally via `docker compose up`.
2. Coolify labels wired; ACME cert provisioned for `obs.bravyr.com`.
3. Dashboard JSON committed and provisioned.
4. Docs updated; `make lint` / `make test` / `make swagger` pass.
5. 5-agent pre-commit review; fix all findings.
6. PR → merge → deploy to Coolify staging.
7. Smoke test: curl from non-allowlisted origin → 403; allowlisted origin → 204.
8. Separate PR in `socialup` repo: add `@grafana/faro-web-sdk` + `@grafana/faro-web-tracing`, init in `src/main.tsx`, wire env vars.

### Decisions locked

- Reverse proxy: **Coolify/Traefik** via container labels (no Caddy container).
- Auth: **public write token** (basic-auth password), per-env, rotatable.
- Sampling: **prod 100% errors / 10% page loads / 100% web-vitals; staging + dev 100% all**.
- Retention bump: **included in this PR**.
- Ingest domain: **obs.bravyr.com/collect**.

---

## 3. Security Specialist

### Threat model

| Threat | Control |
|--------|---------|
| DoS / disk-bill-shock | Traefik rate-limit (100 r/s burst, 20 r/s sustained per IP); 1 MB body cap; Loki ingestion ceiling |
| Telemetry poisoning | Basic-auth + CORS allowlist + per-env token |
| Session/credential exfil in error payloads | Faro `beforeSend` redacts `Authorization`, `Cookie`, `X-*-Token` headers; drops `request.body` / `response.body`; truncates stack at 4096 chars |
| PII leak in URLs | `beforeSend` strips query strings; Alloy `stage.replace` as defence-in-depth |
| SSRF via source-map fetch | Confirm Alloy v1.14 `faro.receiver` does not auto-fetch source maps; if it does, disable or restrict to internal allowlist |
| CSRF | Not applicable — endpoint accepts no cookies |
| Log injection into Loki | Dedicated `loki.process "faro"` pipeline; fixed `source=faro` label, payload fields never promoted to labels; variable fields go to structured metadata |

### Auth model

Public write token (`FARO_APP_KEY`), bundled into browser build, validated as basic-auth password at Traefik layer. Per-environment (`FARO_APP_KEY_STAGING`, `FARO_APP_KEY_PROD`). Rotation = Coolify env var update + socialup rebuild. **Never reuse `INTERNAL_API_KEY`** — that key has Prom remote-write scope and must stay server-only.

### CORS

Strict allowlist. No wildcards. Reject on mismatch.

```
Access-Control-Allow-Origin: https://socialup-staging.bravyr.com
Access-Control-Allow-Origin: https://socialup.bravyr.com
```

Alloy `faro.receiver` `cors_allowed_origins` enforces at app layer; Traefik `headers` middleware enforces at edge as defence-in-depth.

### TLS

HTTPS-only. `Strict-Transport-Security: max-age=31536000; includeSubDomains`. ACME via Coolify/Traefik. Plain HTTP redirected to HTTPS.

### Payload size limit

1 MB body cap at Traefik (`maxRequestBodyBytes`). Return `413` on overage.

### PII scrubbing (socialup repo)

Faro `beforeSend` hook — highest-priority control for GDPR:

- Strip query strings from all captured URLs.
- Redact `Authorization`, `Cookie`, `X-Api-Key`, `X-Faro-*` header values in XHR breadcrumbs.
- Drop `request.body` and `response.body` entirely.
- Truncate `error.message` + `error.stack` at 4096 chars.
- Replace user email with opaque user ID.

### Source-map privacy

Do **not** serve `.map` files from the socialup CDN. Upload to a private symbolicator keyed by release ID. Resolution happens server-side.

### CSP

socialup `Content-Security-Policy` must allow:

```
connect-src 'self' https://obs.bravyr.com;
```

### Data classification

| Field | Store | Action |
|-------|-------|--------|
| Source IP | — | Drop at Alloy before Loki write |
| User-Agent | Loki structured metadata | Keep |
| User ID (opaque UUID) | Loki structured metadata | Keep |
| URL path | Loki log body | Keep after query strip |
| URL query string | — | Drop at Alloy (defence-in-depth) |
| Error stack | Loki log body | Keep; Alloy `stage.replace` strips `Bearer `, `sk-`, `eyJ` prefixes |
| Trace / span IDs | Tempo + Loki metadata | Keep |
| Session ID | Loki structured metadata | Keep |
| Request / response bodies | — | Drop at SDK + Alloy |

### GDPR / retention

Retention raised to 14 days in this PR (Tempo + Loki). Prom stays 30d (aggregates only). Document retention in privacy policy.

### Dependency risk

- Pin `@grafana/faro-web-sdk` + `@grafana/faro-web-tracing` to exact versions in socialup `package.json`.
- Bundle (don't load from CDN) — no SRI needed.
- `npm audit --audit-level=high` after install.
- Confirm Alloy `v1.14.0` `faro.receiver` is non-experimental.

### Open security items (follow-ups if required)

- Alerting: Prom alert when Loki ingest rate from `source=faro` exceeds 5× baseline over 5 min.
- Token rotation runbook: document in `docs/runbooks/`.

---

## 4. DBA / Storage

### Current retention (verified from config)

| Backend | Current | Target |
|---------|---------|--------|
| Tempo `block_retention` | 72 h | **14 d** |
| Loki `retention_period` | 72 h | **14 d** |
| Prometheus TSDB | 30 d | 30 d (unchanged) |
| Tempo `max_block_bytes` | 1 MB | **100 MB** |
| Loki `ingestion_rate_mb` | 8 | **32** |
| Loki `ingestion_burst_size_mb` | 16 | **64** |

Applied in this PR.

### Cardinality

| Label | Cardinality | Decision |
|-------|-------------|----------|
| `service` (`socialup-web`) | 1 | indexed label |
| `environment` | 3 | indexed label |
| `browser_name` | ~10 | indexed label |
| `os_name` | ~6 | indexed label |
| `release` | bounded by deploy cadence | indexed label |
| `route` | **normalized** (`/user/:id`, not `/user/12345`) | indexed label (SDK normalizes, Alloy regex strips `/:id` + `/:uuid` as backstop) |
| `url` (full) | unbounded | NEVER indexed; dropped before Loki write |
| `user_id` | unbounded | structured metadata only |
| `session_id` | unbounded | structured metadata only |

Normalization applied in the Alloy `loki.process "faro"` pipeline via `stage.template` + `regexReplaceAll` for numeric segments and UUID paths. The Faro SDK `beforeSend` hook normalizes too (belt + braces). Dashboard `by (route)` aggregations rely on `route` being a stream label — if this is ever demoted back to structured metadata, update the dashboard in the same PR.

### Volume estimate (10k DAU baseline, post-sampling)

```
10,000 DAU × 5 sessions × 20 events = 1,000,000 events/day
Avg log line                         ~500 B
Raw log volume                       ~500 MB/day
Loki compressed (~5:1)               ~100 MB/day
14-day Loki total                    ~1.4 GB

Traces @ 10% sampling               100,000/day
Avg trace size                       ~1 KB
Tempo raw                            ~100 MB/day
14-day Tempo total                   ~1.4 GB

Prom (web-vitals)                   <500 new series, <50 MB/30d
```

Total new disk demand ≈ **3 GB** at 14-day retention. Coolify VPS disk budget adequate; add `node_filesystem_avail_bytes` alert before ship.

### Sampling

Applied at the Faro SDK transport layer, not backend.

| Env | Errors | Page loads | Web-vitals |
|-----|--------|-----------|------------|
| Prod | 100% | 10% | 100% |
| Staging | 100% | 100% | 100% |
| Dev | 100% | 100% | 100% |

Tempo `metrics_generator` already runs `span-metrics` + `service-graphs` — frontend spans feed those automatically.

### Query patterns

```promql
-- LCP p95 by route
histogram_quantile(0.95, rate(faro_web_vitals_lcp_bucket[5m])) by (route)

-- Error rate by browser
rate(faro_errors_total[5m]) by (browser_name, environment)

-- JS error volume (Loki)
sum by (route) (count_over_time({service="socialup-web", level="error"}[5m]))
```

All grouping labels must exist as Prom metric labels / Loki stream labels — `route` normalization is the critical prerequisite.

### Risks

1. **Route cardinality bomb** — highest-probability failure. SPA with dynamic IDs → O(users) unique `route` values within hours. Enforce normalization in Alloy before write.
2. **Disk-full cascade** — shared Docker volume pool. Add `node_filesystem_avail_bytes` alert.
3. **Tempo tiny-block accumulation** — current 1 MB block size produces thousands of blocks/day at frontend scale. Bumped to 100 MB in this PR.
4. **Loki unauthenticated write path** — `auth_enabled: false` is acceptable because Alloy is the only client on the internal network and the Traefik ingress enforces auth. Document this invariant.

---

## 5. Deferred / follow-up issues

Create GitHub issues (per "Deferred Work Policy") for:

- Source-map private symbolicator setup + build-time upload pipeline (socialup side).
- Disk-full + `source=faro` ingest-rate Prom alerts.
- Token rotation runbook.
- socialup SDK integration PR (separate plan).
