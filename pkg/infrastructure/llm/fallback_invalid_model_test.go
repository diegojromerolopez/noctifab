package llm

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSelectFallbackModel_PrefixMatching(t *testing.T) {
	t.Run("Anthropic prefix match: claude-3-7-sonnet matches claude-3-7-sonnet-20250219", func(t *testing.T) {
		ResetModelBlacklist()
		available := []string{
			"claude-3-opus-20240229",
			"claude-3-7-sonnet-20250219",
			"claude-3-5-sonnet-20241022",
			"claude-3-5-haiku-20241022",
		}
		parsed := parsedModelsFor(available, parseAnthropicModel)

		// Configured model is "claude-3-7-sonnet" (missing date snapshot), failing model is "claude-3-7-sonnet"
		fallback := selectFallbackModel("claude-3-7-sonnet", "claude-3-7-sonnet", parsed)
		assert.Equal(t, "claude-3-7-sonnet-20250219", fallback)
	})

	t.Run("OpenAI prefix match: gpt-4o-custom matches gpt-4o", func(t *testing.T) {
		ResetModelBlacklist()
		available := []string{
			"gpt-4o",
			"gpt-4o-2024-08-06",
			"gpt-4o-mini",
			"gpt-3.5-turbo",
		}
		parsed := parsedModelsFor(available, parseOpenAIModel)

		fallback := selectFallbackModel("gpt-4o-custom", "gpt-4o-custom", parsed)
		assert.Contains(t, []string{"gpt-4o", "gpt-4o-2024-08-06"}, fallback)
	})

	t.Run("Prefix match boundary safety: gpt-4o-mini-custom does NOT match gpt-4", func(t *testing.T) {
		ResetModelBlacklist()
		available := []string{
			"gpt-4",
			"gpt-4o-mini",
		}
		parsed := parsedModelsFor(available, parseOpenAIModel)

		// gpt-4o-mini-custom should match gpt-4o-mini on boundary, NOT jump to gpt-4
		fallback := selectFallbackModel("gpt-4o-mini-custom", "gpt-4o-mini-custom", parsed)
		assert.Equal(t, "gpt-4o-mini", fallback)
	})

	t.Run("Gemini prefix match with models/ prefix", func(t *testing.T) {
		ResetModelBlacklist()
		available := []string{
			"models/gemini-2.5-flash",
			"models/gemini-2.0-flash",
			"models/gemini-1.5-pro",
		}
		parsed := parsedModelsFor(available, parseGeminiModelProvider)

		fallback := selectFallbackModel("gemini-2.5-flash-preview", "gemini-2.5-flash-preview", parsed)
		assert.Equal(t, "models/gemini-2.5-flash", fallback)
	})
}

func TestSelectFallbackModel_BestAvailableWhenNoPrefixMatch(t *testing.T) {
	t.Run("Falls back to top ranked model when model name has zero prefix matches", func(t *testing.T) {
		ResetModelBlacklist()
		available := []string{
			"claude-3-opus-20240229",
			"claude-3-5-sonnet-20241022",
			"claude-3-5-haiku-20241022",
		}
		parsed := parsedModelsFor(available, parseAnthropicModel)

		fallback := selectFallbackModel("completely-unknown-model-xyz", "completely-unknown-model-xyz", parsed)
		// Best available is Opus (highest tier/score: 400 + 30 = 430)
		assert.Equal(t, "claude-3-opus-20240229", fallback)
	})

	t.Run("Falls back to next best available when top model is blacklisted", func(t *testing.T) {
		ResetModelBlacklist()
		available := []string{
			"claude-3-opus-20240229",
			"claude-3-5-sonnet-20241022",
			"claude-3-5-haiku-20241022",
		}
		parsed := parsedModelsFor(available, parseAnthropicModel)

		BlacklistModel("claude-3-opus-20240229")
		fallback := selectFallbackModel("completely-unknown-model-xyz", "completely-unknown-model-xyz", parsed)
		// Next best is Sonnet
		assert.Equal(t, "claude-3-5-sonnet-20241022", fallback)
	})

	t.Run("Prefix match skips blacklisted candidate and picks next prefix or best", func(t *testing.T) {
		ResetModelBlacklist()
		available := []string{
			"claude-3-7-sonnet-20250219",
			"claude-3-7-sonnet-preview",
			"claude-3-5-sonnet-20241022",
			"claude-3-opus-20240229",
		}
		parsed := parsedModelsFor(available, parseAnthropicModel)

		BlacklistModel("claude-3-7-sonnet-20250219")
		fallback := selectFallbackModel("claude-3-7-sonnet", "claude-3-7-sonnet", parsed)
		assert.Equal(t, "claude-3-7-sonnet-preview", fallback)
	})
}

