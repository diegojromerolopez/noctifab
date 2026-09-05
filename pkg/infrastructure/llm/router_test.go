package llm

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/diegojromerolopez/noctifab/pkg/domain"
	"github.com/diegojromerolopez/noctifab/pkg/infrastructure/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type mockLLMClient struct {
	completeFn func(ctx context.Context, prompt string) (*domain.LLMResponse, error)
}

func (m *mockLLMClient) Complete(ctx context.Context, prompt string) (*domain.LLMResponse, error) {
	if m.completeFn != nil {
		return m.completeFn(ctx, prompt)
	}
	return &domain.LLMResponse{Reasoning: "ok"}, nil
}

func TestResilientLLMRouter_Scenarios(t *testing.T) {
	t.Run("removed roles cannot resolve an LLM candidate", func(t *testing.T) {
		router := NewResilientLLMRouter(config.DefaultConfig(), nil)
		for _, role := range []string{"architect", "security", "performance", "docs", "devops"} {
			assert.Empty(t, router.ResolveCandidatesForRole(role), role)
		}
	})

	t.Run("Scenario 1: Agent without providers uses global llm.priority", func(t *testing.T) {
		cfg := &config.Config{
			LLM: config.LLMConfig{
				Priority: []string{"openai-primary", "anthropic-primary"},
				Providers: []config.ProviderSpec{
					{Name: "openai-primary", Provider: "openai", Model: "gpt-4o"},
					{Name: "anthropic-primary", Provider: "anthropic", Model: "claude-3-5-sonnet-latest"},
				},
			},
		}

		router := NewResilientLLMRouter(cfg, nil)
		candidates := router.ResolveCandidatesForRole("unconfigured-role")

		require.Len(t, candidates, 2)
		assert.Equal(t, "openai-primary", candidates[0].Name)
		assert.Equal(t, "gpt-4o", candidates[0].Model)
		assert.Equal(t, "anthropic-primary", candidates[1].Name)
		assert.Equal(t, "claude-3-5-sonnet-latest", candidates[1].Model)
	})

	t.Run("Scenario 2: Agent with provider but no model (version-agnostic)", func(t *testing.T) {
		cfg := &config.Config{
			LLM: config.LLMConfig{
				Priority: []string{"openai-primary"},
				Providers: []config.ProviderSpec{
					{Name: "openai-primary", Provider: "openai"}, // model omitted
				},
			},
			Roles: config.RolesConfig{
				Generator: config.RoleSetting{
					Providers: []config.AgentProviderRef{
						{Name: "openai-primary"}, // model omitted -> version-agnostic
					},
				},
			},
		}

		router := NewResilientLLMRouter(cfg, nil)
		candidates := router.ResolveCandidatesForRole("generator")

		require.Len(t, candidates, 1)
		assert.Equal(t, "openai-primary", candidates[0].Name)
		assert.Equal(t, "openai", candidates[0].Provider)
		assert.Equal(t, "", candidates[0].Model) // empty string triggers dynamic capacity discovery in Client
	})

	t.Run("Scenario 3: Agent with provider and explicit single model or models sequence", func(t *testing.T) {
		cfg := &config.Config{
			LLM: config.LLMConfig{
				Providers: []config.ProviderSpec{
					{Name: "anthropic-primary", Provider: "anthropic"},
					{Name: "openai-primary", Provider: "openai"},
				},
			},
			Roles: config.RolesConfig{
				Planner: config.RoleSetting{
					Providers: []config.AgentProviderRef{
						{
							Name:   "anthropic-primary",
							Models: []string{"claude-3-7-sonnet", "claude-3-5-sonnet-latest"},
						},
						{
							Name:  "openai-primary",
							Model: "gpt-4o",
						},
					},
				},
			},
		}

		router := NewResilientLLMRouter(cfg, nil)
		candidates := router.ResolveCandidatesForRole("planner")

		require.Len(t, candidates, 3)
		assert.Equal(t, "claude-3-7-sonnet", candidates[0].Model)
		assert.Equal(t, "claude-3-5-sonnet-latest", candidates[1].Model)
		assert.Equal(t, "gpt-4o", candidates[2].Model)
	})

	t.Run("Scenario 4: Bad provider reference in role is ignored, falls through to valid or llm.priority", func(t *testing.T) {
		cfg := &config.Config{
			LLM: config.LLMConfig{
				Priority: []string{"valid-global-provider"},
				Providers: []config.ProviderSpec{
					{Name: "valid-provider", Provider: "openai", Model: "gpt-4o"},
					{Name: "valid-global-provider", Provider: "anthropic", Model: "claude-3-5-haiku-latest"},
				},
			},
			Roles: config.RolesConfig{
				Tester: config.RoleSetting{
					Providers: []config.AgentProviderRef{
						{Name: "non-existent-bad-provider"}, // Bad provider -> ignored!
						{Name: "valid-provider"},            // Valid
					},
				},
			},
		}

		router := NewResilientLLMRouter(cfg, nil)
		candidates := router.ResolveCandidatesForRole("tester")

		require.Len(t, candidates, 2)
		assert.Equal(t, "valid-provider", candidates[0].Name)
		assert.Equal(t, "valid-global-provider", candidates[1].Name)
	})

	t.Run("Scenario 5: Inline provider shorthand & bad model runtime fallback", func(t *testing.T) {
		cfg := &config.Config{
			LLM: config.LLMConfig{
				Priority: []string{"global-backup"},
				Providers: []config.ProviderSpec{
					{Name: "global-backup", Provider: "openai", Model: "gpt-4o-mini"},
				},
			},
			Roles: config.RolesConfig{
				QA: config.RoleSetting{
					Providers: []config.AgentProviderRef{
						{Provider: "anthropic", Model: "claude-3-5-sonnet-latest"}, // inline shorthand
					},
				},
			},
		}

		router := NewResilientLLMRouter(cfg, nil)
		candidates := router.ResolveCandidatesForRole("qa")

		require.Len(t, candidates, 2)
		assert.Equal(t, "anthropic", candidates[0].Provider)
		assert.Equal(t, "claude-3-5-sonnet-latest", candidates[0].Model)
		assert.Equal(t, "global-backup", candidates[1].Name)
	})

	t.Run("Scenario 6: Complete execution falls through candidates on error until success", func(t *testing.T) {
		cfg := &config.Config{
			LLM: config.LLMConfig{
				Priority: []string{"p1", "p2"},
				Providers: []config.ProviderSpec{
					{Name: "p1", Provider: "openai", Model: "m1"},
					{Name: "p2", Provider: "anthropic", Model: "m2"},
				},
			},
			Roles: config.RolesConfig{
				Generator: config.RoleSetting{
					Providers: []config.AgentProviderRef{
						{Name: "p1"},
						{Name: "p2"},
					},
				},
			},
		}

		router := NewResilientLLMRouter(cfg, nil)
		require.NotNil(t, router)

		calls := make([]string, 0)
		mock1 := &mockLLMClient{
			completeFn: func(ctx context.Context, prompt string) (*domain.LLMResponse, error) {
				calls = append(calls, "p1")
				return nil, errors.New("p1 rate limited 429")
			},
		}
		mock2 := &mockLLMClient{
			completeFn: func(ctx context.Context, prompt string) (*domain.LLMResponse, error) {
				calls = append(calls, "p2")
				return &domain.LLMResponse{Reasoning: "p2 success"}, nil
			},
		}

		c1 := RouterCandidate{Name: "p1", Provider: "openai", Model: "m1", Client: mock1}
		c2 := RouterCandidate{Name: "p2", Provider: "anthropic", Model: "m2", Client: mock2}

		candidates := []RouterCandidate{c1, c2}
		var finalResp *domain.LLMResponse
		var finalErr error

		ctx := WithRoleContext(context.Background(), "generator")
		for _, c := range candidates {
			resp, err := c.Client.Complete(ctx, "test prompt")
			if err == nil {
				finalResp = resp
				finalErr = nil
				break
			}
			finalErr = err
		}

		require.NoError(t, finalErr)
		require.NotNil(t, finalResp)
		assert.Equal(t, "p2 success", finalResp.Reasoning)
		assert.Equal(t, []string{"p1", "p2"}, calls)
	})

	t.Run("Scenario 7: Cooldown protection prevents calling failed providers until duration expires", func(t *testing.T) {
		cfg := &config.Config{
			LLM: config.LLMConfig{
				Priority: []string{"p1"},
				Providers: []config.ProviderSpec{
					{Name: "p1", Provider: "openai", Model: "m1"},
				},
			},
		}

		router := NewResilientLLMRouter(cfg, nil)
		router.cooldownDuration = 1 * time.Hour

		// Mark p1 in cooldown manually
		router.mu.Lock()
		router.cooldowns["p1"] = time.Now().Add(1 * time.Hour)
		router.mu.Unlock()

		ctx := WithRoleContext(context.Background(), "generator")
		_, err := router.Complete(ctx, "test")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "cooldown")
	})

	t.Run("Scenario 8: Agent provider defined directly inside agents.<role>.providers takes precedence", func(t *testing.T) {
		cfg := &config.Config{
			LLM: config.LLMConfig{
				Priority: []string{"fallback-provider"},
				Providers: []config.ProviderSpec{
					{Name: "gemini-direct", Provider: "gemini", Model: "gemini-2.5-flash"},
					{Name: "fallback-provider", Provider: "openai", Model: "gpt-4o"},
				},
			},
			Agents: config.AgentsConfig{
				Generators: config.AgentRoleConfig{
					Number:     4,
					Iterations: 5,
					Providers: []config.AgentProviderRef{
						{Name: "gemini-direct"},
					},
				},
			},
		}

		router := NewResilientLLMRouter(cfg, nil)
		candidates := router.ResolveCandidatesForRole("generator")

		require.Len(t, candidates, 2)
		assert.Equal(t, "gemini-direct", candidates[0].Name)
		assert.Equal(t, "gemini-2.5-flash", candidates[0].Model)
		assert.Equal(t, "fallback-provider", candidates[1].Name)
	})

	t.Run("Scenario 9: Top-level enable_thinking and thinking_budget fields propagate to Client", func(t *testing.T) {
		enableThinking := true
		thinkingBudget := 8192
		cfg := &config.Config{
			LLM: config.LLMConfig{
				Priority: []string{"qwencloud-thinking"},
				Providers: []config.ProviderSpec{
					{
						Name:           "qwencloud-thinking",
						Provider:       "qwencloud",
						Model:          "qwen3.8-max",
						EnableThinking: &enableThinking,
						ThinkingBudget: &thinkingBudget,
					},
				},
			},
		}

		router := NewResilientLLMRouter(cfg, nil)
		clientObj := router.buildClientForSpec(cfg.LLM.Providers[0], "qwen3.8-max")

		client, ok := clientObj.(*Client)
		require.True(t, ok)
		require.NotNil(t, client.EnableThinking)
		assert.True(t, *client.EnableThinking)
		require.NotNil(t, client.ThinkingBudget)
		assert.Equal(t, 8192, *client.ThinkingBudget)
	})

	t.Run("Scenario 10: Role resolution from agent_role context key overrides global priority", func(t *testing.T) {
		cfg := &config.Config{
			LLM: config.LLMConfig{
				Priority: []string{"opencode-primary", "openrouter-secondary", "qwencloud-tertiary"},
				Providers: []config.ProviderSpec{
					{Name: "qwencloud-tertiary", Provider: "qwencloud", Model: "qwen3.8-max"},
					{Name: "opencode-primary", Provider: "opencode", Model: "glm-5.2"},
					{Name: "openrouter-secondary", Provider: "openrouter", Model: "deepseek-v4"},
				},
			},
			Agents: config.AgentsConfig{
				ProductManager: config.AgentRoleConfig{
					Providers: []config.AgentProviderRef{
						{Name: "qwencloud-tertiary"},
						{Name: "opencode-primary"},
					},
				},
			},
		}

		router := NewResilientLLMRouter(cfg, nil)

		// Context with agent_role = "product_manager"
		ctx := context.WithValue(context.Background(), stringKey("agent_role"), "product_manager")
		role := GetRoleFromContext(ctx)
		assert.Equal(t, "product_manager", role)

		candidates := router.ResolveCandidatesForRole(role)
		require.Len(t, candidates, 3)
		// Agent specific priority takes precedence
		assert.Equal(t, "qwencloud-tertiary", candidates[0].Name)
		assert.Equal(t, "opencode-primary", candidates[1].Name)
		assert.Equal(t, "openrouter-secondary", candidates[2].Name)
	})

	t.Run("Scenario 11: Agent-level provider overrides enable_thinking and thinking_budget", func(t *testing.T) {
		enableFalse := false
		budget1024 := 1024
		cfg := &config.Config{
			LLM: config.LLMConfig{
				Priority: []string{"qwencloud-thinking"},
				Providers: []config.ProviderSpec{
					{
						Name:           "qwencloud-thinking",
						Provider:       "qwencloud",
						Model:          "qwen3.8-max",
						EnableThinking: pointerToBool(true),
						ThinkingBudget: pointerToInt(8192),
					},
				},
			},
			Agents: config.AgentsConfig{
				Generators: config.AgentRoleConfig{
					Providers: []config.AgentProviderRef{
						{
							Name:           "qwencloud-thinking",
							EnableThinking: &enableFalse,
							ThinkingBudget: &budget1024,
						},
					},
				},
			},
		}

		router := NewResilientLLMRouter(cfg, nil)
		candidates := router.ResolveCandidatesForRole("generators")
		require.Len(t, candidates, 1)

		// Overridden spec inside candidate
		assert.Equal(t, "qwencloud-thinking", candidates[0].Name)
	})

	t.Run("Scenario 12: Context role resolution across all role key types and agent roles", func(t *testing.T) {
		rolesToTest := []string{"architect", "planner", "generators", "testers", "fallback", "unblocker", "last_resort"}
		for _, roleName := range rolesToTest {
			ctx1 := context.WithValue(context.Background(), RoleContextKey{}, roleName)
			assert.Equal(t, roleName, GetRoleFromContext(ctx1))

			ctx2 := context.WithValue(context.Background(), stringKey("role"), roleName)
			assert.Equal(t, roleName, GetRoleFromContext(ctx2))

			ctx3 := context.WithValue(context.Background(), stringKey("agent_role"), roleName)
			assert.Equal(t, roleName, GetRoleFromContext(ctx3))
		}
	})

	t.Run("Scenario 13: Fallback Agent with multi-provider prioritized models", func(t *testing.T) {
		cfg := &config.Config{
			LLM: config.LLMConfig{
				Providers: []config.ProviderSpec{
					{Name: "anthropic-deep", Provider: "anthropic", Model: "claude-3-7-sonnet"},
					{Name: "openai-deep", Provider: "openai", Model: "o3-mini"},
					{Name: "deepseek-local", Provider: "deepseek", Model: "deepseek-reasoner"},
				},
			},
			Agents: config.AgentsConfig{
				Fallback: config.FallbackAgentConfig{
					Enabled: true,
					Providers: []config.AgentProviderRef{
						{Name: "anthropic-deep"},
						{Name: "openai-deep"},
						{Name: "deepseek-local"},
					},
				},
			},
		}

		router := NewResilientLLMRouter(cfg, nil)
		candidates := router.ResolveCandidatesForRole("fallback")
		require.Len(t, candidates, 3)

		assert.Equal(t, "anthropic-deep", candidates[0].Name)
		assert.Equal(t, "claude-3-7-sonnet", candidates[0].Model)

		assert.Equal(t, "openai-deep", candidates[1].Name)
		assert.Equal(t, "o3-mini", candidates[1].Model)

		assert.Equal(t, "deepseek-local", candidates[2].Name)
		assert.Equal(t, "deepseek-reasoner", candidates[2].Model)
	})
}

func pointerToBool(b bool) *bool { return &b }
func pointerToInt(i int) *int    { return &i }
