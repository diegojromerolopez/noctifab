package llm

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
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

// TestComplete_ExplicitLatestModelPassedThrough ensures that model names ending
// in `-latest` (e.g. `claude-3-7-sonnet-latest`) are passed directly to the provider
// without triggering dynamic alias resolution or /models queries.
func TestComplete_ExplicitLatestModelPassedThrough(t *testing.T) {
	srv, modelCalls := latestModelServer(t)

	c := &Client{
		Provider:              "opencode",
		Model:                 "glm-5.2-latest",
		APIKey:                "testkey",
		URL:                   srv.URL,
		Timeout:               2 * time.Second,
		IdleTimeout:           2 * time.Second,
		Streaming:             false,
		SkipOnCreditExhausted: true,
	}

	resp, err := c.Complete(context.Background(), "test prompt")
	if err != nil {
		t.Fatalf("Complete failed: %v", err)
	}
	if resp == nil {
		t.Fatal("expected non-nil response")
	}

	// No /models endpoint call should have been made because the model was not "latest", "auto", or "".
	if *modelCalls != 0 {
		t.Errorf("expected 0 calls to /models for explicit model %q, got %d", c.Model, *modelCalls)
	}
}

// TestResolveLatestModel_AutoAndEmptyAliases verifies that "auto" and empty string
// trigger dynamic catalog resolution to the top-ranked model.
func TestResolveLatestModel_AutoAndEmptyAliases(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{
			"data": [
				{"id": "gpt-3.5-turbo"},
				{"id": "gpt-4o"},
				{"id": "gpt-4o-mini"}
			]
		}`))
	}))
	defer server.Close()

	for _, alias := range []string{"auto", ""} {
		t.Run("alias_"+alias, func(t *testing.T) {
			c := &Client{
				Provider: "openai",
				Model:    alias,
				URL:      server.URL,
				APIKey:   "mock-api-key",
			}
			resolved := c.resolveLatestModel(context.Background(), "mock-api-key")
			if resolved != "gpt-4o" {
				t.Errorf("alias %q: expected %q, got %q", alias, "gpt-4o", resolved)
			}
		})
	}
}
