package llm

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/diegojromerolopez/noctifab/pkg/domain"
)

type mockLLM struct {
	calls int
	err   error
	resp  *domain.LLMResponse
}

func (m *mockLLM) Complete(ctx context.Context, prompt string) (*domain.LLMResponse, error) {
	m.calls++
	if m.err != nil {
		return nil, m.err
	}
	return m.resp, nil
}

func TestFailoverClient(t *testing.T) {
	t.Run("first backend succeeds immediately", func(t *testing.T) {
		m1 := &mockLLM{resp: &domain.LLMResponse{Reasoning: "m1 response"}}
		m2 := &mockLLM{resp: &domain.LLMResponse{Reasoning: "m2 response"}}

		backends := []NamedClient{
			{Name: "model-1", Client: m1},
			{Name: "model-2", Client: m2},
		}

		client := NewFailoverClient(backends, 10*time.Millisecond)
		resp, err := client.Complete(context.Background(), "hello")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if resp.Reasoning != "m1 response" {
			t.Errorf("expected m1 response, got %s", resp.Reasoning)
		}
		if m1.calls != 1 {
			t.Errorf("expected m1 to be called once, got %d", m1.calls)
		}
		if m2.calls != 0 {
			t.Errorf("expected m2 to not be called, got %d", m2.calls)
		}
	})

	t.Run("failover on transient error and sets cooldown", func(t *testing.T) {
		m1 := &mockLLM{err: errors.New("HTTP status 429: Too Many Requests")}
		m2 := &mockLLM{resp: &domain.LLMResponse{Reasoning: "m2 response"}}

		backends := []NamedClient{
			{Name: "model-1", Client: m1},
			{Name: "model-2", Client: m2},
		}

		client := NewFailoverClient(backends, 50*time.Millisecond)

		// First call: m1 fails, failover to m2
		resp, err := client.Complete(context.Background(), "hello")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if resp.Reasoning != "m2 response" {
			t.Errorf("expected m2 response, got %s", resp.Reasoning)
		}
		if m1.calls != 1 {
			t.Errorf("expected m1 calls to be 1, got %d", m1.calls)
		}
		if m2.calls != 1 {
			t.Errorf("expected m2 calls to be 1, got %d", m2.calls)
		}

		// Second call: m1 is on cooldown, should skip directly to m2
		resp, err = client.Complete(context.Background(), "hello again")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if resp.Reasoning != "m2 response" {
			t.Errorf("expected m2 response, got %s", resp.Reasoning)
		}
		if m1.calls != 1 {
			t.Errorf("expected m1 calls to remain 1 (skipped), got %d", m1.calls)
		}
		if m2.calls != 2 {
			t.Errorf("expected m2 calls to be 2, got %d", m2.calls)
		}

		// Wait for cooldown to expire
		time.Sleep(60 * time.Millisecond)

		// Third call: m1 cooldown expired, should try m1 again
		_, _ = client.Complete(context.Background(), "after cooldown")
		if m1.calls != 2 {
			t.Errorf("expected m1 to be retried after cooldown expired, calls: %d", m1.calls)
		}
	})

	t.Run("all backends fail returns error", func(t *testing.T) {
		m1 := &mockLLM{err: errors.New("error 503")}
		m2 := &mockLLM{err: errors.New("error 500")}

		backends := []NamedClient{
			{Name: "model-1", Client: m1},
			{Name: "model-2", Client: m2},
		}

		client := NewFailoverClient(backends, 10*time.Millisecond)
		_, err := client.Complete(context.Background(), "hello")
		if err == nil {
			t.Fatal("expected error, got nil")
		}

		if m1.calls != 1 || m2.calls != 1 {
			t.Errorf("expected both backends to be called once, got m1: %d, m2: %d", m1.calls, m2.calls)
		}
	})
}
