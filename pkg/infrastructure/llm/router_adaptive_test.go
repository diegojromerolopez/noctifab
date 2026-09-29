package llm

import (
	"context"
	"testing"

	"github.com/diegojromerolopez/noctifab/pkg/domain"
	"github.com/diegojromerolopez/noctifab/pkg/infrastructure/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetModelContextWindow(t *testing.T) {
	assert.Equal(t, ContextWindowGemini, GetModelContextWindow("gemini", "gemini-2.0-flash"))
	assert.Equal(t, ContextWindowClaude, GetModelContextWindow("anthropic", "claude-sonnet-5"))
	assert.Equal(t, ContextWindowOpenAI, GetModelContextWindow("openai", "gpt-4o"))
	assert.Equal(t, ContextWindowOpenAI, GetModelContextWindow("openai", "gpt-5.6-luna"))
	assert.Equal(t, ContextWindowDeepSeek, GetModelContextWindow("deepseek", "deepseek-v3"))
	assert.Equal(t, ContextWindowQwen, GetModelContextWindow("qwen", "qwen-2.5-coder-32b"))
	assert.Equal(t, ContextWindowQwenSmall, GetModelContextWindow("qwen", "qwen-max"))
	assert.Equal(t, int64(32_768), GetModelContextWindow("mistral", "codestral"))
	assert.Equal(t, int64(128_000), GetModelContextWindow("mistral", "mistral-large"))
}

func TestFilterAndPrioritizeCandidatesByContext_BypassSmallWindows(t *testing.T) {
	c1 := RouterCandidate{Name: "qwen-compact", Provider: "qwen", Model: "qwen-max", ContextWindow: 32_768}
	c2 := RouterCandidate{Name: "claude-sonnet", Provider: "anthropic", Model: "claude-sonnet-5", ContextWindow: 200_000}
	c3 := RouterCandidate{Name: "gemini-pro", Provider: "gemini", Model: "gemini-2.5-pro", ContextWindow: 1_048_576}

	candidates := []RouterCandidate{c1, c2, c3}

	// 1. Small prompt (5,000 tokens): all candidates fit in priority order
	resSmall := FilterAndPrioritizeCandidatesByContext(candidates, 5000, false)
	require.Len(t, resSmall, 3)
	assert.Equal(t, "qwen-compact", resSmall[0].Name)
	assert.Equal(t, "claude-sonnet", resSmall[1].Name)
	assert.Equal(t, "gemini-pro", resSmall[2].Name)

	// 2. Medium prompt (45,000 tokens): qwen (32k) is bypassed, claude and gemini fit
	resMedium := FilterAndPrioritizeCandidatesByContext(candidates, 45_000, false)
	require.Len(t, resMedium, 2)
	assert.Equal(t, "claude-sonnet", resMedium[0].Name)
	assert.Equal(t, "gemini-pro", resMedium[1].Name)
}

func TestFilterAndPrioritizeCandidatesByContext_AdaptivePriorityBypass(t *testing.T) {
	// Static priority had openai first, then claude, then gemini
	c1 := RouterCandidate{Name: "openai-standard", Provider: "openai", Model: "gpt-4o", ContextWindow: 128_000}
	c2 := RouterCandidate{Name: "claude-sonnet", Provider: "anthropic", Model: "claude-sonnet-5", ContextWindow: 200_000}
	c3 := RouterCandidate{Name: "gemini-pro", Provider: "gemini", Model: "gemini-2.5-pro", ContextWindow: 1_048_576}

	candidates := []RouterCandidate{c1, c2, c3}

	// For a large prompt (75,000 tokens), adaptive routing bypasses static priority
	// and promotes Gemini (1M) and Claude (200k) ahead of OpenAI (128k)
	resAdaptive := FilterAndPrioritizeCandidatesByContext(candidates, 75_000, true)
	require.Len(t, resAdaptive, 3)
	assert.Equal(t, "gemini-pro", resAdaptive[0].Name, "Gemini with 1M capacity should be prioritized first")
	assert.Equal(t, "claude-sonnet", resAdaptive[1].Name, "Claude with 200k capacity should be prioritized second")
	assert.Equal(t, "openai-standard", resAdaptive[2].Name, "OpenAI with 128k capacity should be prioritized third")
}

func TestRouter_AdaptiveContextRouting_Integration(t *testing.T) {
	mockGemini := &mockLLMClient{
		completeFn: func(ctx context.Context, prompt string) (*domain.LLMResponse, error) {
			return &domain.LLMResponse{Reasoning: "Gemini executed", Actions: []domain.LLMAction{{Tool: "write_file"}}}, nil
		},
	}
	mockOpenAI := &mockLLMClient{
		completeFn: func(ctx context.Context, prompt string) (*domain.LLMResponse, error) {
			return &domain.LLMResponse{Reasoning: "OpenAI executed", Actions: []domain.LLMAction{{Tool: "write_file"}}}, nil
		},
	}

	cfg := &config.Config{
		LLM: config.LLMConfig{
			AdaptiveContextRouting: true,
			Priority:               []string{"openai-small", "gemini-big"},
			Providers: []config.ProviderSpec{
				{
					Name:          "openai-small",
					Provider:      "openai",
					Model:         "gpt-4-small",
					ContextWindow: 16_000,
				},
				{
					Name:          "gemini-big",
					Provider:      "gemini",
					Model:         "gemini-3.6-pro",
					ContextWindow: 1_048_576,
				},
			},
		},
	}

	router := NewResilientLLMRouter(cfg, nil)
	router.namedProviders["openai-small"] = config.ProviderSpec{
		Name: "openai-small", Provider: "openai", Model: "gpt-4-small", ContextWindow: 16_000,
	}
	router.namedProviders["gemini-big"] = config.ProviderSpec{
		Name: "gemini-big", Provider: "gemini", Model: "gemini-3.6-pro", ContextWindow: 1_048_576,
	}

	// Inject mock clients
	router.candidateCache["generator"] = []RouterCandidate{
		{Name: "openai-small", Provider: "openai", Model: "gpt-4-small", Client: mockOpenAI, ContextWindow: 16_000},
		{Name: "gemini-big", Provider: "gemini", Model: "gemini-3.6-pro", Client: mockGemini, ContextWindow: 1_048_576},
	}

	// 1. Small prompt: OpenAI is first in priority and selected
	smallPrompt := "short prompt under token limit"
	ctx := context.WithValue(context.Background(), stringKey("agent_role"), "generator")
	resp1, err1 := router.Complete(ctx, smallPrompt)
	require.NoError(t, err1)
	assert.Equal(t, "OpenAI executed", resp1.Reasoning)

	// 2. Large prompt (~25,000 tokens > 16k): OpenAI is bypassed because its context window is 16k,
	// Gemini is selected automatically!
	largePrompt := string(make([]byte, 100_000)) // 100,000 bytes = ~25,000 tokens
	resp2, err2 := router.Complete(ctx, largePrompt)
	require.NoError(t, err2)
	assert.Equal(t, "Gemini executed", resp2.Reasoning)
}
