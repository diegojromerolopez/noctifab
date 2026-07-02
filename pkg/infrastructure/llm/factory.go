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
func BuildFailoverClient(cfg *config.LLMConfig, budgetStore domain.BudgetStore) domain.LLMClient {
	if !cfg.Failover.Enabled || len(cfg.Failover.Backends) == 0 {
		return NewClient(
			cfg.Provider, cfg.Model, cfg.APIKeyValue,
			cfg.MaxRetries, time.Duration(cfg.RetryBackoff), cfg.URL,
		)
	}

	backends := make([]NamedClient, 0, len(cfg.Failover.Backends))
	for _, b := range cfg.Failover.Backends {
		apiKey := os.Getenv(b.APIKeyEnv)
		client := NewClient(b.Provider, b.Model, apiKey, b.MaxRetries, time.Duration(cfg.RetryBackoff), b.URL)
		backends = append(backends, NamedClient{
			Name:   b.Provider + "/" + b.Model,
			Model:  b.Model,
			Client: client,
		})
	}

	return NewFailoverClient(backends, time.Duration(cfg.Failover.Cooldown), cfg.Failover.MaxCallLimit, budgetStore, cfg.MaxBudgetUSD)
}
