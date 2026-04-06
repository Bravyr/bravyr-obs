// Package health provides lightweight health check primitives for HTTP services.
//
// The health endpoint should be protected by authentication middleware or
// restricted to an internal network when exposed in production, as check names
// and timing data reveal infrastructure topology.
package health

import (
	"context"
	"encoding/json"
	"net/http"
	"sync"
	"time"
)

// CheckFunc performs a single health check and returns an error if unhealthy.
type CheckFunc func(ctx context.Context) error

// Result represents the outcome of a single health check.
type Result struct {
	Name     string `json:"name"`
	Healthy  bool   `json:"healthy"`
	Duration string `json:"duration"`
	Error    string `json:"error,omitempty"`
}

// Response is the JSON body returned by Handler.
type Response struct {
	Status string   `json:"status"`
	Checks []Result `json:"checks"`
}

// Option configures a Checker.
type Option func(*Checker)

// WithTimeout sets the global check timeout (default 5s).
func WithTimeout(d time.Duration) Option {
	return func(c *Checker) { c.timeout = d }
}

// CheckOption configures an individual check registration.
type CheckOption func(*checkEntry)

// WithCheckTimeout sets a per-check deadline. The effective timeout for a check
// is min(global timeout, per-check timeout) — a per-check timeout cannot exceed
// the global Checker timeout.
func WithCheckTimeout(d time.Duration) CheckOption {
	return func(e *checkEntry) { e.timeout = d }
}

// checkEntry holds a CheckFunc alongside its optional per-check timeout.
type checkEntry struct {
	fn      CheckFunc
	timeout time.Duration // 0 means use Checker.timeout
}

// Checker accumulates named health checks and exposes them as an HTTP handler.
// All checks must be registered via AddCheck before calling Handler — the
// Checker is not safe for concurrent registration and serving.
type Checker struct {
	checks  map[string]checkEntry
	timeout time.Duration
}

// New creates a Checker with a default 5-second global timeout. Pass Option
// values to override defaults.
func New(opts ...Option) *Checker {
	c := &Checker{
		checks:  make(map[string]checkEntry),
		timeout: 5 * time.Second,
	}
	for _, opt := range opts {
		opt(c)
	}
	return c
}

// AddCheck registers a named health check. Must be called before Handler.
func (c *Checker) AddCheck(name string, fn CheckFunc, opts ...CheckOption) {
	entry := checkEntry{fn: fn}
	for _, opt := range opts {
		opt(&entry)
	}
	c.checks[name] = entry
}

// Handler returns an http.HandlerFunc that runs all registered checks
// concurrently and responds with a JSON health report.
func (c *Checker) Handler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			w.Header().Set("Allow", "GET, HEAD")
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		ctx, cancel := context.WithTimeout(r.Context(), c.timeout)
		defer cancel()

		results := make([]Result, 0, len(c.checks))
		var mu sync.Mutex
		var wg sync.WaitGroup

		for name, entry := range c.checks {
			wg.Add(1)
			go func(name string, entry checkEntry) {
				defer wg.Done()

				checkCtx := ctx
				if entry.timeout > 0 {
					var checkCancel context.CancelFunc
					checkCtx, checkCancel = context.WithTimeout(ctx, entry.timeout)
					defer checkCancel()
				}

				start := time.Now()
				err := entry.fn(checkCtx)
				duration := time.Since(start)

				res := Result{
					Name:     name,
					Healthy:  err == nil,
					Duration: duration.Round(time.Millisecond).String(),
				}
				if err != nil {
					res.Error = "check failed"
				}

				mu.Lock()
				results = append(results, res)
				mu.Unlock()
			}(name, entry)
		}

		wg.Wait()

		status := "healthy"
		for _, res := range results {
			if !res.Healthy {
				status = "unhealthy"
				break
			}
		}

		resp := Response{
			Status: status,
			Checks: results,
		}

		w.Header().Set("Content-Type", "application/json")
		if status != "healthy" {
			w.WriteHeader(http.StatusServiceUnavailable)
		}
		// Header already written; nothing useful to do if encoding fails.
		_ = json.NewEncoder(w).Encode(resp)
	}
}

// Handler returns an http.HandlerFunc that executes all named checks
// concurrently and responds with a JSON health report. This is a convenience
// function; for per-check timeouts and other options, use New and AddCheck.
func Handler(checks map[string]CheckFunc) http.HandlerFunc {
	c := New()
	for name, fn := range checks {
		c.AddCheck(name, fn)
	}
	return c.Handler()
}
