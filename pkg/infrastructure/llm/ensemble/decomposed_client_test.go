package ensemble_test

import (
	"context"
	"testing"
	"time"

	"github.com/diegojromerolopez/noctifab/pkg/domain"
	"github.com/diegojromerolopez/noctifab/pkg/infrastructure/llm/ensemble"
)

func TestDecomposedClient_ParallelSliceGeneration(t *testing.T) {
	specialistDomain := &mockLLMClient{
		resp: &domain.LLMResponse{
			Reasoning: "Domain types defined",
			Actions: []domain.LLMAction{
				{Tool: "write_file", Args: map[string]any{"path": "domain/user.go", "content": "package domain\ntype User struct{}\n"}},
			},
			Usage: domain.TokenUsage{TotalTokens: 100},
		},
	}
	specialistService := &mockLLMClient{
		resp: &domain.LLMResponse{
			Reasoning: "Service layer implemented",
			Actions: []domain.LLMAction{
				{Tool: "write_file", Args: map[string]any{"path": "service/user_service.go", "content": "package service\ntype Service struct{}\n"}},
			},
			Usage: domain.TokenUsage{TotalTokens: 150},
		},
	}

	targets := []ensemble.TargetClient{
		{Name: "domain-spec", Client: specialistDomain, RolePrompt: "Focus on domain models"},
		{Name: "service-spec", Client: specialistService, RolePrompt: "Focus on services"},
	}

	client := ensemble.NewDecomposedClient(targets, 2*time.Second)
	resp, err := client.Complete(context.Background(), "Implement User subsystem")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(resp.Actions) != 2 {
		t.Fatalf("expected 2 merged actions, got %d", len(resp.Actions))
	}
	if resp.Usage.TotalTokens != 250 {
		t.Errorf("expected combined tokens 250, got %d", resp.Usage.TotalTokens)
	}
}
