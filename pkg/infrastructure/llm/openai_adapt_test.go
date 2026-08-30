package llm

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestIsNoTemperatureModel(t *testing.T) {
	cases := []struct {
		model string
		want  bool
	}{
		{"claude-3-5-sonnet", true},
		{"claude-opus-5", true},
		{"anthropic/claude-3-7-sonnet-latest", true},
		{"o1-mini", true},
		{"o3-mini", true},
		{"o4-mini", true},
		{"openai/o1", true},
		{"openai/o3-mini-2025-01-31", true},
		{"gpt-4o", false},
		{"deepseek-r1", false},
	}
	for _, tc := range cases {
		t.Run("model "+tc.model, func(t *testing.T) {
			if got := isNoTemperatureModel(tc.model); got != tc.want {
				t.Errorf("isNoTemperatureModel(%q) = %v, want %v", tc.model, got, tc.want)
			}
		})
	}
}

func TestEnsureJSONKeyword(t *testing.T) {
	t.Run("when the prompt already mentions JSON it is unchanged", func(t *testing.T) {
		p := "Return a JSON envelope."
		if got := ensureJSONKeyword(p); got != p {
			t.Errorf("prompt was modified: %q", got)
		}
	})
	t.Run("when the prompt mentions json in any case it is unchanged", func(t *testing.T) {
		p := "respond with jSoN only"
		if got := ensureJSONKeyword(p); got != p {
			t.Errorf("prompt was modified: %q", got)
		}
	})
	t.Run("when the prompt lacks the word json a JSON instruction is appended", func(t *testing.T) {
		got := ensureJSONKeyword("Summarize the spec.")
		if !strings.Contains(strings.ToLower(got), "json") {
			t.Errorf("expected appended json instruction, got %q", got)
		}
		if !strings.HasPrefix(got, "Summarize the spec.") {
			t.Errorf("original prompt not preserved: %q", got)
		}
	})
}

func TestAdaptOptionsForError(t *testing.T) {
	temp := 0.3
	base := completionOptions{enforceJSON: true, maxTokens: 1000, temperature: &temp}

	t.Run("when the server rejects response_format it disables JSON enforcement", func(t *testing.T) {
		err := &httpError{StatusCode: 400, Body: "response_format is not supported"}
		got, ok := adaptOptionsForError(base, err, "test-model")
		if !ok || got.enforceJSON {
			t.Errorf("expected enforceJSON disabled, got ok=%v opts=%+v", ok, got)
		}
		if got.maxTokens != base.maxTokens || got.temperature == nil {
			t.Errorf("unrelated options were modified: %+v", got)
		}
	})

	t.Run("when the gateway router is unavailable it strips response_format and max_tokens", func(t *testing.T) {
		err := &httpError{StatusCode: 500, Body: `{"type":"Router.Unavailable","modelID":"kimi-k3"}`}
		got, ok := adaptOptionsForError(base, err, "test-model")
		if !ok || got.enforceJSON || got.maxTokens != 0 {
			t.Errorf("expected rf+maxTokens stripped, got ok=%v opts=%+v", ok, got)
		}
	})

	t.Run("when the model pins its temperature it omits the temperature field", func(t *testing.T) {
		err := &httpError{StatusCode: 400, Body: "invalid temperature: only 1 is allowed for this model"}
		got, ok := adaptOptionsForError(base, err, "test-model")
		if !ok || got.temperature != nil {
			t.Errorf("expected temperature omitted, got ok=%v opts=%+v", ok, got)
		}
	})

	t.Run("when the error is not parameter-induced it does not adapt", func(t *testing.T) {
		err := &httpError{StatusCode: 500, Body: "internal server error"}
		if _, ok := adaptOptionsForError(base, err, "test-model"); ok {
			t.Error("expected no adaptation for generic 500")
		}
	})

	t.Run("when the error is not an httpError it does not adapt", func(t *testing.T) {
		if _, ok := adaptOptionsForError(base, fmt.Errorf("dial tcp: timeout"), "test-model"); ok {
			t.Error("expected no adaptation for transport error")
		}
	})

	t.Run("when everything is already relaxed it does not loop", func(t *testing.T) {
		relaxed := completionOptions{}
		err := &httpError{StatusCode: 500, Body: `{"type":"Router.Unavailable"}`}
		if _, ok := adaptOptionsForError(relaxed, err, "test-model"); ok {
			t.Error("expected no adaptation when no options remain to relax")
		}
	})
}

