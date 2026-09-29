package llm

import (
	"testing"

	"github.com/diegojromerolopez/noctifab/pkg/infrastructure/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestResilientLLMRouter_ThinkingPerProviderAndRole(t *testing.T) {
	enabledTrue := true
	budget8192 := 8192

	cfg := &config.Config{
		LLM: config.LLMConfig{
			Priority: []string{"claude", "gemini", "openai"},
			Providers: []config.ProviderSpec{
				{Name: "claude", Provider: "anthropic", Model: "claude-3-7-sonnet-20250219"},
				{Name: "gemini", Provider: "gemini", Model: "gemini-2.5-pro"},
				{Name: "openai", Provider: "openai", Model: "gpt-4o"},
			},
		},
		Agents: config.AgentsConfig{
			ProductManager: config.AgentRoleConfig{
				Number:         1,
				Iterations:     2,
				MaxUserStories: 5,
				Passes:         3,
				Providers: []config.AgentProviderRef{
					{
						Name: "claude",
						Thinking: &config.ThinkingConfig{
							Enabled: &enabledTrue,
							Budget:  &budget8192,
						},
					},
					{
						Name: "gemini",
					},
					{
						Name: "openai",
					},
				},
			},
		},
	}

	router := NewResilientLLMRouter(cfg, nil)
	candidates := router.ResolveCandidatesForRole("product_manager")
	require.Len(t, candidates, 3)

	// Candidate 0: claude with thinking enabled and budget 8192
	assert.Equal(t, "claude", candidates[0].Name)
	c0, ok := candidates[0].Client.(*Client)
	require.True(t, ok)
	require.NotNil(t, c0.EnableThinking)
	assert.True(t, *c0.EnableThinking)
	require.NotNil(t, c0.ThinkingBudget)
	assert.Equal(t, 8192, *c0.ThinkingBudget)

	// Candidate 1: gemini without thinking (disabled by default)
	assert.Equal(t, "gemini", candidates[1].Name)
	c1, ok := candidates[1].Client.(*Client)
	require.True(t, ok)
	assert.Nil(t, c1.EnableThinking, "gemini should not have thinking enabled")
	assert.Nil(t, c1.ThinkingBudget, "gemini should not have thinking budget set")

	// Candidate 2: openai without thinking (disabled by default)
	assert.Equal(t, "openai", candidates[2].Name)
	c2, ok := candidates[2].Client.(*Client)
	require.True(t, ok)
	assert.Nil(t, c2.EnableThinking, "openai should not have thinking enabled")
	assert.Nil(t, c2.ThinkingBudget, "openai should not have thinking budget set")
}

func TestResilientLLMRouter_RoleLevelThinkingInheritance(t *testing.T) {
	enabledTrue := true
	budget4096 := 4096
	disabledFalse := false

	cfg := &config.Config{
		LLM: config.LLMConfig{
			Priority: []string{"planner-primary", "planner-fast"},
			Providers: []config.ProviderSpec{
				{Name: "planner-primary", Provider: "anthropic", Model: "claude-3-7-sonnet-20250219"},
				{Name: "planner-fast", Provider: "openai", Model: "gpt-4o-mini"},
			},
		},
		Agents: config.AgentsConfig{
			Planner: config.AgentRoleConfig{
				Thinking: &config.ThinkingConfig{
					Enabled: &enabledTrue,
					Budget:  &budget4096,
				},
				Providers: []config.AgentProviderRef{
					{
						Name: "planner-primary", // Inherits role-level thinking
					},
					{
						Name: "planner-fast", // Explicitly overrides role-level thinking
						Thinking: &config.ThinkingConfig{
							Enabled: &disabledFalse,
						},
					},
				},
			},
		},
	}

	router := NewResilientLLMRouter(cfg, nil)
	candidates := router.ResolveCandidatesForRole("planner")
	require.Len(t, candidates, 2)

	// Candidate 0: planner-primary inherits role-level thinking
	c0, ok := candidates[0].Client.(*Client)
	require.True(t, ok)
	require.NotNil(t, c0.EnableThinking)
	assert.True(t, *c0.EnableThinking)
	require.NotNil(t, c0.ThinkingBudget)
	assert.Equal(t, 4096, *c0.ThinkingBudget)

	// Candidate 1: planner-fast overrides thinking to disabled
	c1, ok := candidates[1].Client.(*Client)
	require.True(t, ok)
	require.NotNil(t, c1.EnableThinking)
	assert.False(t, *c1.EnableThinking)
}
