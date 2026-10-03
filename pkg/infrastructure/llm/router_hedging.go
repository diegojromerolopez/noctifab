package llm

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/diegojromerolopez/noctifab/pkg/domain"
)

// DefaultHedgeDelay is the default duration before launching a speculative fallback request.
const DefaultHedgeDelay = 25 * time.Second

type hedgeResult struct {
	candidate RouterCandidate
	resp      *domain.LLMResponse
	err       error
	duration  time.Duration
}

// completeWithHedging executes an LLM completion using speculative hedging:
// if the primary candidate does not respond within hedgeDelay, a secondary candidate
// is launched in parallel. The first candidate to succeed wins and cancels the other.
func (r *ResilientLLMRouter) completeWithHedging(
	ctx context.Context,
	roleName string,
	candidates []RouterCandidate,
	prompt string,
) (*domain.LLMResponse, error) {
	if len(candidates) < 2 {
		return r.completeSequentially(ctx, roleName, candidates, prompt)
	}
	hedgeDelay := r.hedgeDelayFor(candidates[0].Name)
	if hedgeDelay < 0 {
		return r.completeSequentially(ctx, roleName, candidates, prompt)
	}
	if hedgeDelay == 0 {
		hedgeDelay = DefaultHedgeDelay
	}

	primary := candidates[0]
	secondary := candidates[1]

	// Heavy batch roles (Product Manager, Sovereign Rescue, QA, Spec Drafting)
	// naturally take 40-90s to stream their full JSON payloads. Speculatively hedging at 25s
	// for these roles needlessly doubles token spend. Scale minimum hedge delay to at least 90s.
	cleanRole := strings.ToLower(strings.TrimSpace(roleName))
	isHeavyBatchRole := cleanRole == "product_manager" || cleanRole == "fallback" || cleanRole == "sovereign_rescue" || cleanRole == "qa" || cleanRole == "spec"
	heavyDelay := r.heavyHedgeDelayFor(primary.Name)
	if isHeavyBatchRole && hedgeDelay < heavyDelay {
		hedgeDelay = heavyDelay
	}

	// Scale hedge delay with prompt tokens so large prompts have adequate TTFT before hedging
	promptTokens := estimatePromptTokens(prompt)
	if promptTokens > 4000 {
		extraDelay := time.Duration((promptTokens-4000)/2000) * time.Second
		if extraDelay > 60*time.Second {
			extraDelay = 60 * time.Second
		}
		hedgeDelay += extraDelay
	}

	// Adaptive Speculative Hedging: check dynamic demotion/timeout memory
	if r.latencyTracker != nil {
		penalty := r.latencyTracker.GetPenaltyScore(primary.Name)
		timeouts := r.latencyTracker.GetConsecutiveTimeouts(primary.Name)
		if penalty > 0 || timeouts > 0 {
			origDelay := hedgeDelay
			var targetDelay time.Duration
			if penalty >= 3 || timeouts >= 3 {
				targetDelay = 1 * time.Second
			} else if penalty >= 2 || timeouts >= 2 {
				targetDelay = 3 * time.Second
			} else {
				targetDelay = 10 * time.Second
			}

			// If origDelay is already smaller than targetDelay (e.g. in tests or tuned config),
			// scale down proportionally to avoid inflating small delays.
			if origDelay <= targetDelay {
				factor := time.Duration(penalty * 2)
				if factor < 2 {
					factor = 2
				}
				targetDelay = origDelay / factor
				if targetDelay < 1*time.Millisecond {
					targetDelay = 1 * time.Millisecond
				}
			}

			if targetDelay < hedgeDelay {
				hedgeDelay = targetDelay
				fmt.Fprintf(os.Stderr, "⚡ [LLM Adaptive Hedging] Candidate '%s' has demotion penalty %d (timeouts=%d); dynamically reduced hedge delay from %s to %s\n", primary.Name, penalty, timeouts, origDelay, hedgeDelay)
			}
		}
	}

	parentCtx, parentCancel := context.WithCancel(ctx)
	defer parentCancel()

	tracker := domain.NewStreamLivenessTracker()
	primaryCtx := domain.WithStreamLivenessTracker(parentCtx, tracker)

	ch := make(chan hedgeResult, 2)

	// Launch primary
	t0Primary := time.Now()
	go func() {
		resp, err := primary.Client.Complete(primaryCtx, prompt)
		ch <- hedgeResult{
			candidate: primary,
			resp:      resp,
			err:       err,
			duration:  time.Since(t0Primary),
		}
	}()

	timer := time.NewTimer(hedgeDelay)
	defer timer.Stop()

	var secondaryLaunched bool
	var t0Secondary time.Time
	var failures int

	for {
		select {
		case <-timer.C:
			// Stream liveness check: if primary candidate is actively streaming chunks,
			// do NOT launch a redundant secondary candidate and double token spend!
			if tracker.IsActive(time.Now().Add(-15 * time.Second)) {
				fmt.Fprintf(os.Stderr, "ℹ [LLM Speculative Hedging] Primary candidate '%s' is actively streaming (%d chunks received); postponing speculative hedge.\n", primary.Name, tracker.ChunkCount())
				timer.Reset(10 * time.Second)
				continue
			}

			// Primary exceeded hedge delay without active streaming: launch secondary hedge request
			if !secondaryLaunched {
				secondaryLaunched = true
				t0Secondary = time.Now()
				fmt.Fprintf(os.Stderr, "⚡ [LLM Speculative Hedging] Primary candidate '%s' exceeded %s. Launching concurrent hedge with '%s'...\n", primary.Name, hedgeDelay, secondary.Name)
				go func() {
					resp, err := secondary.Client.Complete(parentCtx, prompt)
					ch <- hedgeResult{
						candidate: secondary,
						resp:      resp,
						err:       err,
						duration:  time.Since(t0Secondary),
					}
				}()
			}

		case res := <-ch:
			r.recordCandidateOutcome(res.candidate, res.duration, res.err)

			if res.err == nil && res.resp != nil {
				// We have a winner! Cancel the other candidate's context
				parentCancel()
				r.recordTokenUsage(ctx, prompt, res.candidate, res.resp)
				if secondaryLaunched {
					// Account for secondary hedged input tokens that were dispatched
					otherCandidate := secondary
					if res.candidate.Name == secondary.Name {
						otherCandidate = primary
					}
					r.recordPromptTokenUsage(ctx, prompt, otherCandidate)
				}
				return res.resp, nil
			}

			// If primary failed before timer fired, launch secondary immediately without waiting
			failures++
			if res.candidate.Name == primary.Name && !secondaryLaunched {
				timer.Stop()
				secondaryLaunched = true
				t0Secondary = time.Now()
				go func() {
					resp, err := secondary.Client.Complete(parentCtx, prompt)
					ch <- hedgeResult{
						candidate: secondary,
						resp:      resp,
						err:       err,
						duration:  time.Since(t0Secondary),
					}
				}()
			}

			if failures >= 2 || (!secondaryLaunched && failures >= 1) {
				// Both primary and secondary failed (or primary failed and no secondary could be launched)
				// Fall back to sequential execution on remaining candidates
				if len(candidates) > 2 {
					return r.completeSequentially(ctx, roleName, candidates[2:], prompt)
				}
				return nil, fmt.Errorf("both primary '%s' and secondary '%s' failed in speculative hedge", primary.Name, secondary.Name)
			}

		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
}

func (r *ResilientLLMRouter) recordCandidateOutcome(c RouterCandidate, dur time.Duration, err error) {
	if r.latencyTracker != nil {
		var candTimeout time.Duration
		if spec, found := r.namedProviders[c.Name]; found && spec.MaxTimeout > 0 {
			candTimeout = time.Duration(spec.MaxTimeout)
		}
		r.latencyTracker.RecordOutcome(c.Name, dur, err, candTimeout)
	}

	if isEvictionError(err) {
		r.mu.Lock()
		r.evictedUntil[c.Name] = time.Now().Add(30 * time.Minute)
		r.evictionReasons[c.Name] = err.Error()
		r.mu.Unlock()
		fmt.Fprintf(os.Stderr, "⚠️ [LLM Provider Evicted] Candidate '%s' (provider '%s') EVICTED for 30 minutes due to depleted credits / auth failure: %v\n", c.Name, c.Provider, err)
	} else if isRateLimitOrQuota(err) {
		r.handleRateLimitRotation(c, err)
	} else if isTransientError(err) {
		r.mu.Lock()
		r.cooldowns[c.Name] = time.Now().Add(r.cooldownDuration)
		r.mu.Unlock()
	}
}