func TestIsNonRetryableHTTPError(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want bool
	}{
		{"400 bad request", &httpError{StatusCode: 400, Body: "x"}, true},
		{"401 unauthorized", &httpError{StatusCode: 401, Body: "Model not supported"}, true},
		{"403 forbidden", &httpError{StatusCode: 403, Body: "x"}, true},
		{"404 not found", &httpError{StatusCode: 404, Body: "x"}, true},
		{"422 unprocessable", &httpError{StatusCode: 422, Body: "x"}, true},
		{"router unavailable 500", &httpError{StatusCode: 500, Body: `{"type":"Router.Unavailable"}`}, true},
		{"generic 500", &httpError{StatusCode: 500, Body: "internal"}, false},
		{"429 rate limit", &httpError{StatusCode: 429, Body: "slow down"}, false},
		{"408 timeout", &httpError{StatusCode: 408, Body: "x"}, false},
		{"transport error", fmt.Errorf("dial tcp: refused"), false},
		{"wrapped 401", fmt.Errorf("call failed: %w", &httpError{StatusCode: 401, Body: "x"}), true},
	}
	for _, tc := range cases {
		t.Run("when "+tc.name, func(t *testing.T) {
			if got := isNonRetryableHTTPError(tc.err); got != tc.want {
				t.Errorf("isNonRetryableHTTPError(%v) = %v, want %v", tc.err, got, tc.want)
			}
		})
	}
}

// chatRequest is the subset of the OpenAI request body inspected by tests.
type chatRequest struct {
	Messages []struct {
		Content string `json:"content"`
	} `json:"messages"`
	MaxTokens           *int            `json:"max_tokens"`
	MaxCompletionTokens *int            `json:"max_completion_tokens"`
	Temperature         *float64        `json:"temperature"`
	ResponseFormat      json.RawMessage `json:"response_format"`
}

func decodeChatRequest(t *testing.T, r *http.Request) chatRequest {
	t.Helper()
	raw, err := io.ReadAll(r.Body)
	if err != nil {
		t.Fatalf("reading request body: %v", err)
	}
	var req chatRequest
	if err := json.Unmarshal(raw, &req); err != nil {
		t.Fatalf("unmarshalling request body: %v", err)
	}
	return req
}

func writeChatCompletion(w http.ResponseWriter, content, reasoning string) {
	w.Header().Set("Content-Type", "application/json")
	msg := map[string]any{"role": "assistant", "content": content}
	if reasoning != "" {
		msg["reasoning_content"] = reasoning
	}
	_ = json.NewEncoder(w).Encode(map[string]any{
		"choices": []any{map[string]any{"finish_reason": "stop", "message": msg}},
	})
}

func TestCallAdaptiveFallback(t *testing.T) {
	t.Run("when the gateway returns Router.Unavailable it retries without response_format and max_tokens", func(t *testing.T) {
		var calls int64
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			atomic.AddInt64(&calls, 1)
			req := decodeChatRequest(t, r)
			if req.ResponseFormat != nil || req.MaxCompletionTokens != nil || req.MaxTokens != nil {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusInternalServerError)
				_, _ = w.Write([]byte(`{"type":"Router.Unavailable","modelID":"kimi-k3"}`))
				return
			}
			writeChatCompletion(w, `{"ok":true}`, "")
		}))
		defer server.Close()

		client := newBaseOpenAIClient("opencode", server.URL, server.URL, 5*time.Second, 0, false)
		body, err := client.Call(context.Background(), "kimi-k3", "k", "Return a json object.", 32768, 1)
		if err != nil {
			t.Fatalf("expected adapted call to succeed, got %v", err)
		}
		if string(body.Body) != `{"ok":true}` {
			t.Errorf("unexpected body: %s", body.Body)
		}
		if got := atomic.LoadInt64(&calls); got != 2 {
			t.Errorf("expected exactly 2 requests (rejected + adapted), got %d", got)
		}
	})

	t.Run("when the model pins its temperature it retries with the field omitted", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			req := decodeChatRequest(t, r)
			if req.Temperature != nil && *req.Temperature != 1 {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusBadRequest)
				_, _ = w.Write([]byte(`{"type":"invalid_request_error","message":"invalid temperature: only 1 is allowed for this model"}`))
				return
			}
			writeChatCompletion(w, `{"ok":true}`, "")
		}))
		defer server.Close()

		client := newBaseOpenAIClient("opencode", server.URL, server.URL, 5*time.Second, 0, false)
		if _, err := client.Call(context.Background(), "kimi-k3", "k", "Return a json object.", 0, 0.3); err != nil {
			t.Fatalf("expected temperature-adapted call to succeed, got %v", err)
		}
	})

	t.Run("when the error is unrecognised it fails without extra attempts", func(t *testing.T) {
		var calls int64
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			atomic.AddInt64(&calls, 1)
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = w.Write([]byte(`{"type":"ModelError","message":"Model x is not supported"}`))
		}))
		defer server.Close()

		client := newBaseOpenAIClient("opencode", server.URL, server.URL, 5*time.Second, 0, false)
		if _, err := client.Call(context.Background(), "x", "k", "json please", 0, 0); err == nil {
			t.Fatal("expected error")
		}
		if got := atomic.LoadInt64(&calls); got != 1 {
			t.Errorf("expected exactly 1 request for a non-adaptable error, got %d", got)
		}
	})

	t.Run("when enforcing JSON the outgoing prompt always contains the word json", func(t *testing.T) {
		var sawJSON atomic.Bool
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			req := decodeChatRequest(t, r)
			if len(req.Messages) > 0 && strings.Contains(strings.ToLower(req.Messages[0].Content), "json") {
				sawJSON.Store(true)
			}
			writeChatCompletion(w, `{"ok":true}`, "")
		}))
		defer server.Close()

		client := newBaseOpenAIClient("opencode", server.URL, server.URL, 5*time.Second, 0, false)
		if _, err := client.Call(context.Background(), "m", "k", "Summarize the spec.", 0, 0); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !sawJSON.Load() {
			t.Error("expected the outgoing prompt to contain the word 'json'")
		}
	})
}

