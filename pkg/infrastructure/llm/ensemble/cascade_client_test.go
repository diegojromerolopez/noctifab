package ensemble_test

import (
	"context"
	"testing"
	"time"

	"github.com/diegojromerolopez/noctifab/pkg/domain"
	"github.com/diegojromerolopez/noctifab/pkg/infrastructure/llm/ensemble"
)

func TestCascadeClient_FastTierPass(t *testing.T) {
	tier1Fast := &mockLLMClient{
		resp: &domain.LLMResponse{
			Actions: []domain.LLMAction{
				{Tool: "write_file", Args: map[string]any{"path": "pkg/simple.go", "content": "package simple\nfunc Add(a, b int) int { return a + b }\n"}},
			},
			Usage: domain.TokenUsage{TotalTokens: 50},
		},
	}
	tier2Frontier := &mockLLMClient{
		resp: &domain.LLMResponse{
			Actions: []domain.LLMAction{
				{Tool: "write_file", Args: map[string]any{"path": "pkg/frontier.go", "content": "package simple\n"}},
			},
		},
	}

	tiers := []ensemble.NamedClient{
		{Name: "tier1", Client: tier1Fast},
		{Name: "tier2", Client: tier2Frontier},
	}

	client := ensemble.NewCascadeClient(tiers, 2*time.Second)
	resp, err := client.Complete(context.Background(), "Implement simple add")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if tier2Frontier.callsCount != 0 {
		t.Errorf("expected tier 2 to be skipped when tier 1 succeeds, got %d calls", tier2Frontier.callsCount)
	}
	if resp.Actions[0].Args["path"] != "pkg/simple.go" {
		t.Errorf("unexpected path: %v", resp.Actions[0].Args["path"])
	}
}

func TestCascadeClient_EscalatesToFrontierOnStubs(t *testing.T) {
	tier1Stub := &mockLLMClient{
		resp: &domain.LLMResponse{
			Actions: []domain.LLMAction{
				{Tool: "write_file", Args: map[string]any{"path": "pkg/complex.go", "content": "package complex\nfunc Solve() { panic(\"unimplemented\") }\n"}},
			},
			Usage: domain.TokenUsage{TotalTokens: 50},
		},
	}
	tier2Frontier := &mockLLMClient{
		resp: &domain.LLMResponse{
			Actions: []domain.LLMAction{
				{Tool: "write_file", Args: map[string]any{"path": "pkg/complex.go", "content": "package complex\nfunc Solve() int { return 42 }\n"}},
			},
			Usage: domain.TokenUsage{TotalTokens: 150},
		},
	}

	tiers := []ensemble.NamedClient{
		{Name: "tier1", Client: tier1Stub},
		{Name: "tier2", Client: tier2Frontier},
	}

	client := ensemble.NewCascadeClient(tiers, 2*time.Second)
	resp, err := client.Complete(context.Background(), "Implement complex solver")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if tier2Frontier.callsCount != 1 {
		t.Errorf("expected tier 2 to be invoked on tier 1 stubs, got %d calls", tier2Frontier.callsCount)
	}
	if resp.Usage.TotalTokens != 200 {
		t.Errorf("expected total tokens 200, got %d", resp.Usage.TotalTokens)
	}
}
