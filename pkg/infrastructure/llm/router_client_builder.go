package llm

import (
	"os"

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

	llmCfg := config.LLMConfig{}
	if r.cfg != nil {
		llmCfg = r.cfg.LLM
	}
	tr := llmCfg.ResolveTransport(spec)

	client := NewClient(spec.Provider, model, apiKey, tr.MaxRetries, tr.RetryBackoff, spec.URL)
	if len(spec.APIKeyPool) > 0 {
		client.APIKeys = spec.APIKeyPool
	}
	if tr.MaxTimeout > 0 {
		client.Timeout = tr.MaxTimeout
	}
	if tr.IdleTimeout > 0 {
		client.IdleTimeout = tr.IdleTimeout
	}
	client.MaxTokens = tr.MaxTokens
	if tr.TemperatureSet {
		client.Temperature = tr.Temperature
	}
	if tr.StreamingSet {
		client.Streaming = tr.Streaming
	}

	if len(spec.ExtraParams) > 0 {
		client.ExtraParams = spec.ExtraParams
	}

	if spec.DisableJSONMode {
		client.DisableJSONMode = true
	}

	client.EnableThinking = llmCfg.ResolveThinkingEnabled(spec)
	thinkingOn := client.EnableThinking != nil && *client.EnableThinking
	client.ThinkingBudget = llmCfg.ResolveThinkingBudget(spec, thinkingOn)

	reminder := llmCfg.ResolveJSONReminder(spec)
	client.JSONReminderTaskCap = reminder.GetTaskCap()
	client.JSONReminderBodyCap = reminder.GetBodyCap()

	if r.cfg != nil {
		client.Compaction = r.cfg.Context.GetCompactionMode()
		client.CavemanCompaction = r.cfg.Context.CavemanCompaction
		client.MaxPromptTokens = r.cfg.LLM.MaxPromptTokens
	}

	client.SkipOnCreditExhausted = r.cfg == nil || r.cfg.LLM.SkipOnCreditExhausted

	return client
}
