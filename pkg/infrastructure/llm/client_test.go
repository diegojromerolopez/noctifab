package llm

import (
	"context"
	"fmt"
	"net/http"
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
			name:       "empty model input maps to gemini-2.5-flash",
			modelInput: "",
			apiKey:     "testkey",
			wantURL:    "https://generativelanguage.googleapis.com/v1beta/models/gemini-2.5-flash:generateContent?key=testkey",
		},
		{
			name:       "gemini-1.5-pro remains gemini-1.5-pro",
			modelInput: "gemini-1.5-pro",
			apiKey:     "testkey",
			wantURL:    "https://generativelanguage.googleapis.com/v1beta/models/gemini-1.5-pro:generateContent?key=testkey",
		},
		{
			name:       "models/gemini-1.5-pro remains gemini-1.5-pro",
			modelInput: "models/gemini-1.5-pro",
			apiKey:     "testkey",
			wantURL:    "https://generativelanguage.googleapis.com/v1beta/models/gemini-1.5-pro:generateContent?key=testkey",
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
			name:         "gemini-1.5-pro falls back to gemini-1.5-flash",
			provider:     "gemini",
			currentModel: "gemini-1.5-pro",
			wantModel:    "gemini-1.5-flash",
		},
		{
			name:         "gemini-2.5-flash falls back to gemini-1.5-pro",
			provider:     "gemini",
			currentModel: "gemini-2.5-flash",
			wantModel:    "gemini-1.5-pro",
		},
		{
			name:         "gemini-1.5-flash is lowest and returns empty",
			provider:     "gemini",
			currentModel: "gemini-1.5-flash",
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
	t.Run("no json braces plain error", func(t *testing.T) {
		err := fmt.Errorf("HTTP error 429: Rate limit exceeded")
		_, ok := parseRetryDelay(err)
		if ok {
			t.Fatal("expected ok=false for plain error with no JSON body")
		}
	})

	t.Run("invalid json body", func(t *testing.T) {
		err := fmt.Errorf("HTTP error 429: {invalid json")
		_, ok := parseRetryDelay(err)
		if ok {
			t.Fatal("expected ok=false for invalid JSON body")
		}
	})

	t.Run("gemini retryDelay with duration suffix", func(t *testing.T) {
		body := `{"error":{"code":429,"status":"RESOURCE_EXHAUSTED","details":[{"@type":"type.googleapis.com/google.rpc.RetryInfo","retryDelay":"25323s"}]}}`
		err := &httpError{StatusCode: 429, Body: body, Header: http.Header{}}
		d, ok := parseRetryDelay(err)
		if !ok {
			t.Fatal("expected ok=true for Gemini retryDelay")
		}
		if d != 25323*time.Second {
			t.Errorf("expected 25323s, got %v", d)
		}
	})

	t.Run("gemini retryDelay numeric seconds", func(t *testing.T) {
		body := `{"error":{"code":429,"status":"RESOURCE_EXHAUSTED","details":[{"@type":"type.googleapis.com/google.rpc.RetryInfo","retryDelay":"12.5"}]}}`
		err := &httpError{StatusCode: 429, Body: body, Header: http.Header{}}
		d, ok := parseRetryDelay(err)
		if !ok {
			t.Fatal("expected ok=true for Gemini numeric retryDelay")
		}
		if d != 12500*time.Millisecond {
			t.Errorf("expected 12.5s, got %v", d)
		}
	})

	t.Run("gemini retryDelay complex duration (hours+minutes+seconds)", func(t *testing.T) {
		body := `{"error":{"code":429,"status":"RESOURCE_EXHAUSTED","details":[{"@type":"type.googleapis.com/google.rpc.RetryInfo","retryDelay":"7h2m3s"}]}}`
		err := &httpError{StatusCode: 429, Body: body, Header: http.Header{}}
		d, ok := parseRetryDelay(err)
		if !ok {
			t.Fatal("expected ok=true for Gemini complex retryDelay")
		}
		want := 7*time.Hour + 2*time.Minute + 3*time.Second
		if d != want {
			t.Errorf("expected %v, got %v", want, d)
		}
	})

	t.Run("Retry-After header integer seconds (OpenAI/Anthropic/Mistral/DeepSeek)", func(t *testing.T) {
		h := http.Header{}
		h.Set("Retry-After", "60")
		err := &httpError{StatusCode: 429, Body: `{"error":{"type":"rate_limit_error"}}`, Header: h}
		d, ok := parseRetryDelay(err)
		if !ok {
			t.Fatal("expected ok=true for Retry-After header")
		}
		if d != 60*time.Second {
			t.Errorf("expected 60s, got %v", d)
		}
	})

	t.Run("Retry-After header fractional seconds", func(t *testing.T) {
		h := http.Header{}
		h.Set("Retry-After", "33.5")
		err := &httpError{StatusCode: 429, Body: `{}`, Header: h}
		d, ok := parseRetryDelay(err)
		if !ok {
			t.Fatal("expected ok=true for fractional Retry-After header")
		}
		if d != 33500*time.Millisecond {
			t.Errorf("expected 33.5s, got %v", d)
		}
	})

	t.Run("HuggingFace ratelimit header t= field", func(t *testing.T) {
		h := http.Header{}
		h.Set("ratelimit", `"api";r=0;t=55`)
		err := &httpError{StatusCode: 429, Body: `{"error":"Rate limit reached."}`, Header: h}
		d, ok := parseRetryDelay(err)
		if !ok {
			t.Fatal("expected ok=true for HuggingFace ratelimit header")
		}
		if d != 55*time.Second {
			t.Errorf("expected 55s, got %v", d)
		}
	})

	t.Run("Retry-After takes priority over HuggingFace ratelimit header", func(t *testing.T) {
		h := http.Header{}
		h.Set("Retry-After", "10")
		h.Set("ratelimit", `"api";r=0;t=55`)
		err := &httpError{StatusCode: 429, Body: `{}`, Header: h}
		d, ok := parseRetryDelay(err)
		if !ok {
			t.Fatal("expected ok=true")
		}
		if d != 10*time.Second {
			t.Errorf("expected Retry-After=10s to win, got %v", d)
		}
	})

	t.Run("no retry hint at all returns false", func(t *testing.T) {
		err := &httpError{StatusCode: 429, Body: `{"error":{"type":"rate_limit_error"}}`, Header: http.Header{}}
		_, ok := parseRetryDelay(err)
		if ok {
			t.Fatal("expected ok=false when no retry hint present anywhere")
		}
	})
}
