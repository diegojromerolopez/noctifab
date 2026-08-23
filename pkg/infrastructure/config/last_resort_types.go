package config

// LastResortAgentConfig configures the Last-Resort Agent for deep holistic unblocking.
type LastResortAgentConfig struct {
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

// LastResortTriggersConfig configures trigger thresholds for summoning the Last-Resort Agent.
type LastResortTriggersConfig struct {
	RetriesExhaustion         bool `yaml:"retries_exhaustion"`
	CyclicLoopDetection       bool `yaml:"cyclic_loop_detection"`
	MissingToolchainFastAbort bool `yaml:"missing_toolchain_fast_abort"`
	QADeadlockTurns           int  `yaml:"qa_deadlock_turns"`
	WatchdogTimeoutTurns      int  `yaml:"watchdog_timeout_turns"`
	StallCountThreshold       int  `yaml:"stall_count_threshold"`
}
