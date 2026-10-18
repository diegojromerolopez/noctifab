package config

import "time"

// This file resolves the effective value of common LLM settings for one
// provider. A provider value always wins over the global llm.* value. Fields
// that the provider does not set are inherited from the global value.

// ResolveThinkingEnabled returns the effective thinking toggle for a provider.
// Provider settings (thinking.enabled / enable_thinking) always win. When the
// provider is silent, the global llm.thinking.enabled is inherited only when it
// is true. A global "false" is the built-in default, so the request does not
// carry an explicit flag (some APIs reject unknown thinking parameters).
func (l LLMConfig) ResolveThinkingEnabled(p ProviderSpec) *bool {
	if th := p.GetEnableThinking(); th != nil {
		return th
	}
	if l.Thinking.IsEnabled() {
		enabled := true
		return &enabled
	}
	return nil
}

// ResolveThinkingBudget returns the effective thinking budget for a provider.
// Order: provider budget, global llm.thinking.budget, then
// llm.thinking.default_budget when thinking is enabled. It returns nil when
// thinking is not enabled and no budget is set.
func (l LLMConfig) ResolveThinkingBudget(p ProviderSpec, thinkingEnabled bool) *int {
	if tb := p.GetThinkingBudget(); tb != nil {
		return tb
	}
	if l.Thinking != nil && l.Thinking.Budget != nil && thinkingEnabled {
		budget := *l.Thinking.Budget
		return &budget
	}
	if thinkingEnabled {
		budget := l.Thinking.GetDefaultBudget()
		return &budget
	}
	return nil
}

// ResolveHedging merges the provider hedging block over llm.hedging.
func (l LLMConfig) ResolveHedging(p ProviderSpec) HedgingConfig {
	merged := l.Hedging
	if p.Hedging == nil {
		return merged
	}
	if p.Hedging.Enabled != nil {
		enabled := *p.Hedging.Enabled
		merged.Enabled = &enabled
	}
	if p.Hedging.Delay > 0 {
		merged.Delay = p.Hedging.Delay
	}
	if p.Hedging.HeavyDelay > 0 {
		merged.HeavyDelay = p.Hedging.HeavyDelay
	}
	return merged
}

// ResolveJSONReminder merges the provider json_reminder block over llm.json_reminder.
func (l LLMConfig) ResolveJSONReminder(p ProviderSpec) JSONReminderConfig {
	merged := l.JSONReminder
	if p.JSONReminder == nil {
		return merged
	}
	if p.JSONReminder.Task.Cap > 0 {
		merged.Task.Cap = p.JSONReminder.Task.Cap
	}
	if p.JSONReminder.Body.Cap > 0 {
		merged.Body.Cap = p.JSONReminder.Body.Cap
	}
	return merged
}

// ResolveTokenUsageLimit returns the daily token cap for one provider.
// It returns 0 (no provider cap) when the provider does not set a value.
// The global llm.token_usage_limit is a separate cap on the sum of all
// providers. It is not copied into each provider.
func (l LLMConfig) ResolveTokenUsageLimit(p ProviderSpec) int64 {
	if p.TokenUsageLimit != nil && *p.TokenUsageLimit > 0 {
		return *p.TokenUsageLimit
	}
	return 0
}

// ProviderTransport holds the effective request settings for one provider.
// A field with its Set flag false means "keep the client default".
type ProviderTransport struct {
	MaxRetries     int
	RetryBackoff   time.Duration
	MaxTimeout     time.Duration
	IdleTimeout    time.Duration
	MaxTokens      int // 0 = provider default / unlimited
	Temperature    float64
	TemperatureSet bool
	Streaming      bool
	StreamingSet   bool
}

// ResolveTransport merges max_retries, retry_backoff, max_timeout,
// idle_timeout, max_tokens, temperature, and streaming. The provider value
// wins when the provider sets the key. An explicit zero in YAML (for example
// temperature: 0 or max_retries: 0) also wins. Otherwise the global llm.*
// value is used. A negative max_tokens means "unlimited" and resolves to 0.
func (l LLMConfig) ResolveTransport(p ProviderSpec) ProviderTransport {
	t := ProviderTransport{
		MaxRetries:   l.MaxRetries,
		RetryBackoff: time.Duration(l.RetryBackoff),
		MaxTimeout:   time.Duration(l.MaxTimeout),
		IdleTimeout:  time.Duration(l.IdleTimeout),
		MaxTokens:    l.MaxTokens,
		Temperature:  l.Temperature,
	}
	t.TemperatureSet = l.Temperature != 0
	if l.Streaming != nil {
		t.Streaming, t.StreamingSet = *l.Streaming, true
	}

	if p.MaxRetries != 0 || p.IsExplicit("max_retries") {
		t.MaxRetries = p.MaxRetries
	}
	if p.RetryBackoff > 0 {
		t.RetryBackoff = time.Duration(p.RetryBackoff)
	}
	if p.MaxTimeout > 0 {
		t.MaxTimeout = time.Duration(p.MaxTimeout)
	}
	if p.IdleTimeout > 0 {
		t.IdleTimeout = time.Duration(p.IdleTimeout)
	}
	if p.MaxTokens != 0 || p.IsExplicit("max_tokens") {
		t.MaxTokens = p.MaxTokens
	}
	if p.Temperature != 0 || p.IsExplicit("temperature") {
		t.Temperature, t.TemperatureSet = p.Temperature, true
	}
	if p.Streaming != nil {
		t.Streaming, t.StreamingSet = *p.Streaming, true
	}

	if t.MaxTokens < 0 {
		t.MaxTokens = 0
	}
	if t.MaxRetries < 0 {
		t.MaxRetries = 0
	}
	return t
}
