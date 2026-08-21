package llm

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestOpenAIProviderClient_Call(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Method != "POST" {
				w.WriteHeader(http.StatusMethodNotAllowed)
				return
			}
			if r.Header.Get("Authorization") != "Bearer test-key" {
				w.WriteHeader(http.StatusUnauthorized)
				return
			}
			var req map[string]any
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			if req["model"] != "gpt-4o" {
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			if req["max_completion_tokens"] != float64(4096) {
				w.WriteHeader(http.StatusBadRequest)
				_, _ = w.Write([]byte("missing max_completion_tokens"))
				return
			}

			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			resp := map[string]any{
				"choices": []map[string]any{
					{
						"message": map[string]any{
							"content": "openai response text",
						},
					},
				},
			}
			_ = json.NewEncoder(w).Encode(resp)
		}))
		defer server.Close()

		client := NewOpenAIProviderClient("openai", server.URL, 0, 0, false)
		res, err := client.Call(context.Background(), "gpt-4o", "test-key", "hello", 4096, 0.0)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if string(res) != "openai response text" {
			t.Errorf("expected 'openai response text', got %s", string(res))
		}
	})

	t.Run("http error", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte("internal error"))
		}))
		defer server.Close()

		client := NewOpenAIProviderClient("openai", server.URL, 0, 0, false)
		_, err := client.Call(context.Background(), "gpt-4o", "test-key", "hello", 4096, 0.0)
		if err == nil {
			t.Fatal("expected error, got nil")
		}
	})
}

func TestOpenAIProviderClient_GetAvailableModels(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Method != "GET" {
				w.WriteHeader(http.StatusMethodNotAllowed)
				return
			}
			if r.URL.Path != "/models" {
				w.WriteHeader(http.StatusNotFound)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			resp := map[string]any{
				"data": []map[string]any{
					{"id": "gpt-4o"},
					{"id": "gpt-4o-mini"},
				},
			}
			_ = json.NewEncoder(w).Encode(resp)
		}))
		defer server.Close()

		client := NewOpenAIProviderClient("openai", server.URL, 0, 0, false)
		models, err := client.GetAvailableModels(context.Background(), "test-key")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(models) != 2 || models[0] != "gpt-4o" || models[1] != "gpt-4o-mini" {
			t.Errorf("unexpected models returned: %v", models)
		}
	})
}

func TestOpenAIProviderClient_Streaming(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("data: {\"choices\":[{\"delta\":{\"content\":\"{\\\"reasoning\\\":\\\"test\\\",\\\"actions\\\":[]}\"}}]}\n\ndata: [DONE]\n"))
	}))
	defer ts.Close()

	client := NewOpenAIProviderClient("openai", ts.URL, 5*time.Second, 1*time.Second, true)
	resp, err := client.Call(context.Background(), "gpt-4o", "test-key", "hello", 4096, 0.0)
	if err != nil {
		t.Fatalf("expected nil error on streaming call, got %v", err)
	}

	if !strings.Contains(string(resp), "reasoning") {
		t.Fatalf("expected response to contain 'reasoning', got %s", string(resp))
	}
}

// TestSDKMaxRetriesDisabled verifies that the SDK is configured with
// WithMaxRetries(0). A hanging server that never responds should only receive
// a single HTTP attempt (no SDK-level automatic retries). Previously, the SDK
// defaulted to 2 retries (3 total attempts), compounding with client.go's own
// retry loop into up to 9 total attempts per provider.
func TestSDKMaxRetriesDisabled(t *testing.T) {
	var requestCount int64
	done := make(chan struct{})

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/models" {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{"data": []any{}})
			return
		}
		atomic.AddInt64(&requestCount, 1)
		// Block until the test signals done. Using a test-owned channel
		// guarantees the handler returns before server.Close() is called,
		// avoiding the WaitGroup deadlock that occurs with r.Context().Done().
		<-done
	}))
	t.Cleanup(func() {
		close(done)    // unblock any in-flight handler goroutines
		server.Close() // now safe: all handlers will have returned
	})

	// Very short timeout so the test completes quickly.
	timeout := 200 * time.Millisecond
	client := newBaseOpenAIClient("openai", server.URL, server.URL, timeout, 0, false)

	ctx := context.Background()
	_, _ = client.sendCompletion(ctx, "gpt-4o", "test-key", "hello", completionOptions{})

	got := atomic.LoadInt64(&requestCount)
	// With WithMaxRetries(0) the SDK makes exactly 1 attempt. Without it the
	// SDK would make 3 attempts (initial + 2 retries) for the same context.
	if got != 1 {
		t.Errorf("expected exactly 1 HTTP request to the hung server (SDK retries disabled), got %d", got)
	}
}

// TestStreamingIdleTimeoutEnforced verifies that idle_timeout is honoured on
// the SDK streaming path. A server that accepts the connection but hangs
// without sending any response data should be cancelled after approximately
// idleTimeout instead of blocking for the full max_timeout.
func TestStreamingIdleTimeoutEnforced(t *testing.T) {
	done := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/models" {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{"data": []any{}})
			return
		}
		// Accept the connection but never send any response until done.
		<-done
	}))
	t.Cleanup(func() {
		close(done)
		server.Close()
	})

	idleTimeout := 150 * time.Millisecond
	maxTimeout := 10 * time.Second // much longer than idleTimeout
	client := newBaseOpenAIClient("openai", server.URL, server.URL, maxTimeout, idleTimeout, true)

	start := time.Now()
	ctx := context.Background()
	_, err := client.sendCompletionStreaming(ctx, "gpt-4o", "test-key", "hello", completionOptions{})
	elapsed := time.Since(start)

	if err == nil {
		t.Fatal("expected error from idle timeout, got nil")
	}
	// Should time out near idleTimeout, well before maxTimeout.
	if elapsed > 2*time.Second {
		t.Errorf("streaming call took %v; expected to fail fast near idle_timeout (%v), not near max_timeout (%v)", elapsed, idleTimeout, maxTimeout)
	}
}
