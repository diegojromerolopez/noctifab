package services

import (
	"context"
	"testing"

	"github.com/diegojromerolopez/noctifab/pkg/domain"
	"github.com/diegojromerolopez/noctifab/pkg/infrastructure/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestProgressiveTaskEscalator_Disabled(t *testing.T) {
	escalator := NewProgressiveTaskEscalator(config.TaskEscalationConfig{Enabled: false}, nil)
	task := &domain.Task{ID: "task-1", Complexity: "MEDIUM", Retries: 5}

	dec := escalator.EvaluateEscalation(context.Background(), task, 0)
	assert.False(t, dec.EnableThinking)
	assert.False(t, dec.UpgradeModel)
	assert.False(t, dec.ShouldSplit)
}

func TestProgressiveTaskEscalator_StandardLadder(t *testing.T) {
	cfg := config.TaskEscalationConfig{
		Enabled:                    true,
		ThinkingThresholdTurns:     2,
		ModelUpgradeThresholdTurns: 2,
		MaxModelUpgrades:           2,
		UpgradedModels:             []string{"claude-3-7-sonnet", "o3-mini"},
	}
	escalator := NewProgressiveTaskEscalator(cfg, nil)

	// Step 1: Turn 0, 0 retries -> Thinking disabled
	task := &domain.Task{ID: "task-1", Complexity: "MEDIUM", Retries: 0}
	dec1 := escalator.EvaluateEscalation(context.Background(), task, 0)
	assert.False(t, dec1.EnableThinking)
	assert.False(t, dec1.UpgradeModel)

	// Step 2: Turn 2 (or Retry 2) -> Thinking enabled
	task.Retries = 2
	dec2 := escalator.EvaluateEscalation(context.Background(), task, 0)
	assert.True(t, dec2.EnableThinking)
	assert.False(t, dec2.UpgradeModel)

	// Step 3: Retries exceed 4 (2 thinking + 2 upgrade) -> Upgrade to claude-3-7-sonnet (standard model)
	// First upgrade attempt starts with thinking disabled
	task.Retries = 4
	dec3 := escalator.EvaluateEscalation(context.Background(), task, 0)
	assert.True(t, dec3.UpgradeModel)
	assert.Equal(t, "claude-3-7-sonnet", dec3.TargetModel)
	assert.False(t, dec3.EnableThinking, "standard upgraded model starts with thinking disabled")
	assert.True(t, dec3.CleanWorktree)

	// Failing again on upgraded model (retry 5) -> Thinking enabled on upgraded model
	task.Retries = 5
	dec4 := escalator.EvaluateEscalation(context.Background(), task, 0)
	assert.True(t, dec4.UpgradeModel)
	assert.Equal(t, "claude-3-7-sonnet", dec4.TargetModel)
	assert.True(t, dec4.EnableThinking, "retrying on upgraded model enables thinking")

	// Upgrading to thinking-only model (e.g. o3-mini) -> skips thinking disabled and enables immediately
	task.ModelUpgrades = 1
	decThinkingOnly := escalator.EvaluateEscalation(context.Background(), task, 0)
	assert.True(t, decThinkingOnly.UpgradeModel)
	assert.Equal(t, "o3-mini", decThinkingOnly.TargetModel)
	assert.True(t, decThinkingOnly.EnableThinking, "thinking-only upgraded model skips disabled step")

	// Step 5: Exhausted max model upgrades (2) -> Last-resort task splitting
	task.ModelUpgrades = 2
	decSplit := escalator.EvaluateEscalation(context.Background(), task, 0)
	assert.True(t, decSplit.ShouldSplit)
	assert.True(t, decSplit.CleanWorktree)
}

func TestProgressiveTaskEscalator_HighComplexityFasterThinking(t *testing.T) {
	cfg := config.TaskEscalationConfig{
		Enabled:                true,
		ThinkingThresholdTurns: 2,
	}
	escalator := NewProgressiveTaskEscalator(cfg, nil)

	// High complexity task activates thinking after 1 retry
	task := &domain.Task{ID: "task-high", Complexity: "HIGH", Retries: 1}
	dec := escalator.EvaluateEscalation(context.Background(), task, 0)
	assert.True(t, dec.EnableThinking, "high complexity task should enable thinking on retry 1")
}

func TestProgressiveTaskEscalator_SplitTask(t *testing.T) {
	cfg := config.TaskEscalationConfig{Enabled: true}
	escalator := NewProgressiveTaskEscalator(cfg, nil)

	parent := &domain.Task{
		ID:          "US-001-TASK-001",
		Title:       "Implement Distributed Protocol Engine",
		Description: "Build distributed consensus, RPC endpoints, and state replication",
		ChangeType:  domain.ChangeTypeFeature,
		TargetFiles: []string{"pkg/engine.go", "pkg/rpc.go", "pkg/replication.go", "tests/engine_test.go"},
		DependsOn:   []string{"US-001-TASK-000"},
		MaxRetries:  3,
	}

	state := &domain.State{
		Tasks: []domain.Task{*parent},
	}

	subtasks, err := escalator.SplitTask(context.Background(), parent, state)
	require.NoError(t, err)
	require.Len(t, subtasks, 3)

	assert.Equal(t, "US-001-TASK-001-A", subtasks[0].ID)
	assert.Equal(t, "US-001-TASK-001-B", subtasks[1].ID)
	assert.Equal(t, "US-001-TASK-001-C", subtasks[2].ID)

	assert.Equal(t, []string{"US-001-TASK-000"}, subtasks[0].DependsOn)
	assert.Equal(t, []string{"US-001-TASK-001-A"}, subtasks[1].DependsOn)
	assert.Equal(t, []string{"US-001-TASK-001-B"}, subtasks[2].DependsOn)

	assert.Equal(t, "US-001-TASK-001", subtasks[0].ParentTaskID)
	assert.Equal(t, "MEDIUM", subtasks[0].Complexity)
	assert.NotEmpty(t, subtasks[0].TargetFiles)
}
