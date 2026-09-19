package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"gopkg.in/yaml.v3"
)

func TestThinkingConfig_DefaultsAndResolution(t *testing.T) {
	t.Run("default is disabled when nil or empty", func(t *testing.T) {
		var cfg ThinkingConfig
		assert.False(t, cfg.IsEnabled())
		assert.Equal(t, 0, cfg.GetBudget())

		var nilCfg *ThinkingConfig
		assert.False(t, nilCfg.IsEnabled())
		assert.Equal(t, 0, nilCfg.GetBudget())
	})

	t.Run("explicitly enabled with budget", func(t *testing.T) {
		enabled := true
		budget := 8192
		cfg := ThinkingConfig{
			Enabled: &enabled,
			Budget:  &budget,
		}
		assert.True(t, cfg.IsEnabled())
		assert.Equal(t, 8192, cfg.GetBudget())
	})

	t.Run("YAML unmarshaling with thinking block", func(t *testing.T) {
		yamlStr := `
product_manager:
  number: 1
  iterations: 2
  max_user_stories: 5
  passes: 3
  providers:
    - name: claude
      thinking:
        enabled: true
        budget: 8192
    - name: gemini
    - name: openai
`
		var parsed struct {
			ProductManager AgentRoleConfig `yaml:"product_manager"`
		}
		err := yaml.Unmarshal([]byte(yamlStr), &parsed)
		assert.NoError(t, err)
		assert.Equal(t, 1, parsed.ProductManager.Number)
		assert.Equal(t, 2, parsed.ProductManager.Iterations)
		assert.Equal(t, 5, parsed.ProductManager.MaxUserStories)
		assert.Equal(t, 3, len(parsed.ProductManager.Providers))

		// Provider 0 (claude): thinking enabled with 8192 budget
		p0 := parsed.ProductManager.Providers[0]
		assert.Equal(t, "claude", p0.Name)
		assert.NotNil(t, p0.Thinking)
		assert.True(t, p0.Thinking.IsEnabled())
		assert.Equal(t, 8192, p0.Thinking.GetBudget())
		assert.True(t, *p0.GetEnableThinking())
		assert.Equal(t, 8192, *p0.GetThinkingBudget())

		// Provider 1 (gemini): thinking disabled / nil
		p1 := parsed.ProductManager.Providers[1]
		assert.Equal(t, "gemini", p1.Name)
		assert.Nil(t, p1.Thinking)
		assert.False(t, p1.Thinking.IsEnabled())
		assert.Nil(t, p1.GetEnableThinking())
		assert.Nil(t, p1.GetThinkingBudget())

		// Provider 2 (openai): thinking disabled / nil
		p2 := parsed.ProductManager.Providers[2]
		assert.Equal(t, "openai", p2.Name)
		assert.Nil(t, p2.Thinking)
		assert.False(t, p2.Thinking.IsEnabled())
	})

	t.Run("backward compatibility with flat enable_thinking and thinking_budget", func(t *testing.T) {
		enabled := true
		budget := 4096
		ref := AgentProviderRef{
			Name:           "custom",
			EnableThinking: &enabled,
			ThinkingBudget: &budget,
		}
		assert.True(t, *ref.GetEnableThinking())
		assert.Equal(t, 4096, *ref.GetThinkingBudget())

		spec := ProviderSpec{
			Name:           "custom-spec",
			EnableThinking: &enabled,
			ThinkingBudget: &budget,
		}
		assert.True(t, *spec.GetEnableThinking())
		assert.Equal(t, 4096, *spec.GetThinkingBudget())
	})
}
