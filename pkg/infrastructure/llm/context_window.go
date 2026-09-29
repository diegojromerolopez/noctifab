package llm

import (
	"strings"
)

// Default context window token limits across major provider model families.
const (
	ContextWindowGemini    int64 = 1_048_576 // 1M tokens (up to 2M)
	ContextWindowClaude    int64 = 200_000   // 200k tokens
	ContextWindowOpenAI    int64 = 128_000   // 128k tokens for modern GPT-4o / GPT-5 / o1 / o3
	ContextWindowDeepSeek  int64 = 128_000   // 128k tokens
	ContextWindowQwen      int64 = 128_000   // 128k tokens (e.g. Qwen 2.5 Coder 128k)
	ContextWindowQwenSmall int64 = 32_768    // 32k tokens (older or compact tiers)
	ContextWindowStandard  int64 = 128_000   // Fallback standard frontier window
	ContextWindowCompact   int64 = 32_768    // Compact / small model window
)

// GetModelContextWindow returns the estimated maximum token capacity for a given provider and model.
func GetModelContextWindow(provider, model string) int64 {
	p := strings.ToLower(strings.TrimSpace(provider))
	m := strings.ToLower(strings.TrimSpace(model))

	switch {
	case strings.Contains(p, "gemini") || strings.Contains(m, "gemini"):
		return ContextWindowGemini

	case strings.Contains(p, "claude") || strings.Contains(p, "anthropic") || strings.Contains(m, "claude"):
		return ContextWindowClaude

	case strings.Contains(p, "openai") || strings.Contains(m, "gpt") || strings.Contains(m, "o1") || strings.Contains(m, "o3") || strings.Contains(m, "luna"):
		if strings.Contains(m, "3.5") || strings.Contains(m, "gpt-4-0613") {
			return 16_384
		}
		return ContextWindowOpenAI

	case strings.Contains(p, "deepseek") || strings.Contains(m, "deepseek"):
		return ContextWindowDeepSeek

	case strings.Contains(p, "qwen") || strings.Contains(m, "qwen"):
		if strings.Contains(m, "coder") || strings.Contains(m, "2.5") {
			return ContextWindowQwen
		}
		return ContextWindowQwenSmall

	case strings.Contains(p, "mistral") || strings.Contains(m, "mistral") || strings.Contains(m, "codestral"):
		if strings.Contains(m, "large") {
			return 128_000
		}
		return 32_768

	case strings.Contains(p, "groq") || strings.Contains(p, "together") || strings.Contains(p, "fireworks") || strings.Contains(p, "cerebras") || strings.Contains(p, "openrouter") || strings.Contains(p, "opencode"):
		if strings.Contains(m, "claude") {
			return ContextWindowClaude
		}
		if strings.Contains(m, "gemini") {
			return ContextWindowGemini
		}
		if strings.Contains(m, "llama-3.1") || strings.Contains(m, "llama-3.3") || strings.Contains(m, "glm-5") || strings.Contains(m, "qwen") {
			return 128_000
		}
		return ContextWindowStandard

	default:
		return ContextWindowStandard
	}
}
