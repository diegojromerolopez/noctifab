package llm

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestReadSSEResponse_Success(t *testing.T) {
	sseData := "data: {\"choices\":[{\"delta\":{\"content\":\"Hello \"}}]}\n\ndata: {\"choices\":[{\"delta\":{\"content\":\"World\"}}]}\n\ndata: [DONE]\n"
	body := io.NopCloser(strings.NewReader(sseData))

	var received []string
	err := readSSEResponse(context.Background(), body, 2*time.Second, func(line string) error {
		received = append(received, line)
		return nil
	})

	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}

	if len(received) == 0 {
		t.Fatalf("expected non-empty received lines")
	}
}

func TestReadSSEResponse_IdleTimeout(t *testing.T) {
	pr, pw := io.Pipe()
	defer func() { _ = pr.Close() }()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	// Write one line to start
	go func() {
		_, _ = pw.Write([]byte("data: chunk1\n"))
		// Do not write anything else to trigger idle timeout
	}()

	err := readSSEResponse(ctx, pr, 100*time.Millisecond, func(line string) error {
		return nil
	})

	if err == nil {
		t.Fatalf("expected idle timeout error, got nil")
	}
	if !strings.Contains(err.Error(), "idle timeout") {
		t.Fatalf("expected idle timeout error message, got: %v", err)
	}
}

func TestOpenAIProviderClient_Streaming(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("data: {\"choices\":[{\"delta\":{\"content\":\"{\\\"reasoning\\\":\\\"test\\\",\\\"actions\\\":[]}\"}}]}\n\ndata: [DONE]\n"))
	}))
	defer ts.Close()

	client := NewOpenAIProviderClient("openai", ts.URL, 5*time.Second, 1*time.Second, true)
	resp, err := client.Call(context.Background(), "gpt-4o", "test-key", "hello")
	if err != nil {
		t.Fatalf("expected nil error on streaming call, got %v", err)
	}

	if !strings.Contains(string(resp), "reasoning") {
		t.Fatalf("expected response to contain 'reasoning', got %s", string(resp))
	}
}
