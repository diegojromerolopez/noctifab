package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

func TestSovereignRescueConfig_SlidingWindow(t *testing.T) {
	t.Run("defaults to 0 (disabled) when context is omitted", func(t *testing.T) {
		cfgYAML := `
sovereign_rescue:
  enabled: true
  max_turns: 5
`
		var wrapper struct {
			SovereignRescue SovereignRescueConfig `yaml:"sovereign_rescue"`
		}
		require.NoError(t, yaml.Unmarshal([]byte(cfgYAML), &wrapper))
		assert.Equal(t, 0, wrapper.SovereignRescue.GetSlidingWindow())
	})

	t.Run("decodes context.sliding_window correctly from YAML", func(t *testing.T) {
		cfgYAML := `
sovereign_rescue:
  enabled: true
  max_turns: 5
  context:
    sliding_window: 15000
`
		var wrapper struct {
			SovereignRescue SovereignRescueConfig `yaml:"sovereign_rescue"`
		}
		require.NoError(t, yaml.Unmarshal([]byte(cfgYAML), &wrapper))
		assert.Equal(t, 15000, wrapper.SovereignRescue.GetSlidingWindow())
	})

	t.Run("environment variable NOCTIFAB_RESCUE_SLIDING_WINDOW overrides sliding window", func(t *testing.T) {
		t.Setenv("NOCTIFAB_RESCUE_SLIDING_WINDOW", "8000")

		cfg := DefaultConfig()
		applyEnvOverrides(cfg)

		assert.Equal(t, 8000, cfg.Fallback.SovereignRescue.GetSlidingWindow())
	})
}
