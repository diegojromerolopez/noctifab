package config

import (
	"os"
	"strconv"
	"strings"
	"time"
)

// SovereignRescueConfig configures the autonomous whole-project sovereign recovery engine.
type SovereignRescueConfig struct {
	Enabled                  *bool              `yaml:"enabled,omitempty"`
	MaxTurns                 int                `yaml:"max_turns"`
	MaxAttempts              int                `yaml:"max_attempts,omitempty"`
	Timeout                  Duration           `yaml:"timeout,omitempty"`
	MissingToolchainStrategy string             `yaml:"missing_toolchain_strategy,omitempty"`
	Providers                []AgentProviderRef `yaml:"providers,omitempty"`
}

func (s SovereignRescueConfig) IsEnabled() bool {
	if s.Enabled != nil {
		return *s.Enabled
	}
	return true
}

func (s SovereignRescueConfig) GetMissingToolchainStrategy() string {
	strat := strings.ToLower(strings.TrimSpace(s.MissingToolchainStrategy))
	if strat == "" {
		return "auto"
	}
	return strat
}

func (s SovereignRescueConfig) GetMaxTurns() int {
	if s.MaxTurns > 0 {
		return s.MaxTurns
	}
	if s.MaxAttempts > 0 {
		return s.MaxAttempts
	}
	return 10
}

func (s SovereignRescueConfig) GetTimeout() time.Duration {
	if s.Timeout > 0 {
		return time.Duration(s.Timeout)
	}
	return 5 * time.Minute
}

// FallbackAgentConfig configures the unified Fallback Agent (Omni-Agent) for autonomous recovery and repairs.
type FallbackAgentConfig struct {
	Enabled             bool                   `yaml:"enabled"`
	Model               string                 `yaml:"model,omitempty"`
	Temperature         float64                `yaml:"temperature,omitempty"`
	Profile             string                 `yaml:"profile,omitempty"`
	Providers           []AgentProviderRef     `yaml:"providers,omitempty"`
	MaxTurns            int                    `yaml:"max_turns"`
	Timeout             Duration               `yaml:"timeout"`
	AllowSpecMutation   bool                   `yaml:"allow_spec_mutation"`
	AllowScopeReduction bool                   `yaml:"allow_scope_reduction"`
	EnforceSpecQuality  bool                   `yaml:"enforce_spec_quality"`
	RescueMaxTurns      int                    `yaml:"rescue_max_turns,omitempty"`
	SovereignRescue     *SovereignRescueConfig `yaml:"sovereign_rescue,omitempty"`
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
	SovereignRescue   SovereignRescueConfig  `yaml:"sovereign_rescue"`
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

// GetSovereignRescue resolves the SovereignRescueConfig from fallback.sovereign_rescue
// or agents.fallback.sovereign_rescue/rescue_max_turns with a default MaxTurns of 10.
// The NOCTIFAB_RESCUE_MAX_TURNS environment variable takes highest precedence if set.
func (c *Config) GetSovereignRescue() SovereignRescueConfig {
	if c == nil {
		res := SovereignRescueConfig{MaxTurns: 10, Timeout: Duration(5 * time.Minute), MissingToolchainStrategy: "auto"}
		if val, ok := os.LookupEnv("NOCTIFAB_RESCUE_MAX_TURNS"); ok {
			if i, err := strconv.Atoi(val); err == nil && i > 0 {
				res.MaxTurns = i
			}
		} else if val, ok := os.LookupEnv("NOCTIFAB_RESCUE_MAX_ATTEMPTS"); ok {
			if i, err := strconv.Atoi(val); err == nil && i > 0 {
				res.MaxTurns = i
			}
		}
		if val, ok := os.LookupEnv("NOCTIFAB_RESCUE_TOOLCHAIN_STRATEGY"); ok && strings.TrimSpace(val) != "" {
			res.MissingToolchainStrategy = strings.ToLower(strings.TrimSpace(val))
		}
		return res
	}
	fb := c.GetFallback()
	res := fb.SovereignRescue
	if res.MaxTurns <= 0 {
		if res.MaxAttempts > 0 {
			res.MaxTurns = res.MaxAttempts
		} else {
			fbAgent := c.Agents.GetFallback()
			if fbAgent.SovereignRescue != nil && fbAgent.SovereignRescue.MaxTurns > 0 {
				res.MaxTurns = fbAgent.SovereignRescue.MaxTurns
				if fbAgent.SovereignRescue.Timeout > 0 {
					res.Timeout = fbAgent.SovereignRescue.Timeout
				}
				if fbAgent.SovereignRescue.Enabled != nil {
					res.Enabled = fbAgent.SovereignRescue.Enabled
				}
				if fbAgent.SovereignRescue.MissingToolchainStrategy != "" {
					res.MissingToolchainStrategy = fbAgent.SovereignRescue.MissingToolchainStrategy
				}
				if len(fbAgent.SovereignRescue.Providers) > 0 {
					res.Providers = fbAgent.SovereignRescue.Providers
				}
			} else if fbAgent.RescueMaxTurns > 0 {
				res.MaxTurns = fbAgent.RescueMaxTurns
			}
		}
	}
	if len(res.Providers) == 0 {
		fbAgent := c.Agents.GetFallback()
		if fbAgent.SovereignRescue != nil && len(fbAgent.SovereignRescue.Providers) > 0 {
			res.Providers = fbAgent.SovereignRescue.Providers
		} else if len(fbAgent.Providers) > 0 {
			res.Providers = fbAgent.Providers
		}
	}
	if val, ok := os.LookupEnv("NOCTIFAB_RESCUE_MAX_TURNS"); ok {
		if i, err := strconv.Atoi(val); err == nil && i > 0 {
			res.MaxTurns = i
		}
	} else if val, ok := os.LookupEnv("NOCTIFAB_RESCUE_MAX_ATTEMPTS"); ok {
		if i, err := strconv.Atoi(val); err == nil && i > 0 {
			res.MaxTurns = i
		}
	}
	if val, ok := os.LookupEnv("NOCTIFAB_RESCUE_TOOLCHAIN_STRATEGY"); ok && strings.TrimSpace(val) != "" {
		res.MissingToolchainStrategy = strings.ToLower(strings.TrimSpace(val))
	}
	if res.MissingToolchainStrategy == "" {
		res.MissingToolchainStrategy = "auto"
	}
	if res.MaxTurns <= 0 {
		res.MaxTurns = 10
	}
	if res.Timeout <= 0 {
		res.Timeout = Duration(5 * time.Minute)
	}
	return res
}
