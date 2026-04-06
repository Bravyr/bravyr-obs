# Plan: Issue #1 — [Phase 0] Repo Scaffold

## Context

bravyr-obs is a new Go observability library for internal services. The repo is completely empty (no commits). Phase 0 establishes the project structure, CI, tooling, and stub API surface so subsequent phases can implement features incrementally.

## File Structure

```
bravyr-obs/
├── .github/workflows/ci.yml
├── .golangci.yml
├── .gitignore
├── CHANGELOG.md
├── LICENSE
├── Makefile
├── README.md
├── obs.go                  # Root facade: Init(), Middleware(), HealthHandler()
├── obs_test.go
├── config/
│   └── config.go           # Config struct, Validate(), env tags
├── health/
│   └── health.go           # CheckFunc type, Handler stub
├── log/
│   └── log.go              # Package doc only (impl in Phase 1)
├── metrics/
│   └── metrics.go          # Package doc only (impl in Phase 1)
├── middleware/
│   └── middleware.go        # Package doc only (impl in Phase 1)
├── trace/
│   └── trace.go            # Package doc only (impl in Phase 1)
├── stack/
│   └── README.md           # Placeholder for Docker Compose files
└── docs/plans/
    └── 001-repo-scaffold.md  # This plan
```

## Key Decisions

| Decision | Choice | Why |
|---|---|---|
| Dependencies | Zero at scaffold | Add when sub-packages need real type signatures |
| Root package | Type alias facade | `obs.Config = config.Config`, clean consumer API |
| Health checks | `CheckFunc` function type | No driver coupling, testable with closures |
| HealthHandler params | `map[string]CheckFunc` | Type-safe, named checks from day one |
| Config secrets | Separate `SeqAPIKey` field | Don't embed credentials in URLs |
| CI security | govulncheck + gosec + go mod verify | Low cost at scaffold, high value |
| Makefile swagger | No-op echo | CLAUDE.md requires target to exist |
| Go version | 1.24 only | Solo consumer, expand matrix later |