func TestIsModelNotFoundOrDeprecated(t *testing.T) {
	testCases := []struct {
		name     string
		err      error
		expected bool
	}{
		{
			name: "Anthropic 400 model not valid",
			err: &httpError{
				StatusCode: 400,
				Body:       `{"type":"error","error":{"type":"invalid_request_error","message":"model: claude-3-7-sonnet is not a valid model"}}`,
			},
			expected: true,
		},
		{
			name: "OpenAI 404 model not found",
			err: &httpError{
				StatusCode: 404,
				Body:       `{"error":{"message":"The model gpt-5 does not exist","type":"invalid_request_error","code":"model_not_found"}}`,
			},
			expected: true,
		},
		{
			name: "Gemini 404 models not found",
			err: &httpError{
				StatusCode: 404,
				Body:       `models/gemini-unknown is not found for API version v1beta`,
			},
			expected: true,
		},
		{
			name: "Generic 400 invalid model",
			err: &httpError{
				StatusCode: 400,
				Body:       `invalid model: mistral-nonexistent`,
			},
			expected: true,
		},
		{
			name: "Ollama 404 model not found",
			err: &httpError{
				StatusCode: 404,
				Body:       `{"error":"model 'llama4:70b' not found, try pulling it first"}`,
			},
			expected: true,
		},
		{
			name:     "Generic error with unsupported model string",
			err:      fmt.Errorf("API error: model is not supported on this endpoint"),
			expected: true,
		},
		{
			name: "Auth error is not a model error",
			err: &httpError{
				StatusCode: 401,
				Body:       `{"error":"Invalid API key provided"}`,
			},
			expected: false,
		},
		{
			name: "Rate limit 429 is not a model error",
			err: &httpError{
				StatusCode: 429,
				Body:       `{"error":"Rate limit exceeded"}`,
			},
			expected: false,
		},
		{
			name: "Internal server error 500 is not a model error",
			err: &httpError{
				StatusCode: 500,
				Body:       `{"error":"Internal server error"}`,
			},
			expected: false,
		},
		{
			name: "404 route not found is not a model error",
			err: &httpError{
				StatusCode: 404,
				Body:       `{"error":"endpoint /v1/chat not found"}`,
			},
			expected: false,
		},
		{
			name: "400 unsupported parameter is not a model error",
			err: &httpError{
				StatusCode: 400,
				Body:       `{"error":"parameter 'temperature' is not supported"}`,
			},
			expected: false,
		},
		{
			name: "400 organization does not exist is not a model error",
			err: &httpError{
				StatusCode: 400,
				Body:       `{"error":"organization does not exist"}`,
			},
			expected: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			actual := isModelNotFoundOrDeprecated(tc.err)
			assert.Equal(t, tc.expected, actual)
			if tc.expected {
				// shouldSkipModelFallback must be false when model is not found
				assert.False(t, shouldSkipModelFallback(tc.err))
			}
		})
	}
}

func TestComplete_InvalidModelPrefixRecoveryE2E(t *testing.T) {
	ResetModelBlacklist()

	var attempts int64
	var requestedModels []string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/models") {
			w.Header().Set("Content-Type", "application/json")
			_, _ = fmt.Fprintf(w, `{"object":"list","data":[{"id":"qwen3.8-max-20260101","object":"model"},{"id":"qwen3.8-max","object":"model"},{"id":"qwen3.7-max","object":"model"}]}`)
			return
		}

		var req map[string]any
		_ = json.NewDecoder(r.Body).Decode(&req)
		m, _ := req["model"].(string)
		requestedModels = append(requestedModels, m)

		count := atomic.AddInt64(&attempts, 1)
		if count == 1 {
			// First attempt fails with invalid model HTTP error
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadRequest)
			_, _ = fmt.Fprintf(w, `{"error":{"type":"invalid_request_error","message":"model: %s is not a valid model"}}`, m)
			return
		}

		// Subsequent attempt with fallback model succeeds
		env := `{"reasoning":"ok","actions":[]}`
		payload, _ := json.Marshal(map[string]any{
			"choices": []map[string]any{{"message": map[string]any{"content": string(env)}}},
		})
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(payload)
	}))
	defer srv.Close()

	c := &Client{
		Provider:              "qwencloud",
		Model:                 "qwen3.8-max-custom",
		APIKey:                "testkey",
		URL:                   srv.URL,
		Timeout:               2 * time.Second,
		IdleTimeout:           2 * time.Second,
		Streaming:             false,
		SkipOnCreditExhausted: true,
	}

	resp, err := c.Complete(context.Background(), "test prompt")
	require.NoError(t, err)
	require.NotNil(t, resp)

	// First attempt was qwen3.8-max-custom (failed), second attempt was recovered prefix model
	require.Len(t, requestedModels, 2)
	assert.Equal(t, "qwen3.8-max-custom", requestedModels[0])
	assert.Contains(t, []string{"qwen3.8-max", "qwen3.8-max-20260101"}, requestedModels[1])
}

