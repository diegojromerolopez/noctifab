package llm

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestClient_ImmediateFailoverOnRateLimit429 verifies that an HTTP 429 rate limit
// immediately aborts the retry ladder and does not perform lower-model fallbacks,
// failing fast so the router can immediately switch providers.
func TestClient_ImmediateFailoverOnRateLimit429(t *testing.T) {
	var calls int64
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/models" {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"object":"list","data":[{"id":"gpt-4o","object":"model"},{"id":"gpt-4o-mini","object":"model"}]}`))
			return
		}
		atomic.AddInt64(&calls, 1)
		w.Header().Set("Retry-After", "45")
		w.WriteHeader(http.StatusTooManyRequests)
		_, _ = w.Write([]byte(`{"error":{"message":"Rate limit exceeded. Please retry after 45 seconds.","type":"rate_limit_error","code":429}}`))
	}))
	t.Cleanup(srv.Close)

	c := &Client{
		Provider:    "openai",
		Model:       "gpt-4o",
		APIKey:      "sk-testkey",
		URL:         srv.URL,
		Timeout:     5 * time.Second,
		IdleTimeout: 5 * time.Second,
		MaxRetries:  5,
		Backoff:     1 * time.Second,
		Streaming:   false,
	}

	start := time.Now()
	_, err := c.Complete(context.Background(), "hello world")
	elapsed := time.Since(start)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "429")

	// Verify failover happened immediately in < 500ms, NOT waiting for 45s Retry-After or 5 retries
	assert.Less(t, elapsed, 500*time.Millisecond, "expected immediate failover in < 500ms, but took %v", elapsed)

	// Verify exactly 1 call was made (no retries, no lower-model attempts on throttled provider)
	gotCalls := atomic.LoadInt64(&calls)
	assert.Equal(t, int64(1), gotCalls, "expected exactly 1 call on rate-limited provider, got %d", gotCalls)
}

// TestRouter_ImmediateFailoverOnRateLimit429 verifies that when candidate 1 hits
// a 429 rate limit, the router immediately fails over to candidate 2 in < 500ms.
func TestRouter_ImmediateFailoverOnRateLimit429(t *testing.T) {
	var cand1Calls int64
	srvThrottled := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/models" {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"object":"list","data":[{"id":"gpt-4o","object":"model"}]}`))
			return
		}
		atomic.AddInt64(&cand1Calls, 1)
		w.Header().Set("Retry-After", "30")
		w.WriteHeader(http.StatusTooManyRequests)
		_, _ = w.Write([]byte(`{"error":{"message":"Rate limit reached","type":"tokens"}}`))
	}))
	t.Cleanup(srvThrottled.Close)

	var cand2Calls int64
	srvHealthy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/models" {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"object":"list","data":[{"id":"gpt-4o","object":"model"}]}`))
			return
		}
		atomic.AddInt64(&cand2Calls, 1)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{
			"id": "chatcmpl-test",
			"choices": [{
				"message": {
					"role": "assistant",
					"content": "{\"thought\":\"ok\",\"actions\":[{\"tool\":\"run_tests\",\"args\":{}}]}"
				}
			}],
			"usage": {"prompt_tokens": 10, "completion_tokens": 10, "total_tokens": 20}
		}`))
	}))
	t.Cleanup(srvHealthy.Close)

	cThrottled := &Client{
		Provider:    "openai",
		Model:       "gpt-4o",
		APIKey:      "sk-cand1",
		URL:         srvThrottled.URL,
		Timeout:     5 * time.Second,
		IdleTimeout: 5 * time.Second,
		MaxRetries:  5,
		Backoff:     1 * time.Second,
		Streaming:   false,
	}

	cHealthy := &Client{
		Provider:    "openai",
		Model:       "gpt-4o",
		APIKey:      "sk-cand2",
		URL:         srvHealthy.URL,
		Timeout:     5 * time.Second,
		IdleTimeout: 5 * time.Second,
		Streaming:   false,
	}

	router := NewResilientLLMRouter(nil, nil)
	seedRouterCandidates(router, "generator", []RouterCandidate{
		{Name: "candidate-throttled", Provider: "openai", Model: "gpt-4o", Client: cThrottled},
		{Name: "candidate-healthy", Provider: "openai", Model: "gpt-4o", Client: cHealthy},
	})

	ctx := WithRoleContext(context.Background(), "generator")
	start := time.Now()
	resp, err := router.Complete(ctx, "run generator")
	elapsed := time.Since(start)

	require.NoError(t, err)
	require.NotNil(t, resp)
	assert.Len(t, resp.Actions, 1)
	assert.Equal(t, "run_tests", resp.Actions[0].Tool)

	// Immediate failover should complete in under 500ms
	assert.Less(t, elapsed, 500*time.Millisecond, "expected total failover time < 500ms, took %v", elapsed)
	assert.Equal(t, int64(1), atomic.LoadInt64(&cand1Calls))
	assert.Equal(t, int64(1), atomic.LoadInt64(&cand2Calls))

	// Verify throttled candidate is placed on cooldown
	router.mu.RLock()
	_, onCooldown := router.cooldowns["candidate-throttled"]
	router.mu.RUnlock()
	assert.True(t, onCooldown, "expected throttled candidate to be placed on cooldown")
}