func TestReasoningContentFallback(t *testing.T) {
	t.Run("when a non-streaming response has empty content it falls back to reasoning_content", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			writeChatCompletion(w, "", `{"greeting":"hello"}`)
		}))
		defer server.Close()

		client := newBaseOpenAIClient("opencode", server.URL, server.URL, 5*time.Second, 0, false)
		body, err := client.Call(context.Background(), "glm-5.2", "k", "Return a json object.", 0, 0.3)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if string(body.Body) != `{"greeting":"hello"}` {
			t.Errorf("expected reasoning_content fallback, got %q", body.Body)
		}
	})

	t.Run("when a streaming response only carries reasoning_content deltas it accumulates them", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "text/event-stream")
			fl := w.(http.Flusher)
			for _, part := range []string{`{\"greeting\":`, `\"hello\"}`} {
				_, _ = fmt.Fprintf(w, "data: {\"choices\":[{\"delta\":{\"content\":\"\",\"reasoning_content\":\"%s\"}}]}\n\n", part)
				fl.Flush()
			}
			_, _ = w.Write([]byte("data: [DONE]\n\n"))
			fl.Flush()
		}))
		defer server.Close()

		client := newBaseOpenAIClient("opencode", server.URL, server.URL, 5*time.Second, time.Second, true)
		body, err := client.Call(context.Background(), "glm-5.2", "k", "Return a json object.", 0, 0.3)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if string(body.Body) != `{"greeting":"hello"}` {
			t.Errorf("expected accumulated reasoning_content, got %q", body.Body)
		}
	})
}

func TestSlidingIdleTimeout(t *testing.T) {
	t.Run("when chunks keep arriving a stream longer than idle_timeout is not cut short", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "text/event-stream")
			fl := w.(http.Flusher)
			// 6 chunks x 60ms = 360ms total, well above the 150ms idle timeout,
			// but no single gap exceeds it.
			for i := 0; i < 6; i++ {
				_, _ = w.Write([]byte("data: {\"choices\":[{\"delta\":{\"content\":\"x\"}}]}\n\n"))
				fl.Flush()
				time.Sleep(60 * time.Millisecond)
			}
			_, _ = w.Write([]byte("data: [DONE]\n\n"))
			fl.Flush()
		}))
		defer server.Close()

		client := newBaseOpenAIClient("opencode", server.URL, server.URL, 10*time.Second, 150*time.Millisecond, true)
		body, err := client.sendCompletionStreaming(context.Background(), "m", "k", "hi", completionOptions{})
		if err != nil {
			t.Fatalf("expected steady stream to survive sliding idle timeout, got %v", err)
		}
		if string(body.Body) != "xxxxxx" {
			t.Errorf("unexpected content: %q", body.Body)
		}
	})

	t.Run("when the stream stalls mid-flight it fails with an idle timeout error", func(t *testing.T) {
		done := make(chan struct{})
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "text/event-stream")
			fl := w.(http.Flusher)
			_, _ = w.Write([]byte("data: {\"choices\":[{\"delta\":{\"content\":\"x\"}}]}\n\n"))
			fl.Flush()
			<-done // stall forever
		}))
		t.Cleanup(func() {
			close(done)
			server.Close()
		})

		client := newBaseOpenAIClient("opencode", server.URL, server.URL, 10*time.Second, 150*time.Millisecond, true)
		start := time.Now()
		_, err := client.sendCompletionStreaming(context.Background(), "m", "k", "hi", completionOptions{})
		if err == nil {
			t.Fatal("expected idle timeout error")
		}
		if !strings.Contains(err.Error(), "idle timeout") {
			t.Errorf("expected idle timeout error, got: %v", err)
		}
		if elapsed := time.Since(start); elapsed > 2*time.Second {
			t.Errorf("stalled stream took %v to fail; expected ~150ms", elapsed)
		}
	})
}

