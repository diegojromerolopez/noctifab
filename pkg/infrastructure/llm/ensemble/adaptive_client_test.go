package ensemble_test

import (
	"context"
	"testing"
	"time"

	"github.com/diegojromerolopez/noctifab/pkg/domain"
	"github.com/diegojromerolopez/noctifab/pkg/infrastructure/llm/ensemble"
)

type mockTierClient struct {
	tierName string
}

func (m *mockTierClient) Complete(ctx context.Context, prompt string) (*domain.LLMResponse, error) {
	return &domain.LLMResponse{
		Reasoning: "Executed by " + m.tierName,
		Actions: []domain.LLMAction{
			{
				Tool: "write_file",
				Args: map[string]any{"tier": m.tierName},
			},
		},
	}, nil
}

func TestAdaptiveClient_Routing(t *testing.T) {
	fast := &mockTierClient{tierName: "fast"}
	standard := &mockTierClient{tierName: "standard"}
	heavy := &mockTierClient{tierName: "heavy"}

	adaptive := ensemble.NewAdaptiveClient(fast, standard, heavy, 5*time.Second)

	// 1. Fast Path (docs/comments/typos)
	respFast, err := adaptive.Complete(context.Background(), "Please fix typo and update docstring in README")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if respFast.Actions[0].Args["tier"] != "fast" {
		t.Errorf("expected fast tier, got %v", respFast.Actions[0].Args["tier"])
	}

	// 2. Heavy Path (concurrency/architecture/system calls/remediation)
	respHeavy, err := adaptive.Complete(context.Background(), "Implement lock-free btree storage engine with asyncio mutex and sys_mmap syscall")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if respHeavy.Actions[0].Args["tier"] != "heavy" {
		t.Errorf("expected heavy tier, got %v", respHeavy.Actions[0].Args["tier"])
	}

	// 3. Standard Path (general feature)
	respStd, err := adaptive.Complete(context.Background(), "Implement User model validation and email verification helper")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if respStd.Actions[0].Args["tier"] != "standard" {
		t.Errorf("expected standard tier, got %v", respStd.Actions[0].Args["tier"])
	}
}
