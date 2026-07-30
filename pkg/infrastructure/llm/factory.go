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
// When budgetStore is non-nil, the returned client enforces TokenUsageLimit.
func BuildFailoverClient(cfg *config.Config, budgetStore domain.BudgetStore) domain.LLMClient {
	if cfg == nil {
		return nil
	}

	// 0. If named providers or global priority list is configured, use the per-agent ResilientLLMRouter
	if len(cfg.LLM.Providers) > 0 || len(cfg.LLM.Priority) > 0 {
		return NewResilientLLMRouter(cfg, budgetStore)
	}

	// 1. If the legacy multiple-LLM client configuration list is provided, use it
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
			if b.IdleTimeout > 0 {
				client.IdleTimeout = time.Duration(b.IdleTimeout)
			}
			if b.Streaming != nil {
				client.Streaming = *b.Streaming
			} else if cfg.LLM.Streaming != nil {
				client.Streaming = *cfg.LLM.Streaming
			}
			backends = append(backends, NamedClient{
				Name:   b.Provider + "/" + b.Model,
				Model:  b.Model,
				Client: client,
			})
		}

		// Determine failover settings. Default to 5 minutes cooldown and 0 call limit.
		cooldown := 5 * time.Minute
		if cfg.LLM.Failover.Cooldown > 0 {
			cooldown = time.Duration(cfg.LLM.Failover.Cooldown)
		}
		maxLimit := cfg.LLM.Failover.MaxCallLimit

		return NewFailoverClient(backends, cooldown, maxLimit, budgetStore, cfg.TokenUsageLimit)
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
			if b.IdleTimeout > 0 {
				client.IdleTimeout = time.Duration(b.IdleTimeout)
			} else if cfg.LLM.IdleTimeout > 0 {
				client.IdleTimeout = time.Duration(cfg.LLM.IdleTimeout)
			}
			if b.Streaming != nil {
				client.Streaming = *b.Streaming
			} else if cfg.LLM.Streaming != nil {
				client.Streaming = *cfg.LLM.Streaming
			}
			backends = append(backends, NamedClient{
				Name:   b.Provider + "/" + b.Model,
				Model:  b.Model,
				Client: client,
			})
		}
		return NewFailoverClient(backends, time.Duration(cfg.LLM.Failover.Cooldown), cfg.LLM.Failover.MaxCallLimit, budgetStore, cfg.TokenUsageLimit)
	}

	client := NewClient(
		cfg.LLM.Provider, cfg.LLM.Model, cfg.LLM.APIKeyValue,
		cfg.LLM.MaxRetries, time.Duration(cfg.LLM.RetryBackoff), cfg.LLM.URL,
	)
	if cfg.LLM.MaxTimeout > 0 {
		client.Timeout = time.Duration(cfg.LLM.MaxTimeout)
	}
	if cfg.LLM.IdleTimeout > 0 {
		client.IdleTimeout = time.Duration(cfg.LLM.IdleTimeout)
	}
	if cfg.LLM.Streaming != nil {
		client.Streaming = *cfg.LLM.Streaming
	}
	return client
}
