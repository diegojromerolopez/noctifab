package llm

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestProvidersLatestAliasResolution(t *testing.T) {
	tests := []struct {
		provider      string
		mockBody      string
		expectedModel string
	}{
		{
			provider: "openai",
			mockBody: `{
				"data": [
					{"id": "text-embedding-3-small"},
					{"id": "gpt-3.5-turbo"},
					{"id": "gpt-4"},
					{"id": "gpt-4o-mini"},
					{"id": "gpt-4o"},
					{"id": "o1-mini"},
					{"id": "o3-mini"}
				]
			}`,
			expectedModel: "gpt-4o",
		},
		{
			provider: "anthropic",
			mockBody: `{
				"data": [
					{"id": "claude-3-opus-20240229"},
					{"id": "claude-3-5-haiku-20241022"},
					{"id": "claude-3-5-sonnet-20241022"}
				]
			}`,
			expectedModel: "claude-3-opus-20240229",
		},
		{
			provider: "gemini",
			mockBody: `{
				"models": [
					{"name": "models/gemini-embed-text-001", "supportedGenerationMethods": ["embedContent"]},
					{"name": "models/gemini-robotics-er-2-preview", "supportedGenerationMethods": ["generateContent"]},
					{"name": "models/gemini-1.5-flash", "supportedGenerationMethods": ["generateContent"]},
					{"name": "models/gemini-2.0-flash", "supportedGenerationMethods": ["generateContent"]},
					{"name": "models/gemini-2.5-flash", "supportedGenerationMethods": ["generateContent"]}
				]
			}`,
			expectedModel: "gemini-2.5-flash",
		},
		{
			provider: "mistral",
			mockBody: `{
				"data": [
					{"id": "mistral-embed"},
					{"id": "mistral-small-latest"},
					{"id": "mistral-medium-latest"},
					{"id": "mistral-large-latest"}
				]
			}`,
			expectedModel: "mistral-large-latest",
		},
		{
			provider: "deepseek",
			mockBody: `{
				"data": [
					{"id": "deepseek-chat"},
					{"id": "deepseek-coder"},
					{"id": "deepseek-reasoner"}
				]
			}`,
			expectedModel: "deepseek-coder",
		},
		{
			provider: "hermes",
			mockBody: `{
				"data": [
					{"id": "hermes-3-llama-3.1-8b"},
					{"id": "hermes-3-llama-3.1-70b"},
					{"id": "hermes-3-llama-3.1-405b"}
				]
			}`,
			expectedModel: "hermes-3-llama-3.1-405b",
		},
		{
			provider: "qwen",
			mockBody: `{
				"data": [
					{"id": "qwen-turbo"},
					{"id": "qwen-plus"},
					{"id": "qwen-max"}
				]
			}`,
			expectedModel: "qwen-max",
		},
		{
			provider: "llama",
			mockBody: `{
				"data": [
					{"id": "Llama-3.1-8B-Instruct"},
					{"id": "Llama-3.1-70B-Instruct"},
					{"id": "Llama-3.3-70B-Instruct"},
					{"id": "Llama-3.1-405B-Instruct"}
				]
			}`,
			expectedModel: "Llama-3.1-405B-Instruct",
		},
		{
			provider: "xai",
			mockBody: `{
				"data": [
					{"id": "grok-2-mini"},
					{"id": "grok-2"},
					{"id": "grok-3"}
				]
			}`,
			expectedModel: "grok-3",
		},
		{
			provider: "perplexity",
			mockBody: `{
				"data": [
					{"id": "sonar-small"},
					{"id": "sonar-pro"},
					{"id": "sonar-deep-research"}
				]
			}`,
			expectedModel: "sonar-deep-research",
		},
		{
			provider: "kimi",
			mockBody: `{
				"data": [
					{"id": "kimi-k1.5"},
					{"id": "kimi-k2.7"},
					{"id": "kimi-k3"}
				]
			}`,
			expectedModel: "kimi-k3",
		},
		{
			provider: "opencode",
			mockBody: `{
				"data": [
					{"id": "glm-4-flash"},
					{"id": "glm-4"},
					{"id": "glm-5.2"}
				]
			}`,
			expectedModel: "glm-5.2",
		},
	}

	for _, tt := range tests {
		t.Run(tt.provider, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte(tt.mockBody))
			}))
			defer server.Close()

			c := &Client{
				Provider: tt.provider,
				Model:    "latest",
				URL:      server.URL,
				APIKey:   "mock-api-key",
			}

			resolved := c.resolveLatestModel(context.Background(), "mock-api-key")
			if resolved != tt.expectedModel {
				t.Errorf("provider %s: expected resolved model %q, got %q", tt.provider, tt.expectedModel, resolved)
			}
		})
	}
}

// TestResolveLatestModel_UsesExactAliasWhenPresent is a regression test for
// OpenRouter-style pinned models whose official name already ends in
// `-latest` (e.g. `~deepseek/deepseek-v4-flash-latest`). resolveLatestModel
// must NOT select the `~`-prefixed moving alias (it routes to variable
// upstreams); it must resolve to a concrete pinned model in the same family
// instead.
func TestResolveLatestModel_UsesExactAliasWhenPresent(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{
			"data": [
				{"id": "sao10k/l3-lunaris-8b"},
				{"id": "deepseek/deepseek-v4-flash-0731"},
				{"id": "~deepseek/deepseek-v4-flash-latest"},
				{"id": "deepseek/deepseek-v4-flash"}
			]
		}`))
	}))
	defer server.Close()

	c := &Client{
		Provider: "openrouter",
		Model:    "deepseek/deepseek-v4-flash-latest",
		URL:      server.URL,
		APIKey:   "mock-api-key",
	}

	resolved := c.resolveLatestModel(context.Background(), "mock-api-key")
	if resolved != "deepseek/deepseek-v4-flash-0731" {
		t.Errorf("expected dated snapshot %q (not the ~ alias or bare base), got %q", "deepseek/deepseek-v4-flash-0731", resolved)
	}
}

// TestResolveLatestModel_AliasAbsentFallsBackToFamily ensures that when the
// exact alias is not in the catalog, resolution stays within the alias's model
// family rather than jumping to an unrelated top-ranked model.
func TestResolveLatestModel_AliasAbsentFallsBackToFamily(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{
			"data": [
				{"id": "sao10k/l3-lunaris-8b"},
				{"id": "deepseek/deepseek-v4-flash-0731"},
				{"id": "deepseek/deepseek-v4-flash"}
			]
		}`))
	}))
	defer server.Close()

	c := &Client{
		Provider: "openrouter",
		Model:    "deepseek/deepseek-v4-flash-latest",
		URL:      server.URL,
		APIKey:   "mock-api-key",
	}

	resolved := c.resolveLatestModel(context.Background(), "mock-api-key")
	if resolved != "deepseek/deepseek-v4-flash-0731" {
		t.Errorf("expected family model %q, got %q", "deepseek/deepseek-v4-flash-0731", resolved)
	}
}
