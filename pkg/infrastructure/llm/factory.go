package llm

import (
	"os"
	"time"

	"github.com/diegojromerolopez/noctifab/pkg/domain"
	"github.com/diegojromerolopez/noctifab/pkg/infrastructure/config"
)

// BuildFailoverClient constructs the appropriate LLM client based on config.
// If failover is disabled (default), returns a plain *Client.
// If failover is enabled, returns a *FailoverClient wrapping all backends.
// When budgetStore is non-nil, the returned client enforces MaxBudgetUSD.
func BuildFailoverClient(cfg *config.Config, budgetStore domain.BudgetStore) domain.LLMClient {
	if cfg == nil {
		return nil
	}

	// 1. If the new multiple-LLM client configuration list is provided, use it
	if len(cfg.LLMs) > 0 {
		backends := make([]NamedClient, 0, len(cfg.LLMs))
		for _, b := range cfg.LLMs {
			client := NewClient(
				b.Provider, b.Model, b.APIKeyValue,
				b.MaxRetries, time.Duration(b.RetryBackoff), b.URL,
			)
			if b.MaxTimeout > 0 {
				client.Timeout = time.Duration(b.MaxTimeout)
			}
			backends = append(backends, NamedClient{
				Name:   b.Provider + "/" + b.Model,
				Model:  b.Model,
				Client: client,
			})
		}

		// Determine failover settings. Default to 5 minutes cooldown, 0 call limit,
		// and use the first config's max budget (or global defaults).
		cooldown := 5 * time.Minute
		if cfg.LLM.Failover.Cooldown > 0 {
			cooldown = time.Duration(cfg.LLM.Failover.Cooldown)
		}
		maxLimit := cfg.LLM.Failover.MaxCallLimit
		maxBudget := cfg.LLM.MaxBudgetUSD

		return NewFailoverClient(backends, cooldown, maxLimit, budgetStore, maxBudget)
	}

	// 2. Fall back to the legacy config.LLM failover if enabled
	if cfg.LLM.Failover.Enabled && len(cfg.LLM.Failover.Backends) > 0 {
		backends := make([]NamedClient, 0, len(cfg.LLM.Failover.Backends))
		for _, b := range cfg.LLM.Failover.Backends {
			apiKey := os.Getenv(b.APIKeyEnv)
			client := NewClient(b.Provider, b.Model, apiKey, b.MaxRetries, time.Duration(cfg.LLM.RetryBackoff), b.URL)
			if cfg.LLM.MaxTimeout > 0 {
				client.Timeout = time.Duration(cfg.LLM.MaxTimeout)
			}
			backends = append(backends, NamedClient{
				Name:   b.Provider + "/" + b.Model,
				Model:  b.Model,
				Client: client,
			})
		}
		return NewFailoverClient(backends, time.Duration(cfg.LLM.Failover.Cooldown), cfg.LLM.Failover.MaxCallLimit, budgetStore, cfg.LLM.MaxBudgetUSD)
	}

	client := NewClient(
		cfg.LLM.Provider, cfg.LLM.Model, cfg.LLM.APIKeyValue,
		cfg.LLM.MaxRetries, time.Duration(cfg.LLM.RetryBackoff), cfg.LLM.URL,
	)
	if cfg.LLM.MaxTimeout > 0 {
		client.Timeout = time.Duration(cfg.LLM.MaxTimeout)
	}
	return client
}
