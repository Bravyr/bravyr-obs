// Package health provides lightweight health check primitives for HTTP services.
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
	Name    string `json:"name"`
	Healthy bool   `json:"healthy"`
	Duration string `json:"duration"`
	Error   string `json:"error,omitempty"`
}

// Response is the JSON body returned by Handler.
type Response struct {
	Status string   `json:"status"`
	Checks []Result `json:"checks"`
}

// Handler returns an http.HandlerFunc that executes all named checks
// concurrently and responds with a JSON health report. Checks are bounded
// by a 5-second timeout to prevent goroutine leaks from hung dependencies.
func Handler(checks map[string]CheckFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()

		results := make([]Result, 0, len(checks))
		var mu sync.Mutex
		var wg sync.WaitGroup

		for name, check := range checks {
			wg.Add(1)
			go func(name string, check CheckFunc) {
				defer wg.Done()

				start := time.Now()
				err := check(ctx)
				duration := time.Since(start)

				res := Result{
					Name:    name,
					Healthy: err == nil,
					Duration: duration.Round(time.Millisecond).String(),
				}
				if err != nil {
					res.Error = "check failed"
				}

				mu.Lock()
				results = append(results, res)
				mu.Unlock()
			}(name, check)
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
