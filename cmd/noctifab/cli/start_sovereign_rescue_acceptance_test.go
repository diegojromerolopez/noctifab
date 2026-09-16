package cli

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/diegojromerolopez/noctifab/pkg/domain"
	"github.com/diegojromerolopez/noctifab/pkg/infrastructure/config"
	"github.com/diegojromerolopez/noctifab/pkg/services"
)

func TestSovereignRescue_AcceptanceGapsInPrompt(t *testing.T) {
	gaps := []string{
		"Missing E2E test scenario for AOF persistence",
		"Missing requirements.txt",
	}
	prompt := buildSovereignRescuePrompt("Build Redis store", []string{"US-001 (failed)"}, gaps, "error log", 1, 2)

	assert.Contains(t, prompt, "WHOLE-PROJECT ACCEPTANCE AUDIT GAPS (REQUIRED REMEDIATION)")
	assert.Contains(t, prompt, "Missing E2E test scenario for AOF persistence")
	assert.Contains(t, prompt, "Missing requirements.txt")
}

func TestSovereignRescue_PostValidationFunc(t *testing.T) {
	t.Run("when unit tests pass but PostValidationFunc fails, sovereign rescue continues to next turn", func(t *testing.T) {
		tempDir := t.TempDir()
		mockRepo := &mockDAGStateRepo{
			state: &domain.State{ProjectPath: tempDir},
		}

		mockLLM := &mockRescueLLM{
			responses: []*domain.LLMResponse{
				{Actions: []domain.LLMAction{{Tool: "noop"}}},
				{Actions: []domain.LLMAction{{Tool: "noop"}}},
			},
		}
		reg := services.NewToolRegistry()
		reg.Register(&services.NoopTool{})

		sandbox := &mockRescueSandbox{
			runFunc: func(ctx context.Context, dir string, cmd string, pkg string) (string, error) {
				return "OK (all tests passed)", nil
			},
		}
		validator := services.NewTestValidator(sandbox, false, mockLLM, reg.Tools())

		postValidationCalls := 0
		opts := SovereignRescueOptions{
			TargetDir:    tempDir,
			Cfg:          config.DefaultConfig(),
			Repo:         mockRepo,
			LLMClient:    mockLLM,
			ToolRegistry: reg,
			Validator:    validator,
			MaxTurns:     2,
			PostValidationFunc: func(ctx context.Context, state *domain.State) (bool, string) {
				postValidationCalls++
				if postValidationCalls == 1 {
					return false, "Missing E2E test suite"
				}
				return true, ""
			},
		}

		err := DispatchSovereignRescue(context.Background(), opts)
		require.NoError(t, err)
		assert.Equal(t, 2, postValidationCalls, "expected post-validation to be called on each turn")
		assert.Equal(t, 2, mockLLM.calls, "expected 2 turns because turn 1 post-validation failed")

		// Verify Turn 2 prompt included the acceptance failure
		require.GreaterOrEqual(t, len(mockLLM.prompts), 2)
		assert.Contains(t, mockLLM.prompts[1], "Missing E2E test suite")
	})

	t.Run("when PostValidationFunc passes on turn 1, sovereign rescue succeeds immediately", func(t *testing.T) {
		tempDir := t.TempDir()
		mockRepo := &mockDAGStateRepo{
			state: &domain.State{ProjectPath: tempDir},
		}

		mockLLM := &mockRescueLLM{
			responses: []*domain.LLMResponse{
				{Actions: []domain.LLMAction{{Tool: "noop"}}},
			},
		}
		reg := services.NewToolRegistry()
		reg.Register(&services.NoopTool{})

		sandbox := &mockRescueSandbox{
			runFunc: func(ctx context.Context, dir string, cmd string, pkg string) (string, error) {
				return "OK", nil
			},
		}
		validator := services.NewTestValidator(sandbox, false, mockLLM, reg.Tools())

		opts := SovereignRescueOptions{
			TargetDir:    tempDir,
			Cfg:          config.DefaultConfig(),
			Repo:         mockRepo,
			LLMClient:    mockLLM,
			ToolRegistry: reg,
			Validator:    validator,
			MaxTurns:     2,
			PostValidationFunc: func(ctx context.Context, state *domain.State) (bool, string) {
				return true, ""
			},
		}

		err := DispatchSovereignRescue(context.Background(), opts)
		require.NoError(t, err)
		assert.Equal(t, 1, mockLLM.calls, "expected 1 turn when post-validation passes immediately")
	})
}
