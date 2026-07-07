package llm

import (
	"fmt"
	"strings"
)

var modelHierarchy = map[string][]string{
	"gemini": {
		"gemini-3.5-flash",
		"gemini-3.1-pro-preview",
		"gemini-3.1-flash-lite",
		"gemini-3-pro-preview",
		"gemini-3-flash-preview",
		"gemini-2.5-pro",
		"gemini-2.5-flash",
		"gemini-2.0-flash",
		"gemini-2.0-flash-lite",
	},
	"openai": {
		"gpt-4o",
		"gpt-4o-mini",
	},
	"mistral": {
		"mistral-large-latest",
		"mistral-medium-latest",
		"mistral-small-latest",
		"open-mistral-7b",
	},
	"deepseek": {
		"deepseek-coder",
		"deepseek-chat",
	},
	"hermes": {
		"hermes-3-llama-3.1-405b",
		"hermes-3-llama-3.1-70b",
		"hermes-3-llama-3.1-8b",
	},
	"anthropic": {
		"claude-3-5-sonnet-latest",
		"claude-3-5-haiku-latest",
	},
	"opencode": {
		"glm-5.2",
		"glm-5.1",
		"kimi-k2.7-code",
		"kimi-k2.6",
		"qwen3.7-max",
		"qwen3.7-plus",
		"minimax-m3",
		"minimax-m2.7",
		"qwen3.6-plus",
		"mimo-v2.5-pro",
		"deepseek-v4-pro",
		"mimo-v2.5",
		"deepseek-v4-flash",
	},
}

func normalizeModel(model string) string {
	trimmed := strings.TrimPrefix(strings.ToLower(model), "models/")
	if trimmed == "" {
		return "gemini-2.5-flash"
	}
	return trimmed
}

func resolveGeminiURL(modelInput, apiKey string) string {
	normModel := normalizeModel(modelInput)
	return fmt.Sprintf("https://generativelanguage.googleapis.com/v1beta/models/%s:generateContent?key=%s", normModel, apiKey)
}
