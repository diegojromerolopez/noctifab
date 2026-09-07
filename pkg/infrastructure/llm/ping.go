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
// Returns latency duration and nil on success, or a descriptive error on failure
// (auth, network, unknown provider).
func Ping(ctx context.Context, provider, apiKey, url string) (time.Duration, error) {
	latency, _, err := PingAndResolveModel(ctx, provider, apiKey, url, "")
	return latency, err
}

// PingAndResolveModel verifies provider connectivity and validates the configured model against /models.
// If the configured model is missing, invalid, or empty, it dynamically resolves and returns the
// best (highest-ranked) available non-blacklisted model from the provider catalog.
func PingAndResolveModel(ctx context.Context, provider, apiKey, url, configuredModel string) (time.Duration, string, error) {
	pClient, err := newProviderClientForPing(provider, url)
	if err != nil {
		return 0, "", fmt.Errorf("unsupported LLM provider: %s", provider)
	}

	pingCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()

	start := time.Now()
	available, err := pClient.GetAvailableModels(pingCtx, apiKey)
	if err != nil {
		return 0, "", classifyPingError(provider, err)
	}
	latency := time.Since(start)

	if len(available) == 0 {
		return latency, configuredModel, nil
	}

	spec, _ := GetProviderSpec(strings.ToLower(provider))
	parser := parseOpenAIModel
	if spec != nil && spec.ParseModelFunc != nil {
		parser = spec.ParseModelFunc
	}
	var parsedModels []*ProviderModelInfo
	for _, m := range available {
		if info, parsed := parser(m); parsed && info != nil {
			parsedModels = append(parsedModels, info)
		}
	}

	if len(parsedModels) == 0 {
		return latency, configuredModel, nil
	}

	sortProviderModels(parsedModels)
	normConfigured := normalizeModelName(configuredModel)
	if normConfigured == "" || normConfigured == "auto" || normConfigured == "latest" {
		return latency, parsedModels[0].Name, nil
	}

	// Check if configured model exists exactly or by prefix
	for _, m := range parsedModels {
		if normalizeModelName(m.Name) == normConfigured && !IsModelBlacklisted(m.Name) {
			return latency, m.Name, nil
		}
	}

	// Model not found in /models: select best available fallback model
	bestFallback := selectFallbackModel(configuredModel, configuredModel, parsedModels)
	if bestFallback != "" {
		return latency, bestFallback, nil
	}

	return latency, parsedModels[0].Name, nil
}

// newProviderClientForPing mirrors the dispatch in client.go but returns an
// error for unknown providers instead of falling through to a default. The
// returned ProviderClient is scoped to a single ping call.
func newProviderClientForPing(provider, url string) (ProviderClient, error) {
	if spec, ok := GetProviderSpec(provider); ok && spec.NewClientFunc != nil {
		return spec.NewClientFunc(url, 15*time.Second, 15*time.Second, false), nil
	}
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
		return fmt.Errorf("LLM provider %s rejected the API key (HTTP 401). Check that api_keys points to a valid key for the configured provider", provider)
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
