package cli_test

import (
	"context"
	"errors"
	"testing"

	"github.com/diegojromerolopez/noctifab/cmd/noctifab/cli"
	"github.com/diegojromerolopez/noctifab/pkg/domain"
	"github.com/diegojromerolopez/noctifab/pkg/infrastructure/config"
)

type mockRepairLLM struct {
	responseYAML string
	err          error
}

func (m *mockRepairLLM) Complete(ctx context.Context, prompt string) (*domain.LLMResponse, error) {
	if m.err != nil {
		return nil, m.err
	}
	return &domain.LLMResponse{
		Reasoning: "Fixed configuration syntax and ensemble models key",
		Actions: []domain.LLMAction{
			{
				Tool: "write_file",
				Args: map[string]any{
					"content": "```yaml\n" + m.responseYAML + "\n```",
				},
			},
		},
	}, nil
}

func TestRepairConfigWithAI_Success(t *testing.T) {
	brokenYAML := `
config_version: '2.0'
agents:
  product_manager:
    ensemble:
      stratgy: "parralel"
      min_proposers: 2
`
	fixedYAML := `
config_version: '2.0'
agents:
  product_manager:
    ensemble:
      strategy: "parallel"
      min_models: 2
      models:
        - name: "claude"
`

	mockClient := &mockRepairLLM{responseYAML: fixedYAML}
	parseErr := errors.New("field stratgy not found in type config.EnsembleConfig")

	repaired, err := cli.RepairConfigWithAI(context.Background(), brokenYAML, parseErr, mockClient)
	if err != nil {
		t.Fatalf("unexpected error from RepairConfigWithAI: %v", err)
	}

	// Validate that repaired YAML decodes cleanly
	cfg, err := config.ValidateBytes([]byte(repaired))
	if err != nil {
		t.Fatalf("repaired YAML failed config validation: %v", err)
	}

	if cfg.Agents.ProductManager.Ensemble.Strategy != config.EnsembleStrategyParallel {
		t.Errorf("expected strategy parallel, got %s", cfg.Agents.ProductManager.Ensemble.Strategy)
	}
	if cfg.Agents.ProductManager.Ensemble.MinModels != 2 {
		t.Errorf("expected min_models 2, got %d", cfg.Agents.ProductManager.Ensemble.MinModels)
	}
}
