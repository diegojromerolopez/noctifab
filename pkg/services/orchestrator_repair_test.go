package services

import (
	"context"
	"errors"
	"testing"

	"github.com/diegojromerolopez/noctifab/pkg/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPostMergeRepair_NilGuards(t *testing.T) {
	state := &domain.State{
		ID: "state-nil-guards",
		Metadata: domain.StateMetadata{
			BaseBranch:        "main",
			IntegrationBranch: "noctifab/feature-state-nil-guards",
		},
	}

	// Orchestrator with nil evaluator, git, llmClient should return nil cleanly without panic
	orch := &Orchestrator{}
	err := orch.RunPostMergeRepairPhase(context.Background(), state)
	assert.NoError(t, err)
}

func TestPostMergeRepair_BuildStatusTransitions(t *testing.T) {
	repoDir, _, cleanup := setupTestGitRepo(t)
	defer cleanup()

	state := &domain.State{
		ID:          "state-repair-status",
		ProjectPath: repoDir,
		Metadata: domain.StateMetadata{
			BaseBranch:        "main",
			IntegrationBranch: "main",
		},
		BuildStatus: domain.BuildFailing,
	}

	repo := &mockRepo{state: state}
	llmClient := &testMockLLM{}
	git := NewGitClient(repoDir)

	// Passing test validator
	evaluatorPass := NewTestValidator(&mockSandbox{Out: "PASS", Err: nil}, false, llmClient, nil)
	orchPass := &Orchestrator{
		repo:      repo,
		evaluator: evaluatorPass,
		git:       git,
		llmClient: llmClient,
	}

	err := orchPass.RunPostMergeRepairPhase(context.Background(), state)
	require.NoError(t, err)

	updatedState, err := repo.Load(context.Background())
	require.NoError(t, err)
	assert.Equal(t, domain.BuildPassing, updatedState.BuildStatus)

	// Failing test validator with no LLM actions
	evaluatorFail := NewTestValidator(&mockSandbox{Out: "FAIL", Err: errors.New("exit 1")}, false, llmClient, nil)
	orchFail := &Orchestrator{
		repo:      repo,
		evaluator: evaluatorFail,
		git:       git,
		llmClient: llmClient,
	}

	err = orchFail.RunPostMergeRepairPhase(context.Background(), state)
	require.NoError(t, err)

	updatedStateFail, err := repo.Load(context.Background())
	require.NoError(t, err)
	assert.Equal(t, domain.BuildFailing, updatedStateFail.BuildStatus)
}
