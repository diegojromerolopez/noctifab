package config

// EnsembleStrategy represents the multi-model execution topology for an agent role.
type EnsembleStrategy string

const (
	EnsembleStrategyParallel      EnsembleStrategy = "parallel"
	EnsembleStrategySerial        EnsembleStrategy = "serial"
	EnsembleStrategyConsensus     EnsembleStrategy = "consensus"
	EnsembleStrategyRace          EnsembleStrategy = "race"
	EnsembleStrategyDecomposed    EnsembleStrategy = "decomposed"
	EnsembleStrategyCascade       EnsembleStrategy = "cascade"
	EnsembleStrategyBestOfNScored EnsembleStrategy = "best_of_n_scored"
	EnsembleStrategyAdaptive      EnsembleStrategy = "adaptive"
)

// EnsembleConfig defines configuration for multi-model ensembling on an agent role.
type EnsembleConfig struct {
	Strategy               EnsembleStrategy `yaml:"strategy"`
	TimeoutSeconds         int              `yaml:"timeout_seconds"`
	SoftTimeoutSeconds     int              `yaml:"soft_timeout_seconds,omitempty"`
	MinModels              int              `yaml:"min_models,omitempty"`
	EarlyExitOnPass        bool             `yaml:"early_exit_on_pass,omitempty"`
	FallbackToSingle       bool             `yaml:"fallback_to_single"`
	FallbackOnStageFailure bool             `yaml:"fallback_on_stage_failure"`

	// Models for Parallel, Race, and BestOfNScored
	Models      []AgentProviderRef `yaml:"models,omitempty"`
	Synthesizer *AgentProviderRef  `yaml:"synthesizer,omitempty"`

	// Serial Refinement Strategy
	Stages []EnsembleStageSpec `yaml:"stages,omitempty"`

	// Consensus Strategy (consensus)
	Voters     []AgentProviderRef `yaml:"voters,omitempty"`
	TieBreaker *AgentProviderRef  `yaml:"tie_breaker,omitempty"`

	// Tiered Cascade Strategy
	Tiers []AgentProviderRef `yaml:"tiers,omitempty"`

	// Adaptive Dynamic Strategy Tiers
	FastTier     []AgentProviderRef `yaml:"fast_tier,omitempty"`
	StandardTier []AgentProviderRef `yaml:"standard_tier,omitempty"`
	HeavyTier    []AgentProviderRef `yaml:"heavy_tier,omitempty"`

	// Decomposed Divide-and-Conquer Strategy
	Targets []DecomposedTargetSpec `yaml:"targets,omitempty"`
}

// IsEnabled returns true if a valid ensemble strategy is configured.
func (e EnsembleConfig) IsEnabled() bool {
	return e.Strategy != ""
}

// DecomposedTargetSpec specifies a target sub-task in a divide-and-conquer strategy.
type DecomposedTargetSpec struct {
	Name        string            `yaml:"name"` // References named provider from llm.providers
	RolePrompt  string            `yaml:"role_prompt,omitempty"`
	MaxTokens   *int              `yaml:"max_tokens,omitempty"`
	ExtraParams map[string]string `yaml:"extra_params,omitempty"`
}

// EnsembleStageSpec defines a stage in a sequential refinement pipeline.
type EnsembleStageSpec struct {
	Name             string            `yaml:"name"` // References named provider from llm.providers
	MaxTokens        *int              `yaml:"max_tokens,omitempty"`
	Temperature      *float64          `yaml:"temperature,omitempty"`
	RefinementPrompt string            `yaml:"refinement_prompt,omitempty"`
	ExtraParams      map[string]string `yaml:"extra_params,omitempty"`
}
