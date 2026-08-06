package llm

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
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
			name:       "empty model input maps to empty path",
			modelInput: "",
			apiKey:     "testkey",
			wantURL:    "https://generativelanguage.googleapis.com/v1beta/models/:generateContent?key=testkey",
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
		apiModels    []string
	}{
		{
			name:         "gemini-2.5-pro falls back to gemini-2.5-flash",
			provider:     "gemini",
			currentModel: "gemini-2.5-pro",
			wantModel:    "gemini-2.5-flash",
			apiModels:    []string{"models/gemini-2.5-pro", "models/gemini-2.5-flash"},
		},
		{
			name:         "models/gemini-2.5-pro falls back to gemini-2.5-flash",
			provider:     "Gemini",
			currentModel: "models/gemini-2.5-pro",
			wantModel:    "gemini-2.5-flash",
			apiModels:    []string{"models/gemini-2.5-pro", "models/gemini-2.5-flash"},
		},
		{
			name:         "gemini-pro-latest falls back to gemini-flash-latest",
			provider:     "gemini",
			currentModel: "gemini-pro-latest",
			wantModel:    "gemini-flash-latest",
			apiModels:    []string{"models/gemini-pro-latest", "models/gemini-flash-latest"},
		},
		{
			name:         "gemini-2.5-flash falls back to gemini-pro-latest",
			provider:     "gemini",
			currentModel: "gemini-2.5-flash",
			wantModel:    "gemini-pro-latest",
			apiModels:    []string{"models/gemini-2.5-pro", "models/gemini-2.5-flash", "models/gemini-pro-latest", "models/gemini-flash-latest"},
		},
		{
			name:         "gemini-flash-lite-latest is lowest and returns empty",
			provider:     "gemini",
			currentModel: "gemini-flash-lite-latest",
			wantModel:    "",
			apiModels:    []string{"models/gemini-flash-lite-latest"},
		},
		{
			name:         "openai gpt-4o falls back to gpt-4o-mini",
			provider:     "openai",
			currentModel: "gpt-4o",
			wantModel:    "gpt-4o-mini",
			apiModels:    []string{"gpt-4o", "gpt-4o-mini"},
		},
		{
			name:         "openai gpt-4o-mini is lowest and returns empty",
			provider:     "openai",
			currentModel: "gpt-4o-mini",
			wantModel:    "",
			apiModels:    []string{"gpt-4o-mini"},
		},
		{
			name:         "mistral-large falls back to mistral-medium",
			provider:     "mistral",
			currentModel: "mistral-large-latest",
			wantModel:    "mistral-medium-latest",
			apiModels:    []string{"mistral-large-latest", "mistral-medium-latest"},
		},
		{
			name:         "mistral-small falls back to open-mistral-7b",
			provider:     "mistral",
			currentModel: "mistral-small-latest",
			wantModel:    "open-mistral-7b",
			apiModels:    []string{"mistral-small-latest", "open-mistral-7b"},
		},
		{
			name:         "deepseek-coder falls back to deepseek-chat",
			provider:     "deepseek",
			currentModel: "deepseek-coder",
			wantModel:    "deepseek-chat",
			apiModels:    []string{"deepseek-coder", "deepseek-chat"},
		},
		{
			name:         "deepseek-chat is lowest and returns empty",
			provider:     "deepseek",
			currentModel: "deepseek-chat",
			wantModel:    "",
			apiModels:    []string{"deepseek-chat"},
		},
		{
			name:         "hermes 405b falls back to hermes 70b",
			provider:     "hermes",
			currentModel: "hermes-3-llama-3.1-405b",
			wantModel:    "hermes-3-llama-3.1-70b",
			apiModels:    []string{"hermes-3-llama-3.1-405b", "hermes-3-llama-3.1-70b"},
		},
		{
			name:         "opencode glm-5.2 falls back to glm-5.1",
			provider:     "opencode",
			currentModel: "glm-5.2",
			wantModel:    "glm-5.1",
			apiModels:    []string{"glm-5.2", "glm-5.1"},
		},
		{
			name:         "opencode deepseek-v4-flash is lowest and returns empty",
			provider:     "opencode",
			currentModel: "deepseek-v4-flash",
			wantModel:    "",
			apiModels:    []string{"deepseek-v4-flash"},
		},
		{
			name:         "kimi-k3 falls back to kimi-k2.7",
			provider:     "kimi",
			currentModel: "kimi-k3",
			wantModel:    "kimi-k2.7",
			apiModels:    []string{"kimi-k3", "kimi-k2.7"},
		},
		{
			name:         "qwen-max falls back to qwen-plus",
			provider:     "qwen",
			currentModel: "qwen-max",
			wantModel:    "qwen-plus",
			apiModels:    []string{"qwen-max", "qwen-plus"},
		},
		{
			name:         "llama-3.1-405b falls back to llama-3.3-70b",
			provider:     "llama",
			currentModel: "llama-3.1-405b",
			wantModel:    "llama-3.3-70b",
			apiModels:    []string{"llama-3.1-405b", "llama-3.3-70b"},
		},
		{
			name:         "unknown provider returns empty",
			provider:     "unknown",
			currentModel: "gpt-4o",
			wantModel:    "",
			apiModels:    nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if strings.Contains(r.URL.Path, "/models") {
					w.Header().Set("Content-Type", "application/json")
					w.WriteHeader(http.StatusOK)
					if strings.ToLower(tt.provider) == "gemini" {
						var geminiModels []string
						for _, m := range tt.apiModels {
							geminiModels = append(geminiModels, fmt.Sprintf(`{"name": "%s", "displayName": "%s", "supportedGenerationMethods": ["generateContent"]}`, m, m))
						}
						_, _ = fmt.Fprintf(w, `{"models": [%s]}`, strings.Join(geminiModels, ","))
					} else {
						var dataModels []string
						for _, m := range tt.apiModels {
							dataModels = append(dataModels, fmt.Sprintf(`{"id": "%s"}`, m))
						}
						_, _ = fmt.Fprintf(w, `{"data": [%s]}`, strings.Join(dataModels, ","))
					}
				} else {
					w.WriteHeader(http.StatusNotFound)
				}
			}))
			defer server.Close()

			c := &Client{Provider: tt.provider, Model: tt.currentModel, URL: server.URL}
			got := c.getNextLowerModel(context.Background(), "mockkey", tt.currentModel)
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