func TestStreamingHTTPErrorSkipsNonStreamingFallback(t *testing.T) {
	t.Run("when streaming receives a structured HTTP rejection it does not re-run non-streaming", func(t *testing.T) {
		var calls int64
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			atomic.AddInt64(&calls, 1)
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = w.Write([]byte(`{"type":"ModelError","message":"Model x is not supported"}`))
		}))
		defer server.Close()

		client := newBaseOpenAIClient("opencode", server.URL, server.URL, 5*time.Second, time.Second, true)
		if _, err := client.Call(context.Background(), "x", "k", "json please", 0, 0); err == nil {
			t.Fatal("expected error")
		}
		if got := atomic.LoadInt64(&calls); got != 1 {
			t.Errorf("expected 1 request (no non-streaming double execution), got %d", got)
		}
	})
}

func TestClientRetryLoopNonRetryable(t *testing.T) {
	t.Run("when the provider rejects deterministically the client does not burn retries", func(t *testing.T) {
		var completionCalls int64
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if strings.Contains(r.URL.Path, "models") {
				w.Header().Set("Content-Type", "application/json")
				_ = json.NewEncoder(w).Encode(map[string]any{"data": []any{}})
				return
			}
			atomic.AddInt64(&completionCalls, 1)
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = w.Write([]byte(`{"type":"ModelError","message":"Model x is not supported"}`))
		}))
		defer server.Close()

		c := &Client{
			Provider:   "opencode",
			Model:      "x",
			APIKey:     "k",
			URL:        server.URL,
			Timeout:    5 * time.Second,
			MaxRetries: 5,
			Backoff:    10 * time.Millisecond,
		}
		start := time.Now()
		if _, err := c.Complete(context.Background(), "json please"); err == nil {
			t.Fatal("expected error")
		}
		if got := atomic.LoadInt64(&completionCalls); got != 1 {
			t.Errorf("expected exactly 1 completion request for a 401, got %d", got)
		}
		if elapsed := time.Since(start); elapsed > 3*time.Second {
			t.Errorf("non-retryable failure took %v; expected fast failover", elapsed)
		}
	})
}

func TestProviderCapabilityCaching(t *testing.T) {
	t.Run("caches parameter rejections so subsequent calls for the same model omit the parameter on first attempt", func(t *testing.T) {
		var calls int64
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			count := atomic.AddInt64(&calls, 1)
			req := decodeChatRequest(t, r)
			// First request: carrying temperature -> rejected
			if count == 1 && req.Temperature != nil {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusBadRequest)
				_, _ = w.Write([]byte(`{"type":"invalid_request_error","message":"invalid temperature: only 1 is allowed for this model"}`))
				return
			}
			// Second request onwards: temperature must be omitted!
			if req.Temperature != nil {
				t.Errorf("request %d carried temperature even though it should be cached as unsupported", count)
			}
			writeChatCompletion(w, `{"ok":true}`, "")
		}))
		defer server.Close()

		client := newBaseOpenAIClient("opencode", server.URL, server.URL, 5*time.Second, 0, false)
		testModel := "test-temp-pinned-model"

		// First Call: hits temperature rejection (call 1), adapts, succeeds on retry (call 2)
		if _, err := client.Call(context.Background(), testModel, "k", "Return json", 0, 0.5); err != nil {
			t.Fatalf("first call failed: %v", err)
		}
		if got := atomic.LoadInt64(&calls); got != 2 {
			t.Fatalf("expected 2 calls for initial rejection + retry, got %d", got)
		}

		// Second Call: capability is cached! Must succeed on attempt 1 (call 3 total) without temperature
		if _, err := client.Call(context.Background(), testModel, "k", "Return json", 0, 0.5); err != nil {
			t.Fatalf("second call failed: %v", err)
		}
		if got := atomic.LoadInt64(&calls); got != 3 {
			t.Fatalf("expected second call to succeed on first attempt (3 total calls), got %d", got)
		}
	})
}

func TestBuildChatParams(t *testing.T) {
	opts := completionOptions{
		maxTokens:   2048,
		enforceJSON: true,
	}
	params := buildChatParams("gpt-4o", "Hello world", opts)
	if params.MaxCompletionTokens.Value != 2048 {
		t.Errorf("expected MaxCompletionTokens=2048, got %v", params.MaxCompletionTokens.Value)
	}
	if params.MaxTokens.Value != 0 {
		t.Errorf("expected MaxTokens to be empty/0, got %v", params.MaxTokens.Value)
	}
}
