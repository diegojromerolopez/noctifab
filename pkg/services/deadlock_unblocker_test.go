package services

import (
	"context"
	"testing"

	"github.com/diegojromerolopez/noctifab/pkg/domain"
	"github.com/stretchr/testify/assert"
)

func TestOrchestrator_UnblockIndependentTasks(t *testing.T) {
	ctx := context.Background()

	state := &domain.State{
		Tasks: []domain.Task{
			{
				ID:          "US-001-TASK-001",
				Title:       "Failed Upstream Task",
				Status:      domain.TaskFailed,
				TargetFiles: []string{"src/upstream.py"},
			},
			{
				ID:          "US-002-TASK-001",
				Title:       "Independent Downstream Task",
				Status:      domain.TaskPending,
				DependsOn:   []string{"US-001-TASK-001"},
				TargetFiles: []string{"src/independent.py"},
			},
			{
				ID:          "US-002-TASK-002",
				Title:       "Colliding Downstream Task",
				Status:      domain.TaskPending,
				DependsOn:   []string{"US-001-TASK-001"},
				TargetFiles: []string{"src/upstream.py"},
			},
		},
	}
	repo := &mockRescueRepo{state: state}
	orch := &Orchestrator{repo: repo}

	unblocked := orch.UnblockIndependentTasks(ctx, state)

	assert.Equal(t, 1, unblocked, "expected 1 independent task to be unblocked")
	assert.Empty(t, state.Tasks[1].DependsOn, "independent task should have failed dep pruned")
	assert.Contains(t, state.Tasks[2].DependsOn, "US-001-TASK-001", "colliding task should retain failed dep")
}
