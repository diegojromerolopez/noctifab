package llm

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// TestOpenAIStream_TerminatesOnDoneMarker is a regression test for the SSE
// hang: OpenRouter sends `data: [DONE]` and then closes the connection. The
// official SDK terminates on the marker + EOF, so Complete must return
// promptly rather than waiting out a deadline.
func TestOpenAIStream_TerminatesOnDoneMarker(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		fl, _ := w.(http.Flusher)
		_, _ = w.Write([]byte("data: {\"choices\":[{\"delta\":{\"content\":\"{\\\"reasoning\\\":\\\"ok\\\",\\\"actions\\\":[]}\"}}]}\n\n"))
		fl.Flush()
		_, _ = w.Write([]byte("data: [DONE]\n\n"))
		fl.Flush()
	}))
	defer srv.Close()

	c := &Client{
		Provider:    "openrouter",
		Model:       "x",
		APIKey:      "k",
		URL:         srv.URL,
		Timeout:     8 * time.Second,
		IdleTimeout: 5 * time.Second,
		Streaming:   true,
	}

	start := time.Now()
	resp, err := c.Complete(context.Background(), "hi")
	elapsed := time.Since(start)
	if err != nil {
		t.Fatalf("Complete returned error: %v", err)
	}
	if resp == nil || resp.Reasoning != "ok" {
		t.Fatalf("unexpected response: %+v", resp)
	}
	if elapsed >= 5*time.Second {
		t.Errorf("stream hung %.0fs waiting after [DONE]; should return immediately", elapsed.Seconds())
	}
}
