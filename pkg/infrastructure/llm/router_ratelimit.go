package llm

import (
	"fmt"
	"os"
	"time"
)

// handleRateLimitRotation updates router cooldowns for a candidate,
// extracting retryDelay if provided by the gateway or defaulting to a short rotation window.
func (r *ResilientLLMRouter) handleRateLimitRotation(c RouterCandidate, err error) {
	cooldown := 60 * time.Second
	if retryAfter, ok := parseRetryDelay(err); ok && retryAfter > 0 {
		cooldown = retryAfter
	}
	r.mu.Lock()
	until := time.Now().Add(cooldown)
	r.cooldowns[c.Name] = until
	r.mu.Unlock()
	fmt.Fprintf(os.Stderr, "⚡ [Rate Limit Tier Rotation] Candidate %s (provider %s) hit 429 rate limit. Placed on %v cooldown. Rotating to next candidate in tier...\n", c.Name, c.Provider, cooldown)
}

// isCandidateInCooldown checks whether the candidate is currently on cooldown.
func (r *ResilientLLMRouter) isCandidateInCooldown(c RouterCandidate) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	now := time.Now()
	if until, inCooldown := r.cooldowns[c.Name]; inCooldown && now.Before(until) {
		return true
	}
	return false
}

// getShortestCooldownCandidate finds the candidate whose cooldown expires earliest,
// allowing graceful wait rather than immediate failure when all providers are temporarily throttled.
func (r *ResilientLLMRouter) getShortestCooldownCandidate(candidates []RouterCandidate) (RouterCandidate, time.Duration) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var best RouterCandidate
	var shortest time.Duration = -1
	now := time.Now()

	for _, c := range candidates {
		var until time.Time
		if u, ok := r.cooldowns[c.Name]; ok {
			until = u
		}
		if until.IsZero() {
			return c, 0
		}
		rem := until.Sub(now)
		if rem <= 0 {
			return c, 0
		}
		if shortest < 0 || rem < shortest {
			shortest = rem
			best = c
		}
	}
	return best, shortest
}
