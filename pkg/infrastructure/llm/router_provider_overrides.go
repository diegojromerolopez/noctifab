package llm

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/diegojromerolopez/noctifab/pkg/domain"
)

// namedUsageKey is the budget-store key for one named provider entry. The
// prefix keeps it apart from the provider-type key (e.g. "gemini") and from
// the "total" key, so two entries of the same provider type get separate caps.
func namedUsageKey(name string) string {
	return "named:" + name
}

// hedgeDelayFor returns the speculative hedge delay when the candidate is the
// primary. A negative value disables hedging. The provider hedging block wins
// over the global llm.hedging block.
func (r *ResilientLLMRouter) hedgeDelayFor(name string) time.Duration {
	r.mu.RLock()
	base := r.hedgeDelay
	r.mu.RUnlock()

	spec, found := r.namedProviders[name]
	if !found || spec.Hedging == nil {
		return base
	}
	h := spec.Hedging
	if h.Enabled != nil && !*h.Enabled {
		return -1
	}
	if h.Delay > 0 {
		return time.Duration(h.Delay)
	}
	if base < 0 && h.Enabled != nil && *h.Enabled {
		if r.cfg != nil {
			return r.cfg.LLM.Hedging.GetDelay()
		}
		return DefaultHedgeDelay
	}
	return base
}

// heavyHedgeDelayFor returns the minimum hedge delay for heavy batch roles.
func (r *ResilientLLMRouter) heavyHedgeDelayFor(name string) time.Duration {
	if spec, found := r.namedProviders[name]; found && spec.Hedging != nil && spec.Hedging.HeavyDelay > 0 {
		return time.Duration(spec.Hedging.HeavyDelay)
	}
	if r.cfg != nil {
		return r.cfg.LLM.Hedging.GetHeavyDelay()
	}
	return 90 * time.Second
}

// filterByProviderTokenLimit removes candidates whose own daily token cap
// (llm.providers[].token_usage_limit) is used up. Budget-store read errors
// keep the candidate (fail open), so a storage fault never blocks generation.
func (r *ResilientLLMRouter) filterByProviderTokenLimit(ctx context.Context, candidates []RouterCandidate) []RouterCandidate {
	if r.budgetStore == nil || r.cfg == nil {
		return candidates
	}
	today := time.Now().UTC().Format("2006-01-02")
	kept := make([]RouterCandidate, 0, len(candidates))
	for _, c := range candidates {
		spec, found := r.namedProviders[c.Name]
		if !found {
			kept = append(kept, c)
			continue
		}
		limit := r.cfg.LLM.ResolveTokenUsageLimit(spec)
		if limit <= 0 {
			kept = append(kept, c)
			continue
		}
		used, err := r.budgetStore.GetDailyUsage(ctx, today, namedUsageKey(c.Name))
		if err != nil || used < limit {
			kept = append(kept, c)
			continue
		}
		fmt.Fprintf(os.Stderr, "⚠️ [LLM Budget] Candidate '%s' skipped: daily token limit %d reached (%d used)\n", c.Name, limit, used)
	}
	return kept
}

// incrementUsage records tokens under the provider type, the named entry, and the total.
func (r *ResilientLLMRouter) incrementUsage(ctx context.Context, c RouterCandidate, tokens int64) {
	if r.budgetStore == nil {
		return
	}
	today := time.Now().UTC().Format("2006-01-02")
	_ = r.budgetStore.IncrementUsage(ctx, today, c.Provider, tokens)
	if c.Name != "" {
		_ = r.budgetStore.IncrementUsage(ctx, today, namedUsageKey(c.Name), tokens)
	}
	_ = r.budgetStore.IncrementUsage(ctx, today, "total", tokens)
}

func (r *ResilientLLMRouter) recordTokenUsage(ctx context.Context, prompt string, c RouterCandidate, resp *domain.LLMResponse) {
	if resp == nil {
		return
	}
	r.incrementUsage(ctx, c, estimateUsageTokens(prompt, resp))
}

func (r *ResilientLLMRouter) recordPromptTokenUsage(ctx context.Context, prompt string, c RouterCandidate) {
	r.incrementUsage(ctx, c, estimatePromptTokens(prompt))
}
