package config

import (
	"testing"
)

func TestAgentsConfig_GetMaxTurnExtensions(t *testing.T) {
	t.Run("defaults to 3 when unset or zero", func(t *testing.T) {
		cfg := AgentsConfig{}
		if got := cfg.GetMaxTurnExtensions(); got != 3 {
			t.Errorf("expected default 3, got %d", got)
		}
	})

	t.Run("returns configured value when set", func(t *testing.T) {
		cfg := AgentsConfig{MaxTurnExtensions: 5}
		if got := cfg.GetMaxTurnExtensions(); got != 5 {
			t.Errorf("expected 5, got %d", got)
		}
	})
}
