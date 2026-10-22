package services

import (
	"context"

	"github.com/diegojromerolopez/noctifab/pkg/domain"
)

// executeTaskSinglePass runs a single-pass execution where implementation and unit/integration
// tests are co-generated together in a single generator turn (Unified Co-Synthesis Mode).
// Supports multi-hypothesis parallel worktree execution and resolution (correct vs mixture).
func (o *Orchestrator) executeTaskSinglePass(ctx context.Context, task *domain.Task, taskState *domain.State, taskGit *GitClient, fileContexts []string, taskID string) {
	action := "single_pass"
	if task.Retries > 0 {
		action = "single_pass_fix"
	}
	o.updateTaskProgress(ctx, taskID, 50)
	o.ExecuteSinglePassHypotheses(ctx, task, taskState, taskGit, fileContexts, taskID, action)
}
