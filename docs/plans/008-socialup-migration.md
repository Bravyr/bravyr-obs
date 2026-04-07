# Migration Plan: socialup-api to bravyr-obs

## Current State

### Logging
- `cmd/api/main.go:67-76`: `zerolog.New(os.Stdout)` with ConsoleWriter in dev
- `cmd/worker/main.go:24-33`: same pattern + `temporalLogger` adapter (lines 170-203)
- No structured request logging middleware
- `internal/config/config.go`: `SeqURL`, `SeqAPIKey`, `Env` fields (Seq not actively used)

### Router & Middleware
- `internal/server/server.go:45`: `chi.NewRouter()`
- Middleware (lines 80-93): RequestID, RealIP, Recoverer, Timeout(30s), securityHeaders, CORS
- No tracing, no metrics collection, no request duration logging

### Health Check
- `internal/server/server.go:540-562`: manual `handleHealth` — pings DB, returns `{"status":"ok"}` or `{"status":"degraded"}`
- Used by Coolify health checks: `wget -qO- http://localhost:8080/api/health | grep -q ok`

### Infrastructure
- `coolify/staging.yml` and `coolify/production.yml` contain Seq, Prometheus, Grafana, exporters
- These will be replaced by the bravyr-obs monitoring stack

## Migration Steps

### Step 1: Add dependency

```bash
cd socialup-api
go get github.com/bravyr/bravyr-obs
```

### Step 2: Update config (`internal/config/config.go`)

Add new fields, remove Seq fields:

```go
// Remove:
// SeqURL    string `env:"SEQ_URL" envDefault:"http://localhost:5341"`
// SeqAPIKey string `env:"SEQ_API_KEY"`

// Add:
OTLPEndpoint  string  `env:"OBS_OTLP_ENDPOINT"`
SampleRate    float64 `env:"OBS_SAMPLE_RATE" envDefault:"1.0"`
MetricsPrefix string  `env:"OBS_METRICS_PREFIX" envDefault:"socialup_api"`
```

### Step 3: Replace logger in API (`cmd/api/main.go`)

```go
// Before (lines 67-76):
logger := zerolog.New(os.Stdout).With().Timestamp().Logger()
cfg, err := config.Load()
if cfg.IsDev() {
    logger = logger.Output(zerolog.ConsoleWriter{Out: os.Stdout})
}

// After:
cfg, err := config.Load()
if err != nil {
    log.Fatalf("failed to load config: %v", err)
}

o, err := obs.Init(obs.Config{
    ServiceName:   "socialup-api",
    Environment:   cfg.Env,
    LogLevel:      "info",
    OTLPEndpoint:  cfg.OTLPEndpoint,
    SampleRate:    cfg.SampleRate,
    DevMode:       cfg.IsDev(),
    MetricsPrefix: cfg.MetricsPrefix,
})
if err != nil {
    log.Fatalf("failed to init observability: %v", err)
}
defer o.Shutdown(context.Background())

logger := o.Logger()
```

Pass `o` to the Server constructor so it can access Middleware, HealthHandler, MetricsHandler.

### Step 4: Replace logger in Worker (`cmd/worker/main.go`)

Same pattern as API. The `temporalLogger` adapter stays but wraps `o.Logger()`:

```go
o, err := obs.Init(obs.Config{
    ServiceName:   "socialup-worker",
    Environment:   cfg.Env,
    LogLevel:      "info",
    OTLPEndpoint:  cfg.OTLPEndpoint,
    DevMode:       cfg.IsDev(),
    MetricsPrefix: "socialup_worker",
})
defer o.Shutdown(context.Background())

temporalClient, _ = client.Dial(client.Options{
    Logger: newTemporalLogger(o.Logger()),
})
```

### Step 5: Add middleware to Chi router (`internal/server/server.go`)

In `setupMiddleware()`, add obs middleware first:

```go
func (s *Server) setupMiddleware() {
    // Observability middleware (tracing + metrics + request logging)
    s.router.Use(s.obs.Middleware())

    // Existing middleware (keep all)
    s.router.Use(chimw.RequestID)
    s.router.Use(chimw.RealIP)
    s.router.Use(chimw.Recoverer)
    s.router.Use(chimw.Timeout(30 * time.Second))
    s.router.Use(securityHeaders)
    s.router.Use(cors.Handler(s.corsOptions()))
}
```

### Step 6: Replace health check

Replace `handleHealth` (lines 540-562) with obs health checker:

```go
// In route setup:
checker := health.New()
checker.AddCheck("postgres", health.PgxCheck(s.db))
checker.AddCheck("redis", health.RedisCheck(s.redis))
s.router.Get("/api/health", checker.Handler())

// Delete handleHealth method
```

Note: the response format changes from `{"status":"ok"}` to `{"status":"healthy","checks":[...]}`. Update the Coolify healthcheck command:

```yaml
# Before:
test: ["CMD-SHELL", "wget -qO- http://localhost:8080/api/health | grep -q ok"]
# After:
test: ["CMD-SHELL", "wget -qO- http://localhost:8080/api/health | grep -q healthy"]
```

### Step 7: Add /metrics endpoint

```go
s.router.Handle("/metrics", s.obs.MetricsHandler())
```

### Step 8: Add user attributes to traces

After auth middleware is registered, add user attributes:

```go
s.router.Use(trace.UserAttributesMiddleware(
    func(r *http.Request) string {
        u := auth.UserFromContext(r.Context())
        if u != nil { return u.ID }
        return ""
    },
    func(r *http.Request) string {
        // Extract workspace ID from URL if available
        return chi.URLParam(r, "workspaceId")
    },
))
```

### Step 9: Add database query tracing

```go
import "github.com/bravyr/bravyr-obs/pgxtrace"

// When creating the pgx pool:
poolConfig, _ := pgxpool.ParseConfig(cfg.DatabaseURL)
poolConfig.ConnConfig.Tracer = pgxtrace.NewTracer()
pool, _ := pgxpool.NewWithConfig(ctx, poolConfig)
```

Every SQL query will appear as a child span in traces with parameterized SQL.

### Step 10: Add custom business metrics

```go
// In a metrics.go or wherever appropriate:
postsPublished, _ := o.Metrics().NewCounter("posts_published_total", "Posts published", []string{"workspace"})
inboxSynced, _ := o.Metrics().NewCounter("inbox_messages_synced_total", "Inbox messages synced", []string{"provider"})
aiResponses, _ := o.Metrics().NewHistogram("ai_response_seconds", "AI response time", []string{"model"}, nil)
```

### Step 10: Update Coolify compose files

In `coolify/staging.yml` and `coolify/production.yml`:

1. Remove backend services: `seq`, `prometheus`, `grafana` (moved to bravyr-obs stack)
2. Keep exporters: `postgres-exporter`, `redis-exporter`, `node-exporter` — these
   stay alongside their parent services. Add their scrape targets to the bravyr-obs
   `stack/prometheus/prometheus.yml`.
3. Remove volumes for removed services
4. Add env vars to the API service:
   ```yaml
   environment:
     OBS_OTLP_ENDPOINT: otel-collector:4317
     OBS_SAMPLE_RATE: "1.0"
     OBS_METRICS_PREFIX: socialup_api
   ```
5. Reference the external bravyr-obs monitoring stack (deployed separately)

### Step 11: Deploy monitoring stack

Deploy the bravyr-obs monitoring stack to the same Docker network:

```bash
cd bravyr-obs/stack
cp .env.example .env  # configure GRAFANA_ADMIN_PASSWORD
docker compose up -d
```

### Step 12: Verify

1. Logs appear in Grafana → Explore → Loki: `{service="socialup-api"}`
2. Traces appear in Grafana → Explore → Tempo: search by service name
3. Metrics appear in Grafana → HTTP Overview dashboard
4. Health check returns `{"status":"healthy","checks":[...]}`
5. Click trace_id in a log line → opens trace in Tempo
6. Click "View Logs" on a trace → opens logs in Loki

## Files Changed in socialup-api

| File | Change |
|---|---|
| `go.mod` | Add `github.com/bravyr/bravyr-obs` |
| `cmd/api/main.go` | Replace zerolog init with obs.Init() |
| `cmd/worker/main.go` | Replace zerolog init with obs.Init() |
| `internal/config/config.go` | Add OTLP/metrics fields, remove Seq fields |
| `internal/server/server.go` | Add obs field, Middleware(), health checker, /metrics |
| `coolify/staging.yml` | Remove monitoring services, add env vars |
| `coolify/production.yml` | Remove monitoring services, add env vars |

## Risk Assessment

- **Low risk**: Logging change (zerolog → obs.Logger wraps zerolog — same API)
- **Low risk**: Middleware addition (additive, existing middleware unchanged)
- **Medium risk**: Health check format change (requires Coolify healthcheck update)
- **Medium risk**: Coolify compose changes (monitoring stack must be deployed first)
