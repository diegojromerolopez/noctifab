package llm

import (
	"context"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/diegojromerolopez/noctifab/pkg/domain"
	"github.com/diegojromerolopez/noctifab/pkg/infrastructure/config"
)

// RouterCandidate represents a candidate client with its provider name, model, and client implementation.
type RouterCandidate struct {
	Name          string
	Provider      string
	Model         string
	Client        domain.LLMClient
	ContextWindow int64
}

// ResilientLLMRouter manages multi-provider per-agent routing, dynamic model fallbacks, and global failovers.
type ResilientLLMRouter struct {
	mu               sync.RWMutex
	cfg              *config.Config
	namedProviders   map[string]config.ProviderSpec
	globalPriority   []string
	roles            config.RolesConfig
	defaultClient    domain.LLMClient
	budgetStore      domain.BudgetStore
	tokenUsageLimit  int64
	cooldowns        map[string]time.Time
	cooldownDuration time.Duration
	candidateCache   map[string][]RouterCandidate
	evictedUntil     map[string]time.Time
	evictionReasons  map[string]string
	latencyTracker   *LatencyTracker
	hedgeDelay       time.Duration
}

// SetHedgeDelay sets the speculative hedging delay. A negative value disables hedging.
func (r *ResilientLLMRouter) SetHedgeDelay(d time.Duration) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.hedgeDelay = d
}

// NewResilientLLMRouter constructs a new ResilientLLMRouter.
func NewResilientLLMRouter(cfg *config.Config, budgetStore domain.BudgetStore) *ResilientLLMRouter {
	named := make(map[string]config.ProviderSpec)
	if cfg != nil && cfg.LLM.Providers != nil {
		for _, p := range cfg.LLM.Providers {
			if p.Name != "" {
				named[p.Name] = p
			}
		}
	}

	var globalPrio []string
	if cfg != nil {
		globalPrio = cfg.LLM.Priority
	}

	var roles config.RolesConfig
	if cfg != nil {
		roles = cfg.Roles
	}

	cooldown := 5 * time.Minute
	if cfg != nil && cfg.LLM.Failover.Cooldown > 0 {
		cooldown = time.Duration(cfg.LLM.Failover.Cooldown)
	}

	var defaultClient domain.LLMClient
	if cfg != nil {
		defaultClient = NewClient(
			cfg.LLM.Provider, cfg.LLM.Model, cfg.LLM.APIKeyValue,
			cfg.LLM.MaxRetries, time.Duration(cfg.LLM.RetryBackoff), cfg.LLM.URL,
		)
		if dc, ok := defaultClient.(*Client); ok {
			dc.APIKeys = cfg.LLM.APIKeyPool
			dc.SkipOnCreditExhausted = cfg.LLM.SkipOnCreditExhausted
		}
	}

	var tokenLimit int64
	var hedgeDelay time.Duration
	if cfg != nil {
		tokenLimit = cfg.LLM.TokenUsageLimit
		if !cfg.LLM.Hedging.IsEnabled() {
			hedgeDelay = -1
		} else {
			hedgeDelay = cfg.LLM.Hedging.GetDelay()
		}
	}

	return &ResilientLLMRouter{
		cfg:              cfg,
		namedProviders:   named,
		globalPriority:   globalPrio,
		roles:            roles,
		defaultClient:    defaultClient,
		budgetStore:      budgetStore,
		tokenUsageLimit:  tokenLimit,
		hedgeDelay:       hedgeDelay,
		cooldowns:        make(map[string]time.Time),
		cooldownDuration: cooldown,
		candidateCache:   make(map[string][]RouterCandidate),
		evictedUntil:     make(map[string]time.Time),
		evictionReasons:  make(map[string]string),
		latencyTracker:   NewLatencyTracker(),
	}
}

