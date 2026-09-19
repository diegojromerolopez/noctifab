package services

import (
	"context"
	"fmt"

	"github.com/diegojromerolopez/noctifab/pkg/domain"
)

// executeTaskSinglePass runs a single-pass execution where implementation and unit/integration
// tests are co-generated together in a single generator turn (Unified Co-Synthesis Mode).
func (o *Orchestrator) executeTaskSinglePass(ctx context.Context, task *domain.Task, taskState *domain.State, taskGit *GitClient, fileContexts []string, taskID string) {
	if task.Retries == 0 {
		o.updateTaskProgress(ctx, taskID, 50)
		o.RunGeneratorAgent(ctx, *task, taskState, fileContexts, "", "single_pass")

		if violations := o.auditGeneratorFunctionalOutput(taskState.ProjectPath, task.TargetFiles); len(violations) > 0 {
			fmt.Printf("⚠️  [Single-Pass Gate] Task %s: Generator produced %d non-functional stub(s). Triggering remediation turn...\n", taskID, len(violations))
			remediationCtx := append(fileContexts, o.formatAntiStubViolations(violations))
			o.RunGeneratorAgent(ctx, *task, taskState, remediationCtx, "", "single_pass_fix")
		}

		_ = o.stageAndCommit(ctx, taskGit, taskID, "feat(core): implement functionality and tests for task %s - %s", task.Title)
	} else {
		o.updateTaskProgress(ctx, taskID, 50)
		o.RunGeneratorAgent(ctx, *task, taskState, fileContexts, "", "single_pass_fix")

		if violations := o.auditGeneratorFunctionalOutput(taskState.ProjectPath, task.TargetFiles); len(violations) > 0 {
			fmt.Printf("⚠️  [Single-Pass Gate] Task %s: Generator produced %d non-functional stub(s). Triggering remediation turn...\n", taskID, len(violations))
			remediationCtx := append(fileContexts, o.formatAntiStubViolations(violations))
			o.RunGeneratorAgent(ctx, *task, taskState, remediationCtx, "", "single_pass_fix")
		}

		_ = o.stageAndCommit(ctx, taskGit, taskID, "feat(core): fix implementation and tests for task %s - %s", task.Title)
	}
}
