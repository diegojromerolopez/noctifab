package llm

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/diegojromerolopez/noctifab/pkg/domain"
	"github.com/diegojromerolopez/noctifab/pkg/infrastructure/config"
)

// RoleContextKey is the context key for passing the active agent role.
type RoleContextKey struct{}

// WithRoleContext attaches an agent role name to the context.
func WithRoleContext(ctx context.Context, role string) context.Context {
	return context.WithValue(ctx, RoleContextKey{}, role)
}

type stringKey string

const (
	roleStringKey      stringKey = "role"
	agentRoleStringKey stringKey = "agent_role"
)

// GetRoleFromContext retrieves the active agent role name from context.
func GetRoleFromContext(ctx context.Context) string {
	if roleVal := ctx.Value(RoleContextKey{}); roleVal != nil {
		if roleStr, ok := roleVal.(string); ok && roleStr != "" {
			return strings.ToLower(roleStr)
		}
	}
	if roleVal := ctx.Value(roleStringKey); roleVal != nil {
		if roleStr, ok := roleVal.(string); ok && roleStr != "" {
			return strings.ToLower(roleStr)
		}
	}
	if roleVal := ctx.Value(agentRoleStringKey); roleVal != nil {
		if roleStr, ok := roleVal.(string); ok && roleStr != "" {
			return strings.ToLower(roleStr)
		}
	}
	if roleVal := ctx.Value("role"); roleVal != nil {
		if roleStr, ok := roleVal.(string); ok && roleStr != "" {
			return strings.ToLower(roleStr)
		}
	}
	if roleVal := ctx.Value("agent_role"); roleVal != nil {
		if roleStr, ok := roleVal.(string); ok && roleStr != "" {
			return strings.ToLower(roleStr)
		}
	}
	return ""
}

// RouterCandidate represents a candidate client with its provider name, model, and client implementation.
type RouterCandidate struct {
	Name     string
	Provider string
	Model    string
	Client   domain.LLMClient
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
	if cfg != nil {
		tokenLimit = cfg.TokenUsageLimit
	}

	return &ResilientLLMRouter{
		cfg:              cfg,
		namedProviders:   named,
		globalPriority:   globalPrio,
		roles:            roles,
		defaultClient:    defaultClient,
		budgetStore:      budgetStore,
		tokenUsageLimit:  tokenLimit,
		cooldowns:        make(map[string]time.Time),
		cooldownDuration: cooldown,
		candidateCache:   make(map[string][]RouterCandidate),
		evictedUntil:     make(map[string]time.Time),
		evictionReasons:  make(map[string]string),
	}
}

