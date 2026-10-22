package services

import (
	"context"
	"testing"

	"github.com/diegojromerolopez/noctifab/pkg/domain"
	"github.com/diegojromerolopez/noctifab/pkg/infrastructure/config"
)

type mockSpikeHypothesesLLMClient struct {
	responses []*domain.LLMResponse
	callIndex int
}

func (m *mockSpikeHypothesesLLMClient) Complete(ctx context.Context, prompt string) (*domain.LLMResponse, error) {
	if m.callIndex < len(m.responses) {
		res := m.responses[m.callIndex]
		m.callIndex++
		return res, nil
	}
	return &domain.LLMResponse{}, nil
}

func TestResolveSpikeHypotheses_Correct(t *testing.T) {
	cand1 := SpikeCandidate{
		Index:   1,
		Valid:   true,
		FileCnt: 2,
		Files: map[string]string{
			"main.go": "package main\nfunc main() {}\n",
			"go.mod":  "module app\ngo 1.22\n",
		},
	}
	cand2 := SpikeCandidate{
		Index:   2,
		Valid:   true,
		FileCnt: 3,
		Files: map[string]string{
			"main.go":      "package main\nfunc main() {}\n",
			"go.mod":       "module app\ngo 1.22\n",
			"main_test.go": "package main\nimport \"testing\"\n",
		},
	}

	res := ResolveSpikeHypotheses([]SpikeCandidate{cand1, cand2}, config.ResolutionCorrect)
	if len(res) != 3 {
		t.Fatalf("expected candidate 2 with 3 files, got %d", len(res))
	}
	if _, ok := res["main_test.go"]; !ok {
		t.Errorf("expected main_test.go in winner")
	}
}

func TestResolveSpikeHypotheses_Mixture(t *testing.T) {
	cand1 := SpikeCandidate{
		Index:   1,
		Valid:   true,
		FileCnt: 2,
		Files: map[string]string{
			"src/lib.rs": "pub fn add(a: i32, b: i32) -> i32 { a + b }",
			"Cargo.toml": "[package]\nname=\"app\"\n",
		},
	}
	cand2 := SpikeCandidate{
		Index:   2,
		Valid:   true,
		FileCnt: 2,
		Files: map[string]string{
			"tests/test_lib.rs": "#[test] fn test_add() {}",
			"Cargo.toml":        "[package]\nname=\"app\"\n",
		},
	}

	res := ResolveSpikeHypotheses([]SpikeCandidate{cand1, cand2}, config.ResolutionMixture)
	// Mixture should union the non-overlapping files (3 distinct files)
	if len(res) != 3 {
		t.Fatalf("expected mixture of 3 files, got %d", len(res))
	}
	if _, ok := res["src/lib.rs"]; !ok {
		t.Errorf("expected src/lib.rs in mixture")
	}
	if _, ok := res["tests/test_lib.rs"]; !ok {
		t.Errorf("expected tests/test_lib.rs in mixture")
	}
}

func TestGenerateSpikeHypotheses(t *testing.T) {
	mockLLM := &mockSpikeHypothesesLLMClient{
		responses: []*domain.LLMResponse{
			{
				Actions: []domain.LLMAction{
					{Tool: "write_file", Args: map[string]any{"path": "a.txt", "content": "hello"}},
				},
			},
			{
				Actions: []domain.LLMAction{
					{Tool: "write_file", Args: map[string]any{"path": "b.txt", "content": "world"}},
				},
			},
		},
	}

	hypCfg := config.HypothesesConfig{
		Enabled:    true,
		Agents:     2,
		Resolution: config.ResolutionMixture,
	}

	candidates := GenerateSpikeHypotheses(context.Background(), "prompt", mockLLM, hypCfg, 5)
	if len(candidates) != 2 {
		t.Fatalf("expected 2 candidates, got %d", len(candidates))
	}
}
