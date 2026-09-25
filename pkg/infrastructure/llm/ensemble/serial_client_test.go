package ensemble_test

import (
	"context"
	"testing"
	"time"

	"github.com/diegojromerolopez/noctifab/pkg/domain"
	"github.com/diegojromerolopez/noctifab/pkg/infrastructure/llm/ensemble"
)

func TestSerialClient_EarlyExit(t *testing.T) {
	stage1Clean := &mockLLMClient{
		resp: &domain.LLMResponse{
			Actions: []domain.LLMAction{
				{Tool: "write_file", Args: map[string]any{"path": "pkg/valid.go", "content": "package valid\nfunc Hello() string { return \"world\" }\n"}},
			},
			Usage: domain.TokenUsage{TotalTokens: 120},
		},
	}
	stage2ShouldSkip := &mockLLMClient{
		resp: &domain.LLMResponse{
			Actions: []domain.LLMAction{
				{Tool: "write_file", Args: map[string]any{"path": "pkg/stage2.go", "content": "package stage2\n"}},
			},
		},
	}

	stages := []ensemble.StageClient{
		{Name: "stage1", Client: stage1Clean},
		{Name: "stage2", Client: stage2ShouldSkip},
	}

	client := ensemble.NewSerialClient(stages, true, true, 5*time.Second)
	resp, err := client.Complete(context.Background(), "Implement valid Go file")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if stage2ShouldSkip.callsCount != 0 {
		t.Errorf("expected stage 2 to be skipped on early exit, but called %d times", stage2ShouldSkip.callsCount)
	}

	if resp.Actions[0].Args["path"] != "pkg/valid.go" {
		t.Errorf("unexpected response path: %v", resp.Actions[0].Args["path"])
	}
}

func TestSerialClient_RefinementOnStubs(t *testing.T) {
	stage1Stub := &mockLLMClient{
		resp: &domain.LLMResponse{
			Actions: []domain.LLMAction{
				{Tool: "write_file", Args: map[string]any{"path": "pkg/stub.go", "content": "package stub\nfunc Hello() string { panic(\"TODO\") }\n"}},
			},
			Usage: domain.TokenUsage{TotalTokens: 100},
		},
	}
	stage2Refined := &mockLLMClient{
		resp: &domain.LLMResponse{
			Actions: []domain.LLMAction{
				{Tool: "write_file", Args: map[string]any{"path": "pkg/refined.go", "content": "package stub\nfunc Hello() string { return \"done\" }\n"}},
			},
			Usage: domain.TokenUsage{TotalTokens: 200},
		},
	}

	stages := []ensemble.StageClient{
		{Name: "stage1", Client: stage1Stub},
		{Name: "stage2", Client: stage2Refined},
	}

	client := ensemble.NewSerialClient(stages, true, true, 5*time.Second)
	resp, err := client.Complete(context.Background(), "Implement Hello")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if stage2Refined.callsCount != 1 {
		t.Errorf("expected stage 2 to be invoked on stubs, got %d calls", stage2Refined.callsCount)
	}

	if resp.Actions[0].Args["path"] != "pkg/refined.go" {
		t.Errorf("unexpected refined path: %v", resp.Actions[0].Args["path"])
	}
	if resp.Usage.TotalTokens != 300 {
		t.Errorf("expected combined token usage 300, got %d", resp.Usage.TotalTokens)
	}
}