func TestComplete_InvalidModelBestAvailableRecoveryE2E(t *testing.T) {
	ResetModelBlacklist()

	var attempts int64
	var requestedModels []string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/models") {
			w.Header().Set("Content-Type", "application/json")
			_, _ = fmt.Fprintf(w, `{"object":"list","data":[{"id":"qwen3.8-max","object":"model"},{"id":"qwen3.7-max","object":"model"}]}`)
			return
		}

		var req map[string]any
		_ = json.NewDecoder(r.Body).Decode(&req)
		m, _ := req["model"].(string)
		requestedModels = append(requestedModels, m)

		count := atomic.AddInt64(&attempts, 1)
		if count == 1 {
			// First attempt with unknown model fails
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadRequest)
			_, _ = fmt.Fprintf(w, `{"error":{"type":"invalid_request_error","message":"model: %s does not exist"}}`, m)
			return
		}

		// Second attempt succeeds
		env := `{"reasoning":"ok","actions":[]}`
		payload, _ := json.Marshal(map[string]any{
			"choices": []map[string]any{{"message": map[string]any{"content": string(env)}}},
		})
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(payload)
	}))
	defer srv.Close()

	c := &Client{
		Provider:              "qwencloud",
		Model:                 "totally-unknown-nonexistent-model",
		APIKey:                "testkey",
		URL:                   srv.URL,
		Timeout:               2 * time.Second,
		IdleTimeout:           2 * time.Second,
		Streaming:             false,
		SkipOnCreditExhausted: true,
	}

	resp, err := c.Complete(context.Background(), "test prompt")
	require.NoError(t, err)
	require.NotNil(t, resp)

	// First attempt was totally-unknown-nonexistent-model (failed), second attempt was best available model
	require.Len(t, requestedModels, 2)
	assert.Equal(t, "totally-unknown-nonexistent-model", requestedModels[0])
	assert.Equal(t, "qwen3.8-max", requestedModels[1])
}

func TestSelectFallbackModel_CaseInsensitivePrefix(t *testing.T) {
	ResetModelBlacklist()
	available := []string{
		"claude-3-7-sonnet-20250219",
		"claude-3-5-haiku-20241022",
	}
	parsed := parsedModelsFor(available, parseAnthropicModel)

	fallback := selectFallbackModel("CLAUDE-3-7-SONNET", "CLAUDE-3-7-SONNET", parsed)
	assert.Equal(t, "claude-3-7-sonnet-20250219", fallback)
}

func TestSelectFallbackModel_AllBlacklistedReturnsEmpty(t *testing.T) {
	ResetModelBlacklist()
	available := []string{
		"gpt-4o",
		"gpt-4o-mini",
	}
	parsed := parsedModelsFor(available, parseOpenAIModel)

	BlacklistModel("gpt-4o")
	BlacklistModel("gpt-4o-mini")

	fallback := selectFallbackModel("gpt-4o-custom", "gpt-4o-custom", parsed)
	assert.Empty(t, fallback)
}

func TestComplete_AnthropicInvalidModelRecoveryE2E(t *testing.T) {
	ResetModelBlacklist()

	var attempts int64
	var requestedModels []string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/models") {
			w.Header().Set("Content-Type", "application/json")
			_, _ = fmt.Fprintf(w, `{"data":[{"id":"claude-3-7-sonnet-20250219"},{"id":"claude-3-5-haiku-20241022"}]}`)
			return
		}

		var req map[string]any
		_ = json.NewDecoder(r.Body).Decode(&req)
		m, _ := req["model"].(string)
		requestedModels = append(requestedModels, m)

		count := atomic.AddInt64(&attempts, 1)
		if count == 1 {
			// First attempt fails with Anthropic 400 invalid model
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadRequest)
			_, _ = fmt.Fprintf(w, `{"type":"error","error":{"type":"invalid_request_error","message":"model: %s is not a valid model"}}`, m)
			return
		}

		// Anthropic messages API response
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprintf(w, `{"content":[{"type":"text","text":"{\"reasoning\":\"ok\",\"actions\":[]}"}]}`)
	}))
	defer srv.Close()

	c := &Client{
		Provider:              "anthropic",
		Model:                 "claude-3-7-sonnet",
		APIKey:                "testkey",
		URL:                   srv.URL,
		Timeout:               2 * time.Second,
		IdleTimeout:           2 * time.Second,
		Streaming:             false,
		SkipOnCreditExhausted: true,
	}

	resp, err := c.Complete(context.Background(), "test prompt")
	require.NoError(t, err)
	require.NotNil(t, resp)

	// First attempt was "claude-3-7-sonnet" (failed), second attempt was recovered "claude-3-7-sonnet-20250219"
	require.Len(t, requestedModels, 2)
	assert.Equal(t, "claude-3-7-sonnet", requestedModels[0])
	assert.Equal(t, "claude-3-7-sonnet-20250219", requestedModels[1])
}
