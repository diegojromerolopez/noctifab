package services

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/diegojromerolopez/noctifab/pkg/domain"
)

// executeTaskSinglePass runs a single-pass execution where implementation and unit/integration
// tests are co-generated together in a single generator turn.
func (o *Orchestrator) executeTaskSinglePass(ctx context.Context, task *domain.Task, taskState *domain.State, taskGit *GitClient, fileContexts []string, taskID string) {
	if task.Retries == 0 {
		o.updateTaskProgress(ctx, taskID, 50)
		o.RunGeneratorAgent(ctx, *task, taskState, fileContexts, "", "single_pass")

		statusOut, _ := taskGit.Run(ctx, false, "status", "--porcelain")
		if strings.TrimSpace(statusOut) != "" {
			_, _ = taskGit.Run(ctx, true, "add", "--all", "--", ":!.noctifab")
			stagedOut, _ := taskGit.Run(ctx, false, "diff", "--cached", "--name-only")
			if strings.TrimSpace(stagedOut) != "" {
				commitMsg := fmt.Sprintf("feat(core): implement functionality and tests for task %s - %s", taskID, task.Title)
				_, commitErr := taskGit.Run(ctx, true, "commit", "-m", commitMsg)
				if commitErr != nil {
					fmt.Fprintf(os.Stderr, "Orchestrator: Git commit failed for task %s single pass: %v\n", taskID, commitErr)
				}
			}
		}
	} else {
		o.updateTaskProgress(ctx, taskID, 50)
		o.RunGeneratorAgent(ctx, *task, taskState, fileContexts, "", "single_pass_fix")

		statusOut, _ := taskGit.Run(ctx, false, "status", "--porcelain")
		if strings.TrimSpace(statusOut) != "" {
			_, _ = taskGit.Run(ctx, true, "add", "--all", "--", ":!.noctifab")
			stagedOut, _ := taskGit.Run(ctx, false, "diff", "--cached", "--name-only")
			if strings.TrimSpace(stagedOut) != "" {
				commitMsg := fmt.Sprintf("feat(core): fix implementation and tests for task %s - %s", taskID, task.Title)
				_, commitErr := taskGit.Run(ctx, true, "commit", "-m", commitMsg)
				if commitErr != nil {
					fmt.Fprintf(os.Stderr, "Orchestrator: Git commit failed for task %s single pass fix: %v\n", taskID, commitErr)
				}
			}
		}
	}
}
