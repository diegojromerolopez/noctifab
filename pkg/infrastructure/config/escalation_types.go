package config

// TaskEscalationConfig configures adaptive thinking activation, progressive model upgrades,
// dirty worktree hygiene, and last-resort task splitting for high-complexity tasks.
type TaskEscalationConfig struct {
	Enabled                    bool     `yaml:"enabled"`
	ThinkingThresholdTurns     int      `yaml:"thinking_threshold_turns,omitempty"`
	ModelUpgradeThresholdTurns int      `yaml:"model_upgrade_threshold_turns,omitempty"`
	MaxModelUpgrades           int      `yaml:"max_model_upgrades,omitempty"`
	SplitOnMaxUpgrades         *bool    `yaml:"split_on_max_upgrades,omitempty"`
	CleanWorktreeOnEscalate    *bool    `yaml:"clean_worktree_on_escalate,omitempty"`
	UpgradedModels             []string `yaml:"upgraded_models,omitempty"`
}

// IsEnabled returns true if task escalation is active.
func (c TaskEscalationConfig) IsEnabled() bool {
	return c.Enabled
}

// GetThinkingThresholdTurns returns the number of failing turns/retries before enabling thinking mode.
func (c TaskEscalationConfig) GetThinkingThresholdTurns() int {
	if c.ThinkingThresholdTurns > 0 {
		return c.ThinkingThresholdTurns
	}
	return 2
}

// GetModelUpgradeThresholdTurns returns the number of failing turns under thinking before upgrading the model.
func (c TaskEscalationConfig) GetModelUpgradeThresholdTurns() int {
	if c.ModelUpgradeThresholdTurns > 0 {
		return c.ModelUpgradeThresholdTurns
	}
	return 2
}

// GetMaxModelUpgrades returns the maximum model upgrades before triggering last-resort task splitting.
func (c TaskEscalationConfig) GetMaxModelUpgrades() int {
	if c.MaxModelUpgrades > 0 {
		return c.MaxModelUpgrades
	}
	return 2
}

// IsSplitOnMaxUpgradesEnabled returns true if splitting high-complexity tasks is enabled after max upgrades.
func (c TaskEscalationConfig) IsSplitOnMaxUpgradesEnabled() bool {
	if c.SplitOnMaxUpgrades != nil {
		return *c.SplitOnMaxUpgrades
	}
	return true
}

// IsCleanWorktreeOnEscalateEnabled returns true if dirty worktrees are cleaned before model upgrades.
func (c TaskEscalationConfig) IsCleanWorktreeOnEscalateEnabled() bool {
	if c.CleanWorktreeOnEscalate != nil {
		return *c.CleanWorktreeOnEscalate
	}
	return true
}

// GetUpgradedModels returns the configured list of escalated models, or empty slice if default pool applies.
func (c TaskEscalationConfig) GetUpgradedModels() []string {
	return c.UpgradedModels
}