// ResolveCandidatesForRole returns the ordered candidate client list for a
// given role. Results are memoized per role: the candidate list is derived
// solely from static configuration and environment variables, so rebuilding
// clients (and re-scanning os.Getenv) on every completion is wasted work.
func (r *ResilientLLMRouter) ResolveCandidatesForRole(roleName string) []RouterCandidate {
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
				if ref.EnableThinking != nil {
					overrideSpec.EnableThinking = ref.EnableThinking
				}
				if ref.ThinkingBudget != nil {
					overrideSpec.ThinkingBudget = ref.ThinkingBudget
				}

				client := r.buildClientForSpec(overrideSpec, m)
				if client != nil {
					candidates = append(candidates, RouterCandidate{
						Name:     overrideSpec.Name,
						Provider: overrideSpec.Provider,
						Model:    m,
						Client:   client,
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
			candidates = append(candidates, RouterCandidate{
				Name:     spec.Name,
				Provider: spec.Provider,
				Model:    m,
				Client:   client,
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

func (r *ResilientLLMRouter) getRoleSetting(roleName string) config.RoleSetting {
	if r.cfg != nil {
		var agentRole config.AgentRoleConfig
		switch strings.ToLower(roleName) {
		case "orchestrator":
			agentRole = r.cfg.Agents.Orchestrator
		case "product_manager", "productmanager":
			agentRole = r.cfg.Agents.ProductManager
		case "planner":
			agentRole = r.cfg.Agents.Planner
		case "architect":
			agentRole = r.cfg.Agents.Architect
		case "generator", "generators":
			agentRole = r.cfg.Agents.Generators
		case "tester", "testers":
			agentRole = r.cfg.Agents.Testers
		case "qa":
			agentRole = r.cfg.Agents.QA
		case "security":
			agentRole = r.cfg.Agents.Security
		case "performance":
			agentRole = r.cfg.Agents.Performance
		case "docs":
			agentRole = r.cfg.Agents.Docs
		case "devops":
			agentRole = r.cfg.Agents.DevOps
		case "unblocker":
			agentRole = r.cfg.Agents.Unblocker
		}
		if len(agentRole.Providers) > 0 {
			return config.RoleSetting{
				Model:       agentRole.Model,
				Temperature: agentRole.Temperature,
				Profile:     agentRole.Profile,
				Providers:   agentRole.Providers,
			}
		}
	}

	switch strings.ToLower(roleName) {
	case "orchestrator":
		return r.roles.Orchestrator
	case "product_manager", "productmanager":
		return config.RoleSetting{}
	case "planner":
		return r.roles.Planner
	case "architect":
		return r.roles.Architect
	case "generator", "generators":
		return r.roles.Generator
	case "tester", "testers":
		return r.roles.Tester
	case "qa":
		return r.roles.QA
	case "security":
		return r.roles.Security
	case "performance":
		return r.roles.Performance
	case "docs":
		return r.roles.Docs
	case "devops":
		return r.roles.DevOps
	case "unblocker":
		return r.roles.Unblocker
	default:
		return config.RoleSetting{}
	}
}

func (r *ResilientLLMRouter) buildClientForSpec(spec config.ProviderSpec, modelOverride string) domain.LLMClient {
	if spec.Provider == "" {
		return nil
	}

	model := spec.Model
	if modelOverride != "" {
		model = modelOverride
	}

	apiKey := spec.APIKeyValue
	if apiKey == "" && len(spec.APIKeys) > 0 {
		apiKey = os.Getenv(spec.APIKeys[0])
	}

	maxRetries := spec.MaxRetries
	if maxRetries == 0 && r.cfg != nil {
		maxRetries = r.cfg.LLM.MaxRetries
	}

	retryBackoff := time.Duration(spec.RetryBackoff)
	if retryBackoff == 0 && r.cfg != nil {
		retryBackoff = time.Duration(r.cfg.LLM.RetryBackoff)
	}

	client := NewClient(spec.Provider, model, apiKey, maxRetries, retryBackoff, spec.URL)
	if len(spec.APIKeyPool) > 0 {
		client.APIKeys = spec.APIKeyPool
	}
	if spec.MaxTimeout > 0 {
		client.Timeout = time.Duration(spec.MaxTimeout)
	} else if r.cfg != nil && r.cfg.LLM.MaxTimeout > 0 {
		client.Timeout = time.Duration(r.cfg.LLM.MaxTimeout)
	}

	if spec.IdleTimeout > 0 {
		client.IdleTimeout = time.Duration(spec.IdleTimeout)
	} else if r.cfg != nil && r.cfg.LLM.IdleTimeout > 0 {
		client.IdleTimeout = time.Duration(r.cfg.LLM.IdleTimeout)
	}

	if spec.MaxTokens > 0 {
		client.MaxTokens = spec.MaxTokens
	} else if r.cfg != nil && r.cfg.LLM.MaxTokens > 0 {
		client.MaxTokens = r.cfg.LLM.MaxTokens
	}

	if spec.Temperature != 0 {
		client.Temperature = spec.Temperature
	} else if r.cfg != nil && r.cfg.LLM.Temperature != 0 {
		client.Temperature = r.cfg.LLM.Temperature
	}

	if spec.Streaming != nil {
		client.Streaming = *spec.Streaming
	} else if r.cfg != nil && r.cfg.LLM.Streaming != nil {
		client.Streaming = *r.cfg.LLM.Streaming
	}

	if len(spec.ExtraParams) > 0 {
		client.ExtraParams = spec.ExtraParams
	}

	if spec.DisableJSONMode {
		client.DisableJSONMode = true
	}

	if spec.EnableThinking != nil {
		client.EnableThinking = spec.EnableThinking
	}

	if spec.ThinkingBudget != nil {
		client.ThinkingBudget = spec.ThinkingBudget
	}

	if r.cfg != nil {
		client.Compaction = r.cfg.Context.GetCompactionMode()
		client.CavemanCompaction = r.cfg.Context.CavemanCompaction
		client.MaxPromptTokens = r.cfg.LLM.MaxPromptTokens
	}

	client.SkipOnCreditExhausted = r.cfg == nil || r.cfg.LLM.SkipOnCreditExhausted

	return client
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

	var lastErr error
	for _, c := range candidates {
		// Check eviction & cooldown
		r.mu.RLock()
		evictedUntil, isEvicted := r.evictedUntil[c.Name]
		until, inCooldown := r.cooldowns[c.Name]
		r.mu.RUnlock()

		if isEvicted && time.Now().Before(evictedUntil) {
			continue
		}

		if inCooldown && time.Now().Before(until) {
			continue
		}

		resp, err := c.Client.Complete(ctx, prompt)
		if err == nil {
			// Record estimated token usage if budget store is present, using
			// the same prompt+completion estimation as FailoverClient so a
			// daily "token" limit means the same thing regardless of which
			// client the factory built.
			if r.budgetStore != nil && resp != nil {
				today := time.Now().UTC().Format("2006-01-02")
				tokens := estimateUsageTokens(prompt, resp)
				_ = r.budgetStore.IncrementUsage(ctx, today, c.Provider, tokens)
				_ = r.budgetStore.IncrementUsage(ctx, today, "total", tokens)
			}
			return resp, nil
		}

		lastErr = err
		if isEvictionError(err) {
			r.mu.Lock()
			r.evictedUntil[c.Name] = time.Now().Add(30 * time.Minute)
			r.evictionReasons[c.Name] = err.Error()
			r.mu.Unlock()
			fmt.Fprintf(os.Stderr, "⚠️ [LLM Provider Evicted] Candidate '%s' (provider '%s') EVICTED for 30 minutes due to depleted credits / auth failure: %v\n", c.Name, c.Provider, err)
		} else if isTransientError(err) {
			r.mu.Lock()
			r.cooldowns[c.Name] = time.Now().Add(r.cooldownDuration)
			r.mu.Unlock()
		}
	}

	if lastErr != nil {
		return nil, fmt.Errorf("all LLM provider candidates for role '%s' failed: %w", roleName, lastErr)
	}

	return nil, fmt.Errorf("all LLM provider candidates for role '%s' are currently in cooldown or evicted", roleName)
}

// isEvictionError reports whether an error signals credit exhaustion or auth failure (401/402).
func isEvictionError(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, ErrCreditExhausted) {
		return true
	}
	var he *httpError
	if errors.As(err, &he) {
		if he.StatusCode == 401 || he.StatusCode == 402 {
			return true
		}
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "creditserror") ||
		strings.Contains(msg, "insufficient balance") ||
		strings.Contains(msg, "payment required") ||
		strings.Contains(msg, "credit exhausted") ||
		strings.Contains(msg, "401 unauthorized") ||
		strings.Contains(msg, "402 payment required")
}

// GetEvictedProviders returns a map of candidate names to their eviction details.
func (r *ResilientLLMRouter) GetEvictedProviders() map[string]string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	res := make(map[string]string)
	now := time.Now()
	for name, until := range r.evictedUntil {
		if now.Before(until) {
			rem := time.Until(until).Round(time.Second)
			reason := r.evictionReasons[name]
			res[name] = fmt.Sprintf("Evicted for %v (Reason: %s)", rem, reason)
		}
	}
	return res
}
