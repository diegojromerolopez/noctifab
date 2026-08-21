package services

import (
	"context"
	"errors"
	"math/rand"
	"net/http"
	"time"

	"github.com/diegojromerolopez/noctifab/pkg/domain"
)

const (
	// occRetryBaseBackoff is the initial sleep between OCC retry attempts in
	// the REST handlers.
	occRetryBaseBackoff = 200 * time.Millisecond
	// occRetryMaxBackoff caps the exponential backoff between OCC retries.
	occRetryMaxBackoff = time.Second
	// occRetryMaxAttempts is the number of Load-mutate-Save attempts.
	occRetryMaxAttempts = 20
)

// saveStateWithBackoff performs a Load-mutate-Save cycle with optimistic
// concurrency retries using exponential backoff (base 200ms, x2 per attempt,
// ±20% jitter, capped at 1s). Non-conflict errors abort immediately.
func saveStateWithBackoff(ctx context.Context, repo domain.StateRepository, mutate func(*domain.State)) error {
	backoff := occRetryBaseBackoff
	var lastErr error
	for attempt := 0; attempt < occRetryMaxAttempts; attempt++ {
		state, err := repo.Load(ctx)
		if err != nil {
			return err
		}
		mutate(state)
		err = repo.Save(ctx, state)
		if err == nil {
			return nil
		}
		if !errors.Is(err, domain.ErrVersionConflict) {
			return err
		}
		lastErr = err

		// ±20% jitter around the current backoff.
		jittered := time.Duration(float64(backoff) * (0.8 + 0.4*rand.Float64()))
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(jittered):
		}
		backoff *= 2
		if backoff > occRetryMaxBackoff {
			backoff = occRetryMaxBackoff
		}
	}
	return lastErr
}

// storyStatusHandler builds a POST handler that transitions the active user
// story to the given status with OCC retry/backoff.
func storyStatusHandler(repo domain.StateRepository, status domain.StoryStatus, response string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		err := saveStateWithBackoff(r.Context(), repo, func(state *domain.State) {
			state.StoryStatus = status
		})
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(response))
	}
}

// The lightweight per-story projection returned by /api/v1/status lives in
// pkg/domain as domain.StateSummary (see domain.SummarizeState), so that
// repositories can compute it directly in SQL without loading full states.
