package llm

import (
	"context"
	"fmt"
	"testing"
	"time"
)

func TestResolveGeminiURL(t *testing.T) {
	tests := []struct {
		name       string
		modelInput string
		apiKey     string
		wantURL    string
	}{
		{
			name:       "empty model input maps to gemini-2.5-pro",
			modelInput: "",
			apiKey:     "testkey",
			wantURL:    "https://generativelanguage.googleapis.com/v1beta/models/gemini-2.5-pro:generateContent?key=testkey",
		},
		{
			name:       "gemini-1.5-pro maps to gemini-2.5-pro",
			modelInput: "gemini-1.5-pro",
			apiKey:     "testkey",
			wantURL:    "https://generativelanguage.googleapis.com/v1beta/models/gemini-2.5-pro:generateContent?key=testkey",
		},
		{
			name:       "models/gemini-1.5-pro maps to gemini-2.5-pro",
			modelInput: "models/gemini-1.5-pro",
			apiKey:     "testkey",
			wantURL:    "https://generativelanguage.googleapis.com/v1beta/models/gemini-2.5-pro:generateContent?key=testkey",
		},
		{
			name:       "gemini-2.5-flash remains gemini-2.5-flash",
			modelInput: "gemini-2.5-flash",
			apiKey:     "testkey",
			wantURL:    "https://generativelanguage.googleapis.com/v1beta/models/gemini-2.5-flash:generateContent?key=testkey",
		},
		{
			name:       "models/gemini-2.5-flash remains gemini-2.5-flash",
			modelInput: "models/gemini-2.5-flash",
			apiKey:     "testkey",
			wantURL:    "https://generativelanguage.googleapis.com/v1beta/models/gemini-2.5-flash:generateContent?key=testkey",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotURL := resolveGeminiURL(tt.modelInput, tt.apiKey)
			if gotURL != tt.wantURL {
				t.Errorf("resolveGeminiURL(%q, %q) = %q; want %q", tt.modelInput, tt.apiKey, gotURL, tt.wantURL)
			}
		})
	}
}

func TestGetNextLowerModel(t *testing.T) {
	tests := []struct {
		name         string
		provider     string
		currentModel string
		wantModel    string
	}{
		{
			name:         "gemini-2.5-pro falls back to gemini-2.5-flash",
			provider:     "gemini",
			currentModel: "gemini-2.5-pro",
			wantModel:    "gemini-2.5-flash",
		},
		{
			name:         "models/gemini-2.5-pro falls back to gemini-2.5-flash",
			provider:     "Gemini",
			currentModel: "models/gemini-2.5-pro",
			wantModel:    "gemini-2.5-flash",
		},
		{
			name:         "gemini-1.5-pro falls back to gemini-2.5-flash (legacy mapping)",
			provider:     "gemini",
			currentModel: "gemini-1.5-pro",
			wantModel:    "gemini-2.5-flash",
		},
		{
			name:         "gemini-2.5-flash is lowest and returns empty",
			provider:     "gemini",
			currentModel: "gemini-2.5-flash",
			wantModel:    "",
		},
		{
			name:         "openai gpt-4o falls back to gpt-4o-mini",
			provider:     "openai",
			currentModel: "gpt-4o",
			wantModel:    "gpt-4o-mini",
		},
		{
			name:         "openai gpt-4o-mini is lowest and returns empty",
			provider:     "openai",
			currentModel: "gpt-4o-mini",
			wantModel:    "",
		},
		{
			name:         "mistral-large falls back to mistral-medium",
			provider:     "mistral",
			currentModel: "mistral-large-latest",
			wantModel:    "mistral-medium-latest",
		},
		{
			name:         "mistral-small falls back to open-mistral-7b",
			provider:     "mistral",
			currentModel: "mistral-small-latest",
			wantModel:    "open-mistral-7b",
		},
		{
			name:         "deepseek-coder falls back to deepseek-chat",
			provider:     "deepseek",
			currentModel: "deepseek-coder",
			wantModel:    "deepseek-chat",
		},
		{
			name:         "deepseek-chat is lowest and returns empty",
			provider:     "deepseek",
			currentModel: "deepseek-chat",
			wantModel:    "",
		},
		{
			name:         "hermes 405b falls back to hermes 70b",
			provider:     "hermes",
			currentModel: "hermes-3-llama-3.1-405b",
			wantModel:    "hermes-3-llama-3.1-70b",
		},
		{
			name:         "anthropic claude-sonnet falls back to claude-haiku",
			provider:     "anthropic",
			currentModel: "claude-3-5-sonnet-latest",
			wantModel:    "claude-3-5-haiku-latest",
		},
		{
			name:         "anthropic claude-haiku is lowest and returns empty",
			provider:     "anthropic",
			currentModel: "claude-3-5-haiku-latest",
			wantModel:    "",
		},
		{
			name:         "unknown provider returns empty",
			provider:     "unknown",
			currentModel: "gpt-4o",
			wantModel:    "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := &Client{Provider: tt.provider, Model: tt.currentModel}
			got := c.getNextLowerModel(context.Background(), "mockkey")
			if got != tt.wantModel {
				t.Errorf("getNextLowerModel(%q, %q) = %q; want %q", tt.provider, tt.currentModel, got, tt.wantModel)
			}
		})
	}
}

func TestParseRetryDelay(t *testing.T) {
	tests := []struct {
		name      string
		errString string
		wantDelay time.Duration
		wantOk    bool
	}{
		{
			name:      "no json braces",
			errString: "HTTP error 429: Rate limit exceeded",
			wantDelay: 0,
			wantOk:    false,
		},
		{
			name:      "invalid json",
			errString: "HTTP error 429: {invalid json",
			wantDelay: 0,
			wantOk:    false,
		},
		{
			name: "valid google rate limit with s suffix",
			errString: `HTTP error 429: {
				"error": {
					"code": 429,
					"message": "...",
					"status": "RESOURCE_EXHAUSTED",
					"details": [
						{
							"@type": "type.googleapis.com/google.rpc.RetryInfo",
							"retryDelay": "25323s"
						}
					]
				}
			}`,
			wantDelay: 25323 * time.Second,
			wantOk:    true,
		},
		{
			name: "valid google rate limit numeric",
			errString: `HTTP error 429: {
				"error": {
					"code": 429,
					"message": "...",
					"status": "RESOURCE_EXHAUSTED",
					"details": [
						{
							"@type": "type.googleapis.com/google.rpc.RetryInfo",
							"retryDelay": "12.5"
						}
					]
				}
			}`,
			wantDelay: 12500 * time.Millisecond,
			wantOk:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := fmt.Errorf("%s", tt.errString)
			gotDelay, gotOk := parseRetryDelay(err)
			if gotOk != tt.wantOk {
				t.Fatalf("parseRetryDelay() ok = %v; want %v", gotOk, tt.wantOk)
			}
			if gotOk && gotDelay != tt.wantDelay {
				t.Errorf("parseRetryDelay() delay = %v; want %v", gotDelay, tt.wantDelay)
			}
		})
	}
}
