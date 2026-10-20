package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestTaskEscalationConfig_Defaults(t *testing.T) {
	cfg := TaskEscalationConfig{}

	assert.False(t, cfg.IsEnabled())
	assert.Equal(t, 2, cfg.GetThinkingThresholdTurns())
	assert.Equal(t, 2, cfg.GetModelUpgradeThresholdTurns())
	assert.Equal(t, 2, cfg.GetMaxModelUpgrades())
	assert.True(t, cfg.IsSplitOnMaxUpgradesEnabled())
	assert.True(t, cfg.IsCleanWorktreeOnEscalateEnabled())
	assert.Empty(t, cfg.GetUpgradedModels())
}

func TestTaskEscalationConfig_CustomOverrides(t *testing.T) {
	split := false
	clean := false
	cfg := TaskEscalationConfig{
		Enabled:                    true,
		ThinkingThresholdTurns:     3,
		ModelUpgradeThresholdTurns: 4,
		MaxModelUpgrades:           1,
		SplitOnMaxUpgrades:         &split,
		CleanWorktreeOnEscalate:    &clean,
		UpgradedModels:             []string{"gemini-2.5-pro", "claude-3-7-sonnet"},
	}

	assert.True(t, cfg.IsEnabled())
	assert.Equal(t, 3, cfg.GetThinkingThresholdTurns())
	assert.Equal(t, 4, cfg.GetModelUpgradeThresholdTurns())
	assert.Equal(t, 1, cfg.GetMaxModelUpgrades())
	assert.False(t, cfg.IsSplitOnMaxUpgradesEnabled())
	assert.False(t, cfg.IsCleanWorktreeOnEscalateEnabled())
	assert.Equal(t, []string{"gemini-2.5-pro", "claude-3-7-sonnet"}, cfg.GetUpgradedModels())
}