func TestDynamicGeminiModelSelection(t *testing.T) {
	t.Run("parseGeminiModel valid names", func(t *testing.T) {
		cases := []struct {
			name        string
			wantVersion float64
			wantTier    string
			wantRank    int
		}{
			{"models/gemini-2.5-pro", 2.5, "pro", 4},
			{"gemini-3.5-flash-lite", 3.5, "flash-lite", 2},
			{"models/gemini-2.0-flash-lite-001", 2.0, "flash-lite", 2},
			{"gemini-pro-latest", 1.5, "pro", 4},
			{"models/gemini-flash-latest", 1.5, "flash", 3},
			{"gemini-3-pro-preview", 3.0, "pro", 4},
			{"models/gemini-3-flash-preview", 3.0, "flash", 3},
			{"nano-banana-pro", 1.5, "nano", 1},
		}

		for _, tc := range cases {
			info, ok := parseGeminiModel(tc.name)
			if !ok {
				t.Fatalf("failed to parse valid model: %s", tc.name)
			}
			if info.Version != tc.wantVersion {
				t.Errorf("parseGeminiModel(%q) Version = %v; want %v", tc.name, info.Version, tc.wantVersion)
			}
			if info.Tier != tc.wantTier {
				t.Errorf("parseGeminiModel(%q) Tier = %q; want %q", tc.name, info.Tier, tc.wantTier)
			}
			if info.Rank != tc.wantRank {
				t.Errorf("parseGeminiModel(%q) Rank = %d; want %d", tc.name, info.Rank, tc.wantRank)
			}
		}
	})

	t.Run("parseGeminiModel invalid names", func(t *testing.T) {
		cases := []string{
			"gemma-4-26b-a4b-it",
			"models/imagen-4.0-generate-001",
			"aqa",
		}
		for _, name := range cases {
			if _, ok := parseGeminiModel(name); ok {
				t.Errorf("expected parse failure for invalid name: %s", name)
			}
		}
	})

	t.Run("sortGeminiModels priority", func(t *testing.T) {
		models := []*GeminiModelInfo{
			{Name: "gemini-2.5-pro", Version: 2.5, Tier: "pro", Rank: 4},
			{Name: "gemini-3.5-flash", Version: 3.5, Tier: "flash", Rank: 3},
			{Name: "gemini-3.5-pro", Version: 3.5, Tier: "pro", Rank: 4},
			{Name: "gemini-pro-latest", Version: 1.5, Tier: "pro", Rank: 4},
			{Name: "gemini-3.0-pro", Version: 3.0, Tier: "pro", Rank: 4},
		}

		sortGeminiModels(models)

		expectedOrder := []string{
			"gemini-3.5-pro",
			"gemini-3.5-flash",
			"gemini-3.0-pro",
			"gemini-2.5-pro",
			"gemini-pro-latest",
		}

		for i, m := range models {
			if m.Name != expectedOrder[i] {
				t.Errorf("at index %d got model %s; want %s", i, m.Name, expectedOrder[i])
			}
		}
	})
}
