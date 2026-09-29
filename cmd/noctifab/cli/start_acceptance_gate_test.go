package cli

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/diegojromerolopez/noctifab/pkg/domain"
	"github.com/diegojromerolopez/noctifab/pkg/infrastructure/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type mockGateLLM struct {
	response *domain.LLMResponse
	err      error
}

func (m *mockGateLLM) Complete(ctx context.Context, prompt string) (*domain.LLMResponse, error) {
	return m.response, m.err
}

func TestRunWholeProjectAcceptanceGate(t *testing.T) {
	t.Run("when repo is nil, it returns nil gracefully", func(t *testing.T) {
		err := RunWholeProjectAcceptanceGate(context.Background(), AcceptanceGateOptions{Repo: nil})
		assert.NoError(t, err)
	})

	t.Run("when SPEC.md does not exist, acceptance audit passes gracefully", func(t *testing.T) {
		tempDir := t.TempDir()
		repo := &mockStateRepo{state: &domain.State{ProjectPath: tempDir}}
		cfg := config.DefaultConfig()

		err := RunWholeProjectAcceptanceGate(context.Background(), AcceptanceGateOptions{
			TargetDir: tempDir,
			Cfg:       cfg,
			Repo:      repo,
			LLMClient: &mockGateLLM{response: &domain.LLMResponse{}},
		})
		assert.NoError(t, err)
	})

	t.Run("when acceptance audit passes with green status, it returns nil", func(t *testing.T) {
		tempDir := t.TempDir()
		require.NoError(t, os.WriteFile(filepath.Join(tempDir, "SPEC.md"), []byte("# Spec\nFeature A"), 0644))
		repo := &mockStateRepo{state: &domain.State{ProjectPath: tempDir}}
		cfg := config.DefaultConfig()

		llm := &mockGateLLM{
			response: &domain.LLMResponse{
				Actions: []domain.LLMAction{
					{
						Tool: "submit_acceptance_audit",
						Args: map[string]any{
							"passed":  true,
							"summary": "All specification requirements verified.",
							"gaps":    []any{},
						},
					},
				},
			},
		}

		err := RunWholeProjectAcceptanceGate(context.Background(), AcceptanceGateOptions{
			TargetDir: tempDir,
			Cfg:       cfg,
			Repo:      repo,
			LLMClient: llm,
		})
		assert.NoError(t, err)
	})

	t.Run("when acceptance audit fails with gaps and rescue disabled, it returns failure error", func(t *testing.T) {
		tempDir := t.TempDir()
		require.NoError(t, os.WriteFile(filepath.Join(tempDir, "SPEC.md"), []byte("# Spec\nFeature A"), 0644))
		repo := &mockStateRepo{state: &domain.State{ProjectPath: tempDir}}
		cfg := config.DefaultConfig()
		rescueDisabled := false
		cfg.Fallback.SovereignRescue.Enabled = &rescueDisabled

		llm := &mockGateLLM{
			response: &domain.LLMResponse{
				Actions: []domain.LLMAction{
					{
						Tool: "submit_acceptance_audit",
						Args: map[string]any{
							"passed":  false,
							"summary": "Tautological tests detected",
							"gaps":    []any{"No real Redis commands tested", "Tautological make e2e"},
						},
					},
				},
			},
		}

		err := RunWholeProjectAcceptanceGate(context.Background(), AcceptanceGateOptions{
			TargetDir: tempDir,
			Cfg:       cfg,
			Repo:      repo,
			LLMClient: llm,
		})
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "Whole-Project Acceptance Audit FAILED")
		assert.Contains(t, err.Error(), "Tautological make e2e")
	})
}
