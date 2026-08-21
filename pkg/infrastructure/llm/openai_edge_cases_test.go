package llm

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

func TestOpenAIEdgeCases_ModelTierRanking(t *testing.T) {
	cases := []struct {
		modelName    string
		expectedTier string
		expectedOk   bool
		minRank      int
	}{
		{"gpt-5", "flagship", true, 85},
		{"gpt-4.5-preview", "flagship", true, 80},
		{"gpt-4o", "flagship", true, 80},
		{"chatgpt-4o-latest", "flagship", true, 80},
		{"gpt-4o-2024-08-06", "flagship", true, 80},
		{"gpt-4-turbo", "pro", true, 60},
		{"gpt-4", "pro", true, 60},
		{"o3", "reasoning", true, 55},
		{"o1", "reasoning", true, 55},
		{"o1-preview", "reasoning", true, 55},
		{"o3-mini", "compact-reasoning", true, 45},
		{"o1-mini", "compact-reasoning", true, 45},
		{"gpt-4o-mini", "compact", true, 40},
		{"gpt-4o-mini-2024-07-18", "compact", true, 40},
		{"gpt-3.5-turbo", "lite", true, 27},
		{"text-embedding-3-small", "", false, 0},
		{"dall-e-3", "", false, 0},
		{"whisper-1", "", false, 0},
		{"tts-1-hd", "", false, 0},
	}

	for _, tc := range cases {
		t.Run("tier_"+tc.modelName, func(t *testing.T) {
			info, ok := parseOpenAIModel(tc.modelName)
			if ok != tc.expectedOk {
				t.Fatalf("model %q: expected ok=%v, got %v", tc.modelName, tc.expectedOk, ok)
			}
			if !tc.expectedOk {
				return
			}
			if info.Tier != tc.expectedTier {
				t.Errorf("model %q: expected tier %q, got %q", tc.modelName, tc.expectedTier, info.Tier)
			}
			if info.Rank < tc.minRank {
				t.Errorf("model %q: expected rank >= %d, got %d", tc.modelName, tc.minRank, info.Rank)
			}
		})
	}
}

func TestOpenAIEdgeCases_IsNoTemperatureModel(t *testing.T) {
	cases := []struct {
		model string
		want  bool
	}{
		{"GPT-4O", false},
		{"gpt-4o-mini", false},
		{"O1", true},
		{"O3-MINI", true},
		{"o4-mini", true},
		{"openai/o1-preview", true},
		{"gateway/router/openai/o3-mini", true},
		{"custom/nested/path/o4", true},
		{"anthropic/claude-3-opus", true},
		{"CLAUDE-3-5-SONNET", true},
		{"deepseek-chat", false},
		{"qwen-2.5-coder", false},
	}

	for _, tc := range cases {
		t.Run("no_temp_"+tc.model, func(t *testing.T) {
			got := isNoTemperatureModel(tc.model)
			if got != tc.want {
				t.Errorf("isNoTemperatureModel(%q) = %v, want %v", tc.model, got, tc.want)
			}
		})
	}
}

func TestOpenAIEdgeCases_BuildChatParamsVariations(t *testing.T) {
	t.Run("zero maxTokens omits MaxCompletionTokens", func(t *testing.T) {
		opts := completionOptions{maxTokens: 0}
		params := buildChatParams("gpt-4o", "hi", opts)
		if params.MaxCompletionTokens.Value != 0 {
			t.Errorf("expected MaxCompletionTokens to be 0/unset, got %d", params.MaxCompletionTokens.Value)
		}
	})

	t.Run("temperature 0 on standard model is set to 0.0", func(t *testing.T) {
		temp := 0.0
		opts := completionOptions{temperature: &temp}
		params := buildChatParams("gpt-4o", "hi", opts)
		if params.Temperature.Value != 0.0 {
			t.Errorf("expected Temperature=0.0, got %v", params.Temperature.Value)
		}
	})

	t.Run("reasoning model ignores temperature", func(t *testing.T) {
		temp := 0.7
		opts := completionOptions{temperature: &temp}
		params := buildChatParams("o3-mini", "hi", opts)
		if params.Temperature.Value != 0 {
			t.Errorf("expected Temperature to be omitted on o3-mini, got %v", params.Temperature.Value)
		}
	})
}

func TestOpenAIEdgeCases_LegacyGatewayMaxCompletionTokensRejection(t *testing.T) {
	t.Run("when legacy gateway rejects max_completion_tokens it retries without maxTokens", func(t *testing.T) {
		var calls int64
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			count := atomic.AddInt64(&calls, 1)
			req := decodeChatRequest(t, r)
			if count == 1 && req.MaxCompletionTokens != nil {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusBadRequest)
				_, _ = w.Write([]byte(`{"type":"invalid_request_error","message":"Unsupported parameter: 'max_completion_tokens' is not supported with this legacy gateway."}`))
				return
			}
			writeChatCompletion(w, `{"ok":true}`, "")
		}))
		defer server.Close()

		client := newBaseOpenAIClient("openai", server.URL, server.URL, 5*time.Second, 0, false)
		testModel := "legacy-gateway-model"

		body, err := client.Call(context.Background(), testModel, "k", "Return json", 4096, 0.0)
		if err != nil {
			t.Fatalf("expected legacy gateway call to adapt and succeed, got %v", err)
		}
		if string(body) != `{"ok":true}` {
			t.Errorf("unexpected response body: %s", body)
		}
		if got := atomic.LoadInt64(&calls); got != 2 {
			t.Errorf("expected exactly 2 calls (initial rejection + adapted retry), got %d", got)
		}
	})

	t.Run("when model rejects an extra body parameter it strips it and retries", func(t *testing.T) {
		var calls int64
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			count := atomic.AddInt64(&calls, 1)
			if count == 1 {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusBadRequest)
				_, _ = w.Write([]byte(`{"type":"invalid_request_error","message":"unknown parameter: 'enable_thinking'"}`))
				return
			}
			writeChatCompletion(w, `{"ok":true}`, "")
		}))
		defer server.Close()

		client := newBaseOpenAIClient("openai", server.URL, server.URL, 5*time.Second, 0, false)
		client.SetExtraBody(map[string]interface{}{"enable_thinking": true})

		body, err := client.Call(context.Background(), "custom-model", "k", "hello", 0, 0.0)
		if err != nil {
			t.Fatalf("expected extraBody-adapted call to succeed, got %v", err)
		}
		if string(body) != `{"ok":true}` {
			t.Errorf("unexpected response body: %s", body)
		}
		if got := atomic.LoadInt64(&calls); got != 2 {
			t.Errorf("expected 2 calls, got %d", got)
		}
	})
}
