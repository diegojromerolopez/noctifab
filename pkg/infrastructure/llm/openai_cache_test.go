package llm

import (
	"context"
	"testing"

	"github.com/diegojromerolopez/noctifab/pkg/domain"
	"github.com/stretchr/testify/assert"
)

func TestDerivePromptCacheKey(t *testing.T) {
	t.Run("returns session ID when present in context", func(t *testing.T) {
		ctx := domain.WithCacheSessionID(context.Background(), "session-42")
		key := derivePromptCacheKey(ctx, "sample prompt")
		assert.Equal(t, "session-42", key)
	})

	t.Run("returns hash of cacheable prefix when present", func(t *testing.T) {
		prompt := "SYSTEM INSTRUCTIONS AND BASE PROMPT\nDynamic user task"
		ctx := domain.WithCacheablePrefix(context.Background(), len("SYSTEM INSTRUCTIONS AND BASE PROMPT"))
		key := derivePromptCacheKey(ctx, prompt)
		assert.NotEmpty(t, key)
		assert.True(t, len(key) >= 10)
		assert.Equal(t, "nf-", key[:3])

		// Identical prefix must yield identical key
		prompt2 := "SYSTEM INSTRUCTIONS AND BASE PROMPT\nDifferent user task"
		ctx2 := domain.WithCacheablePrefix(context.Background(), len("SYSTEM INSTRUCTIONS AND BASE PROMPT"))
		key2 := derivePromptCacheKey(ctx2, prompt2)
		assert.Equal(t, key, key2)
	})

	t.Run("returns empty string when no session ID or prefix", func(t *testing.T) {
		key := derivePromptCacheKey(context.Background(), "no prefix prompt")
		assert.Empty(t, key)
	})
}

func TestApplyProviderCacheParameters(t *testing.T) {
	ctx := domain.WithCacheSessionID(context.Background(), "task-story-101")
	prompt := "Hello, this is a prompt."

	t.Run("openai injects prompt_cache_key", func(t *testing.T) {
		opts := completionOptions{}
		applyProviderCacheParameters(ctx, "openai", "gpt-4o", prompt, &opts)
		assert.Equal(t, "task-story-101", opts.extraBody["prompt_cache_key"])
	})

	t.Run("mistral injects prompt_cache_key", func(t *testing.T) {
		opts := completionOptions{}
		applyProviderCacheParameters(ctx, "mistral", "mistral-large-latest", prompt, &opts)
		assert.Equal(t, "task-story-101", opts.extraBody["prompt_cache_key"])
	})

	t.Run("cerebras injects prompt_cache_key", func(t *testing.T) {
		opts := completionOptions{}
		applyProviderCacheParameters(ctx, "cerebras", "llama3.1-70b", prompt, &opts)
		assert.Equal(t, "task-story-101", opts.extraBody["prompt_cache_key"])
	})

	t.Run("xai injects x-grok-conv-id and prompt_cache_key", func(t *testing.T) {
		opts := completionOptions{}
		applyProviderCacheParameters(ctx, "xai", "grok-2", prompt, &opts)
		assert.Equal(t, "task-story-101", opts.extraHeaders["x-grok-conv-id"])
		assert.Equal(t, "task-story-101", opts.extraBody["prompt_cache_key"])
	})

	t.Run("openrouter injects x-session-id, session_id and X-OpenRouter-Cache", func(t *testing.T) {
		opts := completionOptions{}
		applyProviderCacheParameters(ctx, "openrouter", "anthropic/claude-3.5-sonnet", prompt, &opts)
		assert.Equal(t, "task-story-101", opts.extraHeaders["x-session-id"])
		assert.Equal(t, "task-story-101", opts.extraBody["session_id"])
		assert.Equal(t, "true", opts.extraHeaders["X-OpenRouter-Cache"])
	})

	t.Run("fireworks injects x-session-affinity and user", func(t *testing.T) {
		opts := completionOptions{}
		applyProviderCacheParameters(ctx, "fireworks", "accounts/fireworks/models/llama-v3p1-70b-instruct", prompt, &opts)
		assert.Equal(t, "task-story-101", opts.extraHeaders["x-session-affinity"])
		assert.Equal(t, "task-story-101", opts.extraBody["user"])
	})

	t.Run("moonshot/kimi injects prompt_cache_options", func(t *testing.T) {
		opts := completionOptions{}
		applyProviderCacheParameters(ctx, "moonshot", "kimi-k3", prompt, &opts)
		assert.Equal(t, map[string]string{"ttl": "1h"}, opts.extraBody["prompt_cache_options"])

		optsKimi := completionOptions{}
		applyProviderCacheParameters(ctx, "kimi", "kimi-k3", prompt, &optsKimi)
		assert.Equal(t, map[string]string{"ttl": "1h"}, optsKimi.extraBody["prompt_cache_options"])
	})

	t.Run("suppressed when marked unsupported in capability cache", func(t *testing.T) {
		model := "unsupported-model"
		globalCapabilityCache.markExtraParamUnsupported(model, "prompt_cache_key")
		opts := completionOptions{}
		applyProviderCacheParameters(ctx, "openai", model, prompt, &opts)
		assert.Nil(t, opts.extraBody["prompt_cache_key"])
	})
}