// ResolveCandidatesForRole returns the ordered candidate client list for a
// given role. Results are memoized per role: the candidate list is derived
// solely from static configuration and environment variables, so rebuilding
// clients (and re-scanning os.Getenv) on every completion is wasted work.
func (r *ResilientLLMRouter) ResolveCandidatesForRole(roleName string) []RouterCandidate {
	if config.IsRemovedAgentRole(roleName) {
		return nil
	}
	r.mu.RLock()
	if cached, ok := r.candidateCache[roleName]; ok {
		r.mu.RUnlock()
		return cached
	}
	r.mu.RUnlock()

	candidates := r.buildCandidatesForRole(roleName)

	r.mu.Lock()
	if r.candidateCache == nil {
		r.candidateCache = make(map[string][]RouterCandidate)
	}
	r.candidateCache[roleName] = candidates
	r.mu.Unlock()

	return candidates
}

// InvalidateCandidateCache clears the memoized per-role candidate lists,
// forcing the next resolution to rebuild clients from config/env.
func (r *ResilientLLMRouter) InvalidateCandidateCache() {
	r.mu.Lock()
	r.candidateCache = make(map[string][]RouterCandidate)
	r.mu.Unlock()
}

// buildCandidatesForRole constructs the ordered candidate client list for a
// given role from configuration.
func (r *ResilientLLMRouter) buildCandidatesForRole(roleName string) []RouterCandidate {
	var candidates []RouterCandidate
	seen := make(map[string]bool)

	roleSetting := r.getRoleSetting(roleName)

	// 0. If ensemble strategy is configured for this role, build ensemble candidate as top priority
	if roleSetting.Ensemble.IsEnabled() {
		ensCand := r.buildEnsembleCandidate(roleName, roleSetting.Ensemble)
		if ensCand != nil {
			candidates = append(candidates, *ensCand)
		}
	}

	// 1. Process role-specific providers if configured
	if len(roleSetting.Providers) > 0 {
		for _, ref := range roleSetting.Providers {
			var spec config.ProviderSpec
			var found bool

			if ref.Name != "" {
				spec, found = r.namedProviders[ref.Name]
				if !found {
					// Bad provider reference in role config -> skip/ignore silently
					continue
				}
			} else if ref.Provider != "" {
				// Inline provider shorthand
				spec = config.ProviderSpec{
					Name:     ref.Provider,
					Provider: ref.Provider,
					APIKeys:  config.APIKeys{strings.ToUpper(ref.Provider) + "_API_KEY"},
				}
				spec.APIKeyValue = os.Getenv(spec.APIKeys[0])
				found = true
			}

			if !found || spec.Provider == "" {
				continue
			}

			// Determine model list for this provider entry
			var modelsToTry []string
			if len(ref.Models) > 0 {
				modelsToTry = ref.Models
			} else if ref.Model != "" {
				modelsToTry = []string{ref.Model}
			} else if spec.Model != "" {
				modelsToTry = []string{spec.Model}
			} else {
				// Version-agnostic -> empty string allows Client capacity fallback
				modelsToTry = []string{""}
			}

			for _, m := range modelsToTry {
				key := spec.Provider + ":" + m
				if seen[key] {
					continue
				}
				seen[key] = true

				overrideSpec := spec
				if ref.Temperature != nil {
					overrideSpec.Temperature = *ref.Temperature
				}
				if ref.MaxTokens != nil {
					overrideSpec.MaxTokens = *ref.MaxTokens
				}
				if th := ref.GetEnableThinking(); th != nil {
					overrideSpec.EnableThinking = th
				} else if roleSetting.Thinking != nil && roleSetting.Thinking.Enabled != nil {
					overrideSpec.EnableThinking = roleSetting.Thinking.Enabled
				}
				if tb := ref.GetThinkingBudget(); tb != nil {
					overrideSpec.ThinkingBudget = tb
				} else if roleSetting.Thinking != nil && roleSetting.Thinking.Budget != nil {
					overrideSpec.ThinkingBudget = roleSetting.Thinking.Budget
				}

				client := r.buildClientForSpec(overrideSpec, m)
				if client != nil {
					if ref.Temperature != nil {
						if c, ok := client.(*Client); ok {
							c.Temperature = *ref.Temperature
						}
					}
					if roleSetting.Timeout > 0 {
						if c, ok := client.(*Client); ok {
							c.Timeout = time.Duration(roleSetting.Timeout)
						}
					} else if roleName == "product_manager" || roleName == "productmanager" {
						if c, ok := client.(*Client); ok {
							c.Timeout = 180 * time.Second
						}
					}
					cw := overrideSpec.ContextWindow
					if cw <= 0 {
						cw = GetModelContextWindow(overrideSpec.Provider, m)
					}
					candidates = append(candidates, RouterCandidate{
						Name:          overrideSpec.Name,
						Provider:      overrideSpec.Provider,
						Model:         m,
						Client:        client,
						ContextWindow: cw,
					})
				}
			}
		}
	}

	// 2. Append global llm.priority candidates (for unconfigured roles or as fallthrough)
	for _, pName := range r.globalPriority {
		spec, found := r.namedProviders[pName]
		if !found {
			// Check if pName is a raw provider name (e.g., "openai", "anthropic")
			if pName != "" {
				spec = config.ProviderSpec{
					Name:     pName,
					Provider: pName,
					APIKeys:  config.APIKeys{strings.ToUpper(pName) + "_API_KEY"},
				}
				spec.APIKeyValue = os.Getenv(spec.APIKeys[0])
				found = true
			}
		}

		if !found || spec.Provider == "" {
			continue
		}

		m := spec.Model
		key := spec.Provider + ":" + m
		if seen[key] {
			continue
		}
		seen[key] = true

		client := r.buildClientForSpec(spec, m)
		if client != nil {
			if roleSetting.Timeout > 0 {
				if c, ok := client.(*Client); ok {
					c.Timeout = time.Duration(roleSetting.Timeout)
				}
			} else if roleName == "product_manager" || roleName == "productmanager" {
				if c, ok := client.(*Client); ok {
					c.Timeout = 180 * time.Second
				}
			}
			cw := spec.ContextWindow
			if cw <= 0 {
				cw = GetModelContextWindow(spec.Provider, m)
			}
			candidates = append(candidates, RouterCandidate{
				Name:          spec.Name,
				Provider:      spec.Provider,
				Model:         m,
				Client:        client,
				ContextWindow: cw,
			})
		}
	}

	// 3. Ultimate fallback to defaultClient if no candidates resolved
	if len(candidates) == 0 && r.defaultClient != nil {
		candidates = append(candidates, RouterCandidate{
			Name:     "default",
			Provider: r.cfg.LLM.Provider,
			Model:    r.cfg.LLM.Model,
			Client:   r.defaultClient,
		})
	}

	return candidates
}

