package llm

import (
	"context"
	"crypto/sha256"
	"encoding/hex"

	"github.com/diegojromerolopez/noctifab/pkg/domain"
)

// derivePromptCacheKey returns a stable identifier for grouping related requests
// to maximize prefix cache affinity across LLM providers that support cache keys.
func derivePromptCacheKey(ctx context.Context, prompt string) string {
	if s := domain.CacheSessionID(ctx); s != "" {
		return s
	}
	prefixLen := domain.CacheablePrefixLen(ctx)
	if prefixLen > 0 && prefixLen <= len(prompt) {
		h := sha256.Sum256([]byte(prompt[:prefixLen]))
		return "nf-" + hex.EncodeToString(h[:8])
	}
	return ""
}

// applyProviderCacheParameters injects official API cache parameters and headers
// supported by each respective LLM provider.
func applyProviderCacheParameters(ctx context.Context, provider, model, prompt string, opts *completionOptions) {
	if opts.extraBody == nil {
		opts.extraBody = make(map[string]interface{})
	}
	if opts.extraHeaders == nil {
		opts.extraHeaders = make(map[string]string)
	}

	cacheKey := derivePromptCacheKey(ctx, prompt)

	switch provider {
	case "openai", "mistral", "cerebras":
		// Official prompt_cache_key parameter for OpenAI, Mistral, and Cerebras
		if cacheKey != "" && !globalCapabilityCache.isExtraParamUnsupported(model, "prompt_cache_key") {
			opts.extraBody["prompt_cache_key"] = cacheKey
		}
	case "xai":
		// xAI supports x-grok-conv-id header and prompt_cache_key body parameter
		if cacheKey != "" {
			opts.extraHeaders["x-grok-conv-id"] = cacheKey
			if !globalCapabilityCache.isExtraParamUnsupported(model, "prompt_cache_key") {
				opts.extraBody["prompt_cache_key"] = cacheKey
			}
		}
	case "openrouter":
		// OpenRouter supports x-session-id header, session_id body parameter, and X-OpenRouter-Cache
		if cacheKey != "" {
			opts.extraHeaders["x-session-id"] = cacheKey
			opts.extraBody["session_id"] = cacheKey
		}
		opts.extraHeaders["X-OpenRouter-Cache"] = "true"
	case "fireworks":
		// Fireworks supports x-session-affinity header and user body parameter
		if cacheKey != "" {
			opts.extraHeaders["x-session-affinity"] = cacheKey
			opts.extraBody["user"] = cacheKey
		}
	case "moonshot", "kimi":
		// Moonshot / Kimi supports prompt_cache_options with ttl
		if !globalCapabilityCache.isExtraParamUnsupported(model, "prompt_cache_options") {
			opts.extraBody["prompt_cache_options"] = map[string]string{"ttl": "1h"}
		}
	}
}
