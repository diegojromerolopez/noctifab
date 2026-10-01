package llm

import (
	"os"
	"time"

	"github.com/diegojromerolopez/noctifab/pkg/domain"
	"github.com/diegojromerolopez/noctifab/pkg/infrastructure/config"
)

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

	if th := spec.GetEnableThinking(); th != nil {
		client.EnableThinking = th
	}

	if tb := spec.GetThinkingBudget(); tb != nil {
		client.ThinkingBudget = tb
	} else if client.EnableThinking != nil && *client.EnableThinking {
		defaultBudget := 2048
		if r.cfg != nil && r.cfg.LLM.Thinking != nil {
			defaultBudget = r.cfg.LLM.Thinking.GetDefaultBudget()
		}
		client.ThinkingBudget = &defaultBudget
	}

	if r.cfg != nil {
		client.Compaction = r.cfg.Context.GetCompactionMode()
		client.CavemanCompaction = r.cfg.Context.CavemanCompaction
		client.MaxPromptTokens = r.cfg.LLM.MaxPromptTokens
		client.JSONReminderTaskCap = r.cfg.LLM.JSONReminder.GetTaskCap()
		client.JSONReminderBodyCap = r.cfg.LLM.JSONReminder.GetBodyCap()
	}

	client.SkipOnCreditExhausted = r.cfg == nil || r.cfg.LLM.SkipOnCreditExhausted

	return client
}
