package llm

import (
	"fmt"
	"os"
	"sort"
)

// FilterAndPrioritizeCandidatesByContext inspects the candidate list against the estimated
// prompt token size. It bypasses candidates whose context window is too small, and when
// adaptive is true, re-prioritizes high-capacity models for large prompts.
func FilterAndPrioritizeCandidatesByContext(candidates []RouterCandidate, promptTokens int64, adaptive bool) []RouterCandidate {
	if len(candidates) <= 1 || promptTokens <= 0 {
		return candidates
	}

	// 1. Ensure every candidate has an effective ContextWindow
	for i := range candidates {
		if candidates[i].ContextWindow <= 0 {
			candidates[i].ContextWindow = GetModelContextWindow(candidates[i].Provider, candidates[i].Model)
		}
	}

	// 2. Filter out candidates whose context window cannot fit the prompt (plus 512 token margin)
	var fitting []RouterCandidate
	for _, c := range candidates {
		if c.ContextWindow > 0 && promptTokens+512 > c.ContextWindow {
			fmt.Fprintf(os.Stderr, "ℹ [LLM Router] Candidate '%s' (context window %d) too small for prompt (%d tokens); bypassing.\n", c.Name, c.ContextWindow, promptTokens)
			continue
		}
		fitting = append(fitting, c)
	}

	// Safety: if all candidates were filtered out, preserve original list
	if len(fitting) == 0 {
		return candidates
	}

	// If adaptive routing is disabled, return fitting candidates in original priority order
	if !adaptive {
		return fitting
	}

	// 3. Adaptive Re-ordering:
	// For large prompts (> 40,000 tokens), prioritize ultra-high capacity models (>= 200k, then >= 128k)
	// over smaller capacity models, bypassing the static priority configuration.
	if promptTokens > 40_000 {
		type scoredCandidate struct {
			cand  RouterCandidate
			score int
			idx   int
		}
		scored := make([]scoredCandidate, len(fitting))
		for i, c := range fitting {
			score := 0
			switch {
			case c.ContextWindow >= ContextWindowGemini:
				score = 300 // 1M+ tokens
			case c.ContextWindow >= ContextWindowClaude:
				score = 200 // 200k tokens
			case c.ContextWindow >= ContextWindowOpenAI:
				score = 100 // 128k tokens
			default:
				score = 10
			}
			scored[i] = scoredCandidate{cand: c, score: score, idx: i}
		}

		sort.SliceStable(scored, func(i, j int) bool {
			if scored[i].score != scored[j].score {
				return scored[i].score > scored[j].score
			}
			return scored[i].idx < scored[j].idx
		})

		reordered := make([]RouterCandidate, len(scored))
		for i, s := range scored {
			reordered[i] = s.cand
		}
		return reordered
	}

	return fitting
}
