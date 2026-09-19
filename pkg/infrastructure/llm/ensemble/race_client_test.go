package ensemble_test

import (
	"context"
	"testing"
	"time"

	"github.com/diegojromerolopez/noctifab/pkg/domain"
	"github.com/diegojromerolopez/noctifab/pkg/infrastructure/llm/ensemble"
)

func TestRaceClient_FastestValidWins(t *testing.T) {
	fastValid := &mockLLMClient{
		delay: 10 * time.Millisecond,
		resp: &domain.LLMResponse{
			Actions: []domain.LLMAction{
				{Tool: "write_file", Args: map[string]any{"path": "pkg/fast.go", "content": "package fast\nfunc Fast() int { return 1 }\n"}},
			},
			Usage: domain.TokenUsage{TotalTokens: 80},
		},
	}
	slowModel := &mockLLMClient{
		delay: 300 * time.Millisecond,
		resp: &domain.LLMResponse{
			Actions: []domain.LLMAction{
				{Tool: "write_file", Args: map[string]any{"path": "pkg/slow.go", "content": "package slow\nfunc Slow() int { return 2 }\n"}},
			},
			Usage: domain.TokenUsage{TotalTokens: 150},
		},
	}

	models := []ensemble.NamedClient{
		{Name: "fast", Client: fastValid},
		{Name: "slow", Client: slowModel},
	}

	client := ensemble.NewRaceClient(models, 1*time.Second)
	resp, err := client.Complete(context.Background(), "Implement Fast")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if resp.Actions[0].Args["path"] != "pkg/fast.go" {
		t.Errorf("expected fastest model to win, got %v", resp.Actions[0].Args["path"])
	}
}
