package health

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"golang.org/x/sync/errgroup"
)

// Holds the outcome of a single health check
type result struct {
	Name string `json:"name"`
	Status string `json:"status"`
	Error string `json:"error,omitempty"`
	Duration float64 `json:"duration"`
}

// Runs multiple health checkers and exposes an HTTP endpoint
type Handler struct {
	checks []Checker
	timeout time.Duration
}

// Creates a handler with a default 2 sceond timeout

func NewHandler(checks ...Checker) *Handler {
	return &Handler{
		checks: checks,
		timeout: 2 * time.Second,
	}
}

// Implement http hander

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request)  {
	ctx, cancel := context.WithTimeout(r.Context(), h.timeout)	
	defer cancel()

	results := make([]result, len(h.checks))
	g, ctx := errgroup.WithContext(ctx)

	for i, check := range h.checks {
		i := i       // capture loop variables
		check := check
		g.Go(func() error {
			start := time.Now()
			err := check.Check(ctx)
			elapsed := time.Since(start)
			ms := float64(elapsed.Nanoseconds()) / 1e6
			res := result{
				Name:     check.Name(),
				Duration: ms,
			}
			if err != nil {
				res.Status = "unhealthy"
				res.Error = err.Error()
			} else {
				res.Status = "healthy"
			}
			results[i] = res
			return nil // we never cancel the group; all checks should complete
		})
	}

	_ = g.Wait() // errors are captured in individual results, not the group

	// Determine overall status.
	overall := "healthy"
	for _, r := range results {
		if r.Status == "unhealthy" {
			overall = "unhealthy"
			break
		}
	}

	// Build response.
	response := struct {
		Status string   `json:"status"`
		Checks []result `json:"checks"`
	}{
		Status: overall,
		Checks: results,
	}

	// Write status code and JSON.
	w.Header().Set("Content-Type", "application/json")
	if overall == "fail" {
		w.WriteHeader(http.StatusServiceUnavailable)
	} else {
		w.WriteHeader(http.StatusOK)
	}
	json.NewEncoder(w).Encode(response)
}
