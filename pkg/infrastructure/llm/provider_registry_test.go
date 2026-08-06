package llm

import (
	"testing"

	"github.com/diegojromerolopez/noctifab/pkg/infrastructure/config"
)

func TestProviderRegistry(t *testing.T) {
	t.Run("GetProviderSpec standard providers", func(t *testing.T) {
		providers := []string{
			"openai", "anthropic", "gemini", "opencode", "kimi", "moonshot",
			"groq", "openrouter", "qwen", "dashscope", "together", "llama", "meta",
			"huggingface", "mistral", "deepseek", "hermes", "ollama",
			"xai", "grok", "perplexity", "fireworks", "sambanova", "cohere",
			"cerebras", "nvidia", "ai21", "upstage",
		}

		for _, p := range providers {
			spec, ok := GetProviderSpec(p)
			if !ok || spec == nil {
				t.Errorf("expected spec for provider %s, got nil/not found", p)
				continue
			}
			if spec.NewClientFunc == nil {
				t.Errorf("expected NewClientFunc for provider %s, got nil", p)
			}
		}
	})

	t.Run("every registered provider is accepted by config validation", func(t *testing.T) {
		for name, spec := range RegistrySnapshot() {
			if spec == nil || spec.Name == "" {
				continue
			}
			if !config.IsValidLLMProvider(name) {
				t.Errorf("registered provider %q is rejected by config.IsValidLLMProvider; add it to validLLMProviders in pkg/infrastructure/config/config.go", name)
			}
		}
	})

	t.Run("RegisterProvider custom provider", func(t *testing.T) {
		custom := &ProviderSpec{
			Name:           "custom-llm",
			BaseURL:        "https://api.custom.ai/v1",
			EnvKeys:        []string{"CUSTOM_LLM_KEY"},
			ParseModelFunc: parseOpenAIModel,
			Protocol:       "openai",
		}
		RegisterProvider(custom)

		spec, ok := GetProviderSpec("CUSTOM-LLM")
		if !ok || spec.BaseURL != "https://api.custom.ai/v1" {
			t.Errorf("unexpected custom spec: %+v", spec)
		}
	})
}
