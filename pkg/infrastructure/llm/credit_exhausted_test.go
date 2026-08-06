package llm

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

// newCreditServer returns an httptest server that always answers HTTP 402 with
// a credit-limit body, and a counter of how many completion requests it served.
func newCreditServer(t *testing.T) (*httptest.Server, *int64) {
	t.Helper()
	var calls int64
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/models" {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"object":"list","data":[{"id":"gpt-4o","object":"model"}]}`))
			return
		}
		atomic.AddInt64(&calls, 1)
		w.WriteHeader(http.StatusPaymentRequired)
		_, _ = w.Write([]byte(`{"error":{"message":"You have depleted your monthly included credits","type":"openrouter_key_limit"}}`))
	}))
	t.Cleanup(srv.Close)
	return srv, &calls
}

// TestComplete_SkipOnCreditExhausted verifies that when llm.skip_on_credit_exhausted
// is enabled, an HTTP 402 credit-limit response is treated as a hard failure:
// no retries, no lower-model fallback, and ErrCreditExhausted is surfaced so the
// router can move to the next provider in priority immediately.
func TestComplete_SkipOnCreditExhausted(t *testing.T) {
	srv, calls := newCreditServer(t)

	c := &Client{
		Provider:              "openai",
		Model:                 "gpt-4o",
		APIKey:                "testkey",
		URL:                   srv.URL,
		Timeout:               2 * time.Second,
		IdleTimeout:           2 * time.Second,
		Streaming:             false,
		SkipOnCreditExhausted: true,
	}

	_, err := c.Complete(context.Background(), "do the thing")
	if err == nil {
		t.Fatal("expected an error for credit exhaustion")
	}
	if !errors.Is(err, ErrCreditExhausted) {
		t.Errorf("expected ErrCreditExhausted, got: %v", err)
	}
	if got := atomic.LoadInt64(calls); got != 1 {
		t.Errorf("expected exactly 1 completion request (fast-fail, no retries), got %d", got)
	}
}

// TestComplete_CreditExhaustedRetriesWithSkipDisabled verifies that when
// skip_on_credit_exhausted is disabled, the client falls through to normal
// retry behavior (more than one attempt) instead of fast-failing.
func TestComplete_CreditExhaustedRetriesWithSkipDisabled(t *testing.T) {
	srv, calls := newCreditServer(t)

	c := &Client{
		Provider:              "openai",
		Model:                 "gpt-4o",
		APIKey:                "skkey",
		URL:                   srv.URL,
		Timeout:               2 * time.Second,
		IdleTimeout:           2 * time.Second,
		Streaming:             false,
		MaxRetries:            2,
		Backoff:               1 * time.Millisecond,
		SkipOnCreditExhausted: false,
	}

	_, err := c.Complete(context.Background(), "do the thing")
	if err == nil {
		t.Fatal("expected an error for credit exhaustion")
	}
	if errors.Is(err, ErrCreditExhausted) {
		t.Errorf("expected a generic fallback error (skip disabled), got ErrCreditExhausted: %v", err)
	}
	// maxRetries=2 => attempt 0,1,2 => 3 requests.
	if got := atomic.LoadInt64(calls); got != 3 {
		t.Errorf("expected 3 requests when skip disabled, got %d", got)
	}
}

// TestIsCreditExhausted verifies the detection helper across 402, credit-429,
// and rate-limit-only 429 bodies.
func TestIsCreditExhausted(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want bool
	}{
		{"http 402", &httpError{StatusCode: 402, Body: `{"error":"out of credits"}`, Header: http.Header{}}, true},
		{"429 with credit word", &httpError{StatusCode: 429, Body: `{"error":"You have depleted your monthly included credits"}`, Header: http.Header{}}, true},
		{"429 pure rate limit", &httpError{StatusCode: 429, Body: `{"error":"rate limited"}`, Header: http.Header{}}, false},
		{"200 with quota", &httpError{StatusCode: 200, Body: `quota`, Header: http.Header{}}, false},
		{"plain error", errors.New("boom"), false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := isCreditExhausted(tc.err); got != tc.want {
				t.Errorf("isCreditExhausted(%v) = %v; want %v", tc.err, got, tc.want)
			}
		})
	}
}
