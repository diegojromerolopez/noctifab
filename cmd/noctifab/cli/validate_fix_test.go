package cli_test

import (
	"context"
	"errors"
	"strings"
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

func TestRepairConfigWithAIAndExplanation(t *testing.T) {
	brokenYAML := "config_version: '1.0'\n"
	fixedYAML := "EXPLANATION:\n- Upgraded version to 2.0\n\n```yaml\nconfig_version: '2.0'\n```"

	mockClient := &mockRepairLLM{responseYAML: "config_version: '2.0'"}
	mockClient.err = nil
	// Override Complete directly for custom formatting
	customClient := &customMockRepairLLM{content: fixedYAML}

	repaired, explanation, err := cli.RepairConfigWithAIAndExplanation(context.Background(), brokenYAML, errors.New("invalid version"), customClient)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !strings.Contains(repaired, "config_version: '2.0'") {
		t.Errorf("expected repaired config_version: '2.0', got %s", repaired)
	}
	if !strings.Contains(explanation, "Upgraded version to 2.0") {
		t.Errorf("expected explanation to contain upgrade text, got %q", explanation)
	}
}

type customMockRepairLLM struct {
	content string
}

func (c *customMockRepairLLM) Complete(ctx context.Context, prompt string) (*domain.LLMResponse, error) {
	return &domain.LLMResponse{
		Reasoning: "Completed repair",
		Actions: []domain.LLMAction{
			{
				Tool: "write_file",
				Args: map[string]any{"content": c.content},
			},
		},
	}, nil
}
