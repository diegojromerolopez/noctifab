package cli

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/diegojromerolopez/noctifab/pkg/domain"
	"github.com/diegojromerolopez/noctifab/pkg/infrastructure/config"
	"github.com/diegojromerolopez/noctifab/pkg/services"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSovereignProjectRescue_SliceSpec(t *testing.T) {
	longSpec := "# Project Specification\n\n## Core Invariants\nMust pass tests.\n\n## Test Matrix\n" +
		strings.Repeat("- Case: granular assertion detail\n", 500)
	require.Greater(t, len(longSpec), 12000)

	t.Run("when slice_spec is enabled (default), oversized spec is sliced", func(t *testing.T) {
		tempDir := t.TempDir()
		specFile := filepath.Join(tempDir, "SPEC.md")
		require.NoError(t, os.WriteFile(specFile, []byte(longSpec), 0644))

		mockRepo := &mockDAGStateRepo{
			state: &domain.State{
				ProjectPath: tempDir,
				Stories: []domain.Story{
					{ID: "story-0001", Title: "US-001", Status: domain.StoryFailed},
				},
				Tasks: []domain.Task{
					{ID: "task-1", Status: domain.TaskFailed},
				},
			},
		}

		mockLLM := &mockRescueLLM{
			responses: []*domain.LLMResponse{
				{
					Actions: []domain.LLMAction{
						{
							Tool: "noop",
							Args: map[string]any{},
						},
					},
				},
			},
		}

		reg := services.NewToolRegistry()
		sandbox := &mockRescueSandbox{}
		validator := services.NewTestValidator(sandbox, false, mockLLM, reg.Tools())

		opts := SovereignRescueOptions{
			TargetDir:     tempDir,
			Cfg:           config.DefaultConfig(),
			Repo:          mockRepo,
			GitClient:     services.NewGitClient(tempDir),
			StoryFiles:    []string{"US-001.md"},
			FailedStories: []string{"US-001.md (compilation failed)"},
			LLMClient:     mockLLM,
			ToolRegistry:  reg,
			Validator:     validator,
			MaxTurns:      1,
		}

		_ = runSovereignProjectRescue(context.Background(), opts)

		require.NotEmpty(t, mockLLM.prompts)
		prompt := mockLLM.prompts[0]
		assert.Contains(t, prompt, "Core Invariants")
		assert.NotContains(t, prompt, "granular assertion detail")
	})

	t.Run("when slice_spec is explicitly disabled, full spec is included", func(t *testing.T) {
		tempDir := t.TempDir()
		specFile := filepath.Join(tempDir, "SPEC.md")
		require.NoError(t, os.WriteFile(specFile, []byte(longSpec), 0644))

		mockRepo := &mockDAGStateRepo{
			state: &domain.State{
				ProjectPath: tempDir,
				Stories: []domain.Story{
					{ID: "story-0001", Title: "US-001", Status: domain.StoryFailed},
				},
				Tasks: []domain.Task{
					{ID: "task-1", Status: domain.TaskFailed},
				},
			},
		}

		mockLLM := &mockRescueLLM{
			responses: []*domain.LLMResponse{
				{
					Actions: []domain.LLMAction{
						{
							Tool: "noop",
							Args: map[string]any{},
						},
					},
				},
			},
		}

		reg := services.NewToolRegistry()
		sandbox := &mockRescueSandbox{}
		validator := services.NewTestValidator(sandbox, false, mockLLM, reg.Tools())

		cfg := config.DefaultConfig()
		disabled := false
		cfg.Fallback.SovereignRescue.SliceSpec = &disabled

		opts := SovereignRescueOptions{
			TargetDir:     tempDir,
			Cfg:           cfg,
			Repo:          mockRepo,
			GitClient:     services.NewGitClient(tempDir),
			StoryFiles:    []string{"US-001.md"},
			FailedStories: []string{"US-001.md (compilation failed)"},
			LLMClient:     mockLLM,
			ToolRegistry:  reg,
			Validator:     validator,
			MaxTurns:      1,
		}

		_ = runSovereignProjectRescue(context.Background(), opts)

		require.NotEmpty(t, mockLLM.prompts)
		prompt := mockLLM.prompts[0]
		assert.Contains(t, prompt, "Core Invariants")
		assert.Contains(t, prompt, "granular assertion detail")
	})
}