// Complete executes an LLM completion using role-aware multi-provider routing and fallbacks.
func (r *ResilientLLMRouter) Complete(ctx context.Context, prompt string) (*domain.LLMResponse, error) {
	// Check token usage limit before execution
	if r.budgetStore != nil && r.tokenUsageLimit > 0 {
		today := time.Now().UTC().Format("2006-01-02")
		used, err := r.budgetStore.GetDailyUsage(ctx, today, "total")
		if err == nil && used >= r.tokenUsageLimit {
			return nil, fmt.Errorf("%w: daily token limit of %d reached (%d used)", domain.ErrBudgetExhausted, r.tokenUsageLimit, used)
		}
	}

	roleName := GetRoleFromContext(ctx)
	candidates := r.ResolveCandidatesForRole(roleName)

	if len(candidates) == 0 {
		return nil, fmt.Errorf("no valid LLM candidates available for role '%s'", roleName)
	}

	promptTokens := estimatePromptTokens(prompt)
	adaptive := r.cfg != nil && r.cfg.LLM.IsAdaptiveContextRoutingEnabled()
	candidates = FilterAndPrioritizeCandidatesByContext(candidates, promptTokens, adaptive)
	if r.latencyTracker != nil {
		candidates = r.latencyTracker.ApplyDynamicDemotion(candidates)
	}
	candidates = r.filterByProviderTokenLimit(ctx, candidates)
	if len(candidates) == 0 {
		return nil, fmt.Errorf("%w: every provider for role '%s' reached its daily token limit", domain.ErrBudgetExhausted, roleName)
	}

	if len(candidates) >= 2 {
		eligible := r.filterEligibleCandidates(candidates)
		if len(eligible) >= 2 && r.hedgeDelayFor(eligible[0].Name) >= 0 {
			return r.completeWithHedging(ctx, roleName, eligible, prompt)
		}
	}

	return r.completeSequentially(ctx, roleName, candidates, prompt)
}

