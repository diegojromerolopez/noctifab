package ensemble_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/diegojromerolopez/noctifab/pkg/domain"
	"github.com/diegojromerolopez/noctifab/pkg/infrastructure/llm/ensemble"
)

type mockLLMClient struct {
	delay      time.Duration
	resp       *domain.LLMResponse
	err        error
	callsCount int
	lastPrompt string
}

func (m *mockLLMClient) Complete(ctx context.Context, prompt string) (*domain.LLMResponse, error) {
	m.callsCount++
	m.lastPrompt = prompt
	if m.delay > 0 {
		select {
		case <-time.After(m.delay):
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	if m.err != nil {
		return nil, m.err
	}
	return m.resp, nil
}

func TestParallelClient_SpeculativeQuorum(t *testing.T) {
	fastModel1 := &mockLLMClient{
		resp: &domain.LLMResponse{
			Actions: []domain.LLMAction{
				{Tool: "write_file", Args: map[string]any{"path": "pkg/a.go", "content": "package a\nfunc A() int { return 1 }\n"}},
			},
			Usage: domain.TokenUsage{TotalTokens: 100},
		},
	}
	fastModel2 := &mockLLMClient{
		resp: &domain.LLMResponse{
			Actions: []domain.LLMAction{
				{Tool: "write_file", Args: map[string]any{"path": "pkg/b.go", "content": "package b\nfunc B() int { return 2 }\n"}},
			},
			Usage: domain.TokenUsage{TotalTokens: 150},
		},
	}
	slowModel3 := &mockLLMClient{
		delay: 500 * time.Millisecond,
		resp: &domain.LLMResponse{
			Actions: []domain.LLMAction{{Tool: "write_file", Args: map[string]any{"path": "pkg/c.go", "content": "package c"}}},
			Usage:   domain.TokenUsage{TotalTokens: 200},
		},
	}
	synth := &mockLLMClient{
		resp: &domain.LLMResponse{
			Actions: []domain.LLMAction{
				{Tool: "write_file", Args: map[string]any{"path": "pkg/synth.go", "content": "package synth\nfunc Main() {}\n"}},
			},
			Usage: domain.TokenUsage{TotalTokens: 300},
		},
	}

	models := []ensemble.NamedClient{
		{Name: "fast1", Client: fastModel1},
		{Name: "fast2", Client: fastModel2},
		{Name: "slow3", Client: slowModel3},
	}

	client := ensemble.NewParallelClient(models, synth, 2, 50*time.Millisecond, 2*time.Second, true)
	resp, err := client.Complete(context.Background(), "Implement feature X")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(resp.Actions) != 1 || resp.Actions[0].Args["path"] != "pkg/synth.go" {
		t.Errorf("unexpected synthesized actions: %+v", resp.Actions)
	}

	if resp.Usage.TotalTokens < 500 {
		t.Errorf("expected combined token usage >= 500, got %d", resp.Usage.TotalTokens)
	}
}

func TestParallelClient_FallbackToSingle(t *testing.T) {
	failingModel := &mockLLMClient{err: errors.New("provider 500")}
	synth := &mockLLMClient{
		resp: &domain.LLMResponse{
			Actions: []domain.LLMAction{{Tool: "write_file", Args: map[string]any{"path": "fallback.go", "content": "package fallback\n"}}},
		},
	}

	models := []ensemble.NamedClient{
		{Name: "fail", Client: failingModel},
	}

	client := ensemble.NewParallelClient(models, synth, 1, 0, time.Second, true)
	resp, err := client.Complete(context.Background(), "Implement fallback")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(resp.Actions) != 1 || resp.Actions[0].Args["path"] != "fallback.go" {
		t.Errorf("unexpected actions: %+v", resp.Actions)
	}
}
