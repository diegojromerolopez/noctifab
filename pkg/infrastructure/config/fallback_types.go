package config

// FallbackAgentConfig configures the unified Fallback Agent (Omni-Agent) for autonomous recovery and repairs.
type FallbackAgentConfig struct {
	Enabled             bool               `yaml:"enabled"`
	Model               string             `yaml:"model,omitempty"`
	Temperature         float64            `yaml:"temperature,omitempty"`
	Profile             string             `yaml:"profile,omitempty"`
	Providers           []AgentProviderRef `yaml:"providers,omitempty"`
	MaxTurns            int                `yaml:"max_turns"`
	Timeout             Duration           `yaml:"timeout"`
	AllowSpecMutation   bool               `yaml:"allow_spec_mutation"`
	AllowScopeReduction bool               `yaml:"allow_scope_reduction"`
	EnforceSpecQuality  bool               `yaml:"enforce_spec_quality"`
}

// LastResortAgentConfig is a backwards-compatible alias for FallbackAgentConfig.
type LastResortAgentConfig = FallbackAgentConfig

// FallbackTriggersConfig configures trigger thresholds for summoning the Fallback Agent.
type FallbackTriggersConfig struct {
	RetriesExhaustion         bool `yaml:"retries_exhaustion"`
	CyclicLoopDetection       bool `yaml:"cyclic_loop_detection"`
	MissingToolchainFastAbort bool `yaml:"missing_toolchain_fast_abort"`
	QADeadlockTurns           int  `yaml:"qa_deadlock_turns"`
	WatchdogTimeoutTurns      int  `yaml:"watchdog_timeout_turns"`
	StallCountThreshold       int  `yaml:"stall_count_threshold"`
}

// LastResortTriggersConfig is a backwards-compatible alias for FallbackTriggersConfig.
type LastResortTriggersConfig = FallbackTriggersConfig

// FallbackConfig controls the unified Fallback Agent monitoring and sovereign execution.
type FallbackConfig struct {
	Enabled           bool                   `yaml:"enabled"`
	PollInterval      Duration               `yaml:"poll_interval"`
	MaxRetries        int                    `yaml:"max_retries"`
	StallThreshold    Duration               `yaml:"stall_threshold"`
	ConflictThreshold Duration               `yaml:"conflict_threshold"`
	LLMAssessment     bool                   `yaml:"llm_assessment"`
	BudgetCliffRatio  float64                `yaml:"budget_cliff_ratio"`
	Triggers          FallbackTriggersConfig `yaml:"triggers"`
}

// GetFallback returns the active FallbackConfig, checking Fallback then falling back to legacy Unblocker.
func (c *Config) GetFallback() FallbackConfig {
	if c.Fallback.Enabled {
		return c.Fallback
	}
	if c.Unblocker.Enabled {
		return c.Unblocker
	}
	if c.Fallback.PollInterval > 0 || c.Fallback.MaxRetries > 0 {
		return c.Fallback
	}
	if c.Unblocker.PollInterval > 0 || c.Unblocker.MaxRetries > 0 {
		return c.Unblocker
	}
	return c.Fallback
}

// GetFallback returns the active FallbackAgentConfig, checking Fallback then falling back to legacy LastResort.
func (a AgentsConfig) GetFallback() FallbackAgentConfig {
	if a.Fallback.Enabled || a.Fallback.Model != "" || a.Fallback.Profile != "" || len(a.Fallback.Providers) > 0 {
		return a.Fallback
	}
	if a.LastResort.Enabled || a.LastResort.Model != "" || a.LastResort.Profile != "" || len(a.LastResort.Providers) > 0 {
		return a.LastResort
	}
	return a.Fallback
}