func (r *ResilientLLMRouter) filterEligibleCandidates(candidates []RouterCandidate) []RouterCandidate {
	var eligible []RouterCandidate
	now := time.Now()
	r.mu.RLock()
	defer r.mu.RUnlock()
	for _, c := range candidates {
		if evictedUntil, isEvicted := r.evictedUntil[c.Name]; isEvicted && now.Before(evictedUntil) {
			continue
		}
		if until, inCooldown := r.cooldowns[c.Name]; inCooldown && now.Before(until) {
			continue
		}
		eligible = append(eligible, c)
	}
	return eligible
}

func (r *ResilientLLMRouter) completeSequentially(
	ctx context.Context,
	roleName string,
	candidates []RouterCandidate,
	prompt string,
) (*domain.LLMResponse, error) {
	var lastErr error
	for _, c := range candidates {
		// Check eviction & cooldown
		r.mu.RLock()
		evictedUntil, isEvicted := r.evictedUntil[c.Name]
		r.mu.RUnlock()

		if isEvicted && time.Now().Before(evictedUntil) {
			continue
		}

		if r.isCandidateInCooldown(c) {
			continue
		}

		t0 := time.Now()
		resp, err := c.Client.Complete(ctx, prompt)
		callDur := time.Since(t0)
		if r.latencyTracker != nil {
			var candTimeout time.Duration
			if spec, found := r.namedProviders[c.Name]; found && spec.MaxTimeout > 0 {
				candTimeout = time.Duration(spec.MaxTimeout)
			}
			r.latencyTracker.RecordOutcome(c.Name, callDur, err, candTimeout)
		}
		if err == nil {
			// Same prompt+completion estimate as FailoverClient, so a daily
			// "token" limit means the same thing for every client type.
			r.recordTokenUsage(ctx, prompt, c, resp)
			return resp, nil
		}

		lastErr = err
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

	if lastErr != nil {
		return nil, fmt.Errorf("all LLM provider candidates for role '%s' failed: %w", roleName, lastErr)
	}

	// Emergency recovery: If all candidates were skipped due to active cooldown or eviction,
	// do not deadlock the orchestrator. Attempt the candidate with the earliest cooldown expiry.
	if len(candidates) > 0 {
		var fallbackCandidate *RouterCandidate
		var earliestTime time.Time
		r.mu.RLock()
		for _, c := range candidates {
			until := r.cooldowns[c.Name]
			if evictedUntil := r.evictedUntil[c.Name]; evictedUntil.After(until) {
				until = evictedUntil
			}
			if fallbackCandidate == nil || until.Before(earliestTime) {
				candidateCopy := c
				fallbackCandidate = &candidateCopy
				earliestTime = until
			}
		}
		r.mu.RUnlock()

		if fallbackCandidate != nil {
			fmt.Fprintf(os.Stderr, "⚠️ [LLM Router Recovery] All candidates for role '%s' in cooldown/eviction. Attempting emergency recovery with '%s' (%s)...\n", roleName, fallbackCandidate.Name, fallbackCandidate.Provider)
			resp, err := fallbackCandidate.Client.Complete(ctx, prompt)
			if err == nil {
				r.mu.Lock()
				delete(r.cooldowns, fallbackCandidate.Name)
				delete(r.evictedUntil, fallbackCandidate.Name)
				r.mu.Unlock()
				return resp, nil
			}
			return nil, fmt.Errorf("all LLM provider candidates for role '%s' in cooldown/eviction (emergency recovery on '%s' failed: %w)", roleName, fallbackCandidate.Name, err)
		}
	}

	return nil, fmt.Errorf("all LLM provider candidates for role '%s' are currently in cooldown or evicted", roleName)
}
