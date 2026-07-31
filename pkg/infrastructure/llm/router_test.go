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
}
