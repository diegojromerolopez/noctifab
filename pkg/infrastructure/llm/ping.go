package llm

import (
	"context"
	"fmt"
	"strings"
	"time"
)

// Ping verifies that the configured LLM provider is reachable and the API key
// has at least model-listing access. It constructs the same ProviderClient the
// would use for completions and calls GetAvailableModels with a short timeout.
//
// Ping does NOT verify that the configured model is available under the
// caller's plan (a quota/model-availability failure surfaces only on the first
// real completion). It only verifies: provider name is recognized, base URL is
// reachable, and the API key authenticates against the models endpoint.
//
// Returns nil on success, or a descriptive error on failure (auth, network,
// unknown provider).
func Ping(ctx context.Context, provider, apiKey, url string) error {
	pClient, err := newProviderClientForPing(provider, url)
	if err != nil {
		return fmt.Errorf("unsupported LLM provider: %s", provider)
	}

	pingCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()

	if _, err := pClient.GetAvailableModels(pingCtx, apiKey); err != nil {
		return classifyPingError(provider, err)
	}
	return nil
}

// newProviderClientForPing mirrors the dispatch in client.go but returns an
// error for unknown providers instead of falling through to a default. The
// returned ProviderClient is scoped to a single ping call.
func newProviderClientForPing(provider, url string) (ProviderClient, error) {
	switch strings.ToLower(provider) {
	case "openai", "hermes", "huggingface", "mistral", "deepseek", "ollama", "opencode":
		return NewOpenAIProviderClient(provider, url, 15*time.Second, 15*time.Second, false), nil
	case "gemini":
		return NewGeminiProviderClient(url, 15*time.Second, 15*time.Second, false), nil
	case "anthropic":
		return NewAnthropicProviderClient(url, 15*time.Second, 15*time.Second, false), nil
	default:
		return nil, fmt.Errorf("unsupported LLM provider: %s", provider)
	}
}

// classifyPingError translates a raw transport error into a human-readable
// failure category so the pre-flight output tells the operator the likely
// cause (bad key vs quota vs network) instead of a raw HTTP dump.
func classifyPingError(provider string, err error) error {
	msg := err.Error()
	switch {
	case strings.Contains(msg, "401") || strings.Contains(msg, "Unauthorized") || strings.Contains(msg, "invalid_api_key") || strings.Contains(msg, "Authentication Fails"):
		return fmt.Errorf("LLM provider %s rejected the API key (HTTP 401). Check that api_key/api_key_env points to a valid key for the configured provider", provider)
	case strings.Contains(msg, "403") || strings.Contains(msg, "Forbidden"):
		return fmt.Errorf("LLM provider %s returned 403 Forbidden. The key may lack model-listing scope or be rate-limited; verify key permissions at the provider dashboard", provider)
	case strings.Contains(msg, "429") || strings.Contains(msg, "RESOURCE_EXHAUSTED") || strings.Contains(msg, "Quota") || strings.Contains(msg, "quota"):
		return fmt.Errorf("LLM provider %s returned 429 quota exhausted during ping. Retry later or upgrade the plan", provider)
	case strings.Contains(msg, "no such host") || strings.Contains(msg, "timeout") || strings.Contains(msg, "deadline exceeded") || strings.Contains(msg, "connection refused") || strings.Contains(msg, "i/o timeout"):
		return fmt.Errorf("LLM provider %s unreachable: %v. Check network connectivity and the configured url override", provider, err)
	default:
		return fmt.Errorf("LLM provider %s ping failed: %v", provider, err)
	}
}
