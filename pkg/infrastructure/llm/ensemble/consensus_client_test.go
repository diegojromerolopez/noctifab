package ensemble_test

import (
	"context"
	"testing"
	"time"

	"github.com/diegojromerolopez/noctifab/pkg/domain"
	"github.com/diegojromerolopez/noctifab/pkg/infrastructure/llm/ensemble"
)

func TestConsensusClient_UnanimousPass(t *testing.T) {
	voter1 := &mockLLMClient{
		resp: &domain.LLMResponse{
			Actions: []domain.LLMAction{
				{Tool: "submit_story_qa_audit", Args: map[string]any{"passed": true, "reason": "all tests pass"}},
			},
			Usage: domain.TokenUsage{TotalTokens: 100},
		},
	}
	voter2 := &mockLLMClient{
		resp: &domain.LLMResponse{
			Actions: []domain.LLMAction{
				{Tool: "submit_story_qa_audit", Args: map[string]any{"passed": true, "reason": "contracts verified"}},
			},
			Usage: domain.TokenUsage{TotalTokens: 110},
		},
	}
	tieBreaker := &mockLLMClient{
		resp: &domain.LLMResponse{
			Actions: []domain.LLMAction{
				{Tool: "submit_story_qa_audit", Args: map[string]any{"passed": true}},
			},
		},
	}

	voters := []ensemble.NamedClient{
		{Name: "voter1", Client: voter1},
		{Name: "voter2", Client: voter2},
	}

	client := ensemble.NewConsensusClient(voters, tieBreaker, 2*time.Second)
	resp, err := client.Complete(context.Background(), "Audit Story 1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if tieBreaker.callsCount != 0 {
		t.Errorf("expected tie-breaker to be skipped on unanimous vote, got %d calls", tieBreaker.callsCount)
	}

	passed, ok := resp.Actions[0].Args["passed"].(bool)
	if !ok || !passed {
		t.Errorf("expected passed=true in audit response")
	}
	if resp.Usage.TotalTokens != 210 {
		t.Errorf("expected total tokens 210, got %d", resp.Usage.TotalTokens)
	}
}

func TestConsensusClient_SplitDivergence(t *testing.T) {
	voter1Pass := &mockLLMClient{
		resp: &domain.LLMResponse{
			Actions: []domain.LLMAction{
				{Tool: "submit_story_qa_audit", Args: map[string]any{"passed": true}},
			},
			Usage: domain.TokenUsage{TotalTokens: 100},
		},
	}
	voter2Fail := &mockLLMClient{
		resp: &domain.LLMResponse{
			Actions: []domain.LLMAction{
				{Tool: "submit_story_qa_audit", Args: map[string]any{"passed": false, "issues": []any{"missing flag"}}},
			},
			Usage: domain.TokenUsage{TotalTokens: 120},
		},
	}
	tieBreakerFinal := &mockLLMClient{
		resp: &domain.LLMResponse{
			Actions: []domain.LLMAction{
				{Tool: "submit_story_qa_audit", Args: map[string]any{"passed": false, "reason": "confirmed missing flag"}},
			},
			Usage: domain.TokenUsage{TotalTokens: 150},
		},
	}

	voters := []ensemble.NamedClient{
		{Name: "voter1", Client: voter1Pass},
		{Name: "voter2", Client: voter2Fail},
	}

	client := ensemble.NewConsensusClient(voters, tieBreakerFinal, 2*time.Second)
	resp, err := client.Complete(context.Background(), "Audit Story 2")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if tieBreakerFinal.callsCount != 1 {
		t.Errorf("expected tie-breaker to be invoked on split vote, got %d calls", tieBreakerFinal.callsCount)
	}

	passed, ok := resp.Actions[0].Args["passed"].(bool)
	if !ok || passed {
		t.Errorf("expected final tie-breaker verdict passed=false, got %v", passed)
	}
	if resp.Usage.TotalTokens != 370 {
		t.Errorf("expected total tokens 370 (100+120+150), got %d", resp.Usage.TotalTokens)
	}
}
