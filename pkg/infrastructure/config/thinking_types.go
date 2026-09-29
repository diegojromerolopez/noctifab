package config

// ThinkingConfig controls chain-of-thought / extended reasoning settings for LLM models.
type ThinkingConfig struct {
	Enabled *bool `yaml:"enabled,omitempty"`
	Budget  *int  `yaml:"budget,omitempty"`
}

// IsEnabled returns true if thinking is explicitly enabled.
// By default (when nil or false), thinking is disabled.
func (t *ThinkingConfig) IsEnabled() bool {
	return t != nil && t.Enabled != nil && *t.Enabled
}

// GetBudget returns the configured thinking token budget, or 0 if unset.
func (t *ThinkingConfig) GetBudget() int {
	if t != nil && t.Budget != nil {
		return *t.Budget
	}
	return 0
}

// GetEnableThinking resolves enable_thinking preference from ThinkingConfig or fallback EnableThinking.
func (p ProviderSpec) GetEnableThinking() *bool {
	if p.Thinking != nil && p.Thinking.Enabled != nil {
		return p.Thinking.Enabled
	}
	return p.EnableThinking
}

// GetThinkingBudget resolves thinking_budget preference from ThinkingConfig or fallback ThinkingBudget.
func (p ProviderSpec) GetThinkingBudget() *int {
	if p.Thinking != nil && p.Thinking.Budget != nil {
		return p.Thinking.Budget
	}
	return p.ThinkingBudget
}

// GetEnableThinking resolves enable_thinking preference from ThinkingConfig or fallback EnableThinking.
func (a AgentProviderRef) GetEnableThinking() *bool {
	if a.Thinking != nil && a.Thinking.Enabled != nil {
		return a.Thinking.Enabled
	}
	return a.EnableThinking
}

// GetThinkingBudget resolves thinking_budget preference from ThinkingConfig or fallback ThinkingBudget.
func (a AgentProviderRef) GetThinkingBudget() *int {
	if a.Thinking != nil && a.Thinking.Budget != nil {
		return a.Thinking.Budget
	}
	return a.ThinkingBudget
}
