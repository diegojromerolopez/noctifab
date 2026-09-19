package ensemble_test

import (
	"context"
	"testing"
	"time"

	"github.com/diegojromerolopez/noctifab/pkg/domain"
	"github.com/diegojromerolopez/noctifab/pkg/infrastructure/llm/ensemble"
)

func TestScoredClient_PromotesHighestQualityCandidate(t *testing.T) {
	weakCandidate := &mockLLMClient{
		resp: &domain.LLMResponse{
			Actions: []domain.LLMAction{
				{Tool: "write_file", Args: map[string]any{"path": "pkg/weak.go", "content": "package weak\n// TODO: implement\n"}},
			},
			Usage: domain.TokenUsage{TotalTokens: 60},
		},
	}
	strongCandidate := &mockLLMClient{
		resp: &domain.LLMResponse{
			Actions: []domain.LLMAction{
				{Tool: "write_file", Args: map[string]any{"path": "pkg/strong_test.go", "content": "package strong_test\nimport \"testing\"\nfunc TestA(t *testing.T) { if false { t.Fatal(\"error\") } }\n"}},
			},
			Usage: domain.TokenUsage{TotalTokens: 120},
		},
	}

	models := []ensemble.NamedClient{
		{Name: "weak", Client: weakCandidate},
		{Name: "strong", Client: strongCandidate},
	}

	client := ensemble.NewScoredClient(models, 2*time.Second)
	resp, err := client.Complete(context.Background(), "Write unit tests for strong")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if resp.Actions[0].Args["path"] != "pkg/strong_test.go" {
		t.Errorf("expected strong candidate to be promoted by local CPU scorer, got %v", resp.Actions[0].Args["path"])
	}
	if resp.Usage.TotalTokens != 180 {
		t.Errorf("expected combined tokens 180, got %d", resp.Usage.TotalTokens)
	}
}
