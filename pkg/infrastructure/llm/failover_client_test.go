package llm

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/diegojromerolopez/noctifab/pkg/domain"
)

type mockBudgetStore struct {
	mu           sync.Mutex
	records      map[string]int64
	incrementErr error
}

func (m *mockBudgetStore) GetDailyUsage(_ context.Context, date string, provider string) (int64, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.records[date+"|"+provider], nil
}

func (m *mockBudgetStore) IncrementUsage(_ context.Context, date string, provider string, tokens int64) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.incrementErr != nil {
		return m.incrementErr
	}
	m.records[date+"|"+provider] += tokens
	return nil
}

func newMockBudgetStore() *mockBudgetStore {
	return &mockBudgetStore{records: make(map[string]int64)}
}

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

		client := NewFailoverClient(backends, 10*time.Millisecond, 0, nil, 0)
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

		client := NewFailoverClient(backends, 50*time.Millisecond, 0, nil, 0)

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
		client := NewFailoverClient(backends, 10*time.Millisecond, 0, nil, 0)
		_, err := client.Complete(context.Background(), "hello")
		if err == nil {
			t.Fatal("expected error, got nil")

		}

		if m1.calls != 1 || m2.calls != 1 {
			t.Errorf("expected both backends to be called once, got m1: %d, m2: %d", m1.calls, m2.calls)
		}
	})

	t.Run("budget exhausted when maxCalls reached", func(t *testing.T) {
		m1 := &mockLLM{resp: &domain.LLMResponse{Reasoning: "m1 response"}}
		backends := []NamedClient{
			{Name: "model-1", Client: m1},
		}

		client := NewFailoverClient(backends, 10*time.Millisecond, 2, nil, 0)

		// First call should succeed
		resp, err := client.Complete(context.Background(), "hello")
		if err != nil {
			t.Fatalf("unexpected error on first call: %v", err)
		}
		if resp.Reasoning != "m1 response" {
			t.Errorf("expected m1 response, got %s", resp.Reasoning)
		}

		// Second call should succeed
		_, err = client.Complete(context.Background(), "hello again")
		if err != nil {
			t.Fatalf("unexpected error on second call: %v", err)
		}

		// Third call should hit budget limit
		_, err = client.Complete(context.Background(), "third time")
		if err == nil {
			t.Fatal("expected budget exhausted error, got nil")
		}
		if !errors.Is(err, domain.ErrBudgetExhausted) {
			t.Errorf("expected ErrBudgetExhausted, got: %v", err)
		}
	})

	t.Run("budget store blocks when daily token limit exceeded", func(t *testing.T) {
		m1 := &mockLLM{resp: &domain.LLMResponse{Reasoning: "m1 response"}}
		backends := []NamedClient{
			{Name: "model-1", Model: "gpt-4o", Client: m1},
		}
		store := newMockBudgetStore()
		_ = store.IncrementUsage(context.Background(), mustToday(), "gpt-4o", 10000)
		client := NewFailoverClient(backends, 10*time.Millisecond, 0, store, 5000)

		_, err := client.Complete(context.Background(), "hello")
		if err == nil {
			t.Fatal("expected token limit exhausted error, got nil")
		}
		if m1.calls != 0 {
			t.Errorf("expected backend not to be called when token limit exceeded, calls: %d", m1.calls)
		}
	})

	t.Run("budget store allows calls within token limit", func(t *testing.T) {
		m1 := &mockLLM{resp: &domain.LLMResponse{Reasoning: "m1 response"}}
		backends := []NamedClient{
			{Name: "model-1", Model: "gpt-4o", Client: m1},
		}
		store := newMockBudgetStore()
		client := NewFailoverClient(backends, 10*time.Millisecond, 0, store, 100000)

		resp, err := client.Complete(context.Background(), "hello")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if resp.Reasoning != "m1 response" {
			t.Errorf("expected m1 response, got %s", resp.Reasoning)
		}
	})

	t.Run("budget store records token usage after successful call", func(t *testing.T) {
		m1 := &mockLLM{resp: &domain.LLMResponse{Reasoning: "response", Actions: []domain.LLMAction{
			{Tool: "write_file", Args: map[string]any{"path": "test.txt"}},
		}}}
		backends := []NamedClient{
			{Name: "model-1", Model: "gpt-4o", Client: m1},
		}
		store := newMockBudgetStore()
		client := NewFailoverClient(backends, 10*time.Millisecond, 0, store, 100000)

		_, err := client.Complete(context.Background(), "Write a file called test.txt with hello world in it.")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		today := mustToday()
		usage, _ := store.GetDailyUsage(context.Background(), today, "gpt-4o")
		if usage <= 0 {
			t.Errorf("expected usage > 0 after successful call, got %d", usage)
		}
	})

	t.Run("nil budget store skips token tracking", func(t *testing.T) {
		m1 := &mockLLM{resp: &domain.LLMResponse{Reasoning: "m1 response"}}
		backends := []NamedClient{
			{Name: "model-1", Model: "gpt-4o", Client: m1},
		}
		client := NewFailoverClient(backends, 10*time.Millisecond, 0, nil, 100000)

		resp, err := client.Complete(context.Background(), "hello")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if resp.Reasoning != "m1 response" {
			t.Errorf("expected m1 response, got %s", resp.Reasoning)
		}
	})

	t.Run("budget store save error fails completion", func(t *testing.T) {
		m1 := &mockLLM{resp: &domain.LLMResponse{Reasoning: "m1 response"}}
		backends := []NamedClient{
			{Name: "model-1", Model: "gpt-4o", Client: m1},
		}
		store := newMockBudgetStore()
		store.incrementErr = fmt.Errorf("DB connection lost")
		client := NewFailoverClient(backends, 10*time.Millisecond, 0, store, 100000)

		_, err := client.Complete(context.Background(), "hello")
		if err == nil {
			t.Fatal("expected error, got nil")
		}
		if !strings.Contains(err.Error(), "failed to record token usage") && !strings.Contains(err.Error(), "DB connection lost") {
			t.Errorf("expected token save error, got: %v", err)
		}
	})
}

func mustToday() string {
	return time.Now().UTC().Format("2006-01-02")
}
