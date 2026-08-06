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
		singlePassPrompt := fmt.Sprintf("Execute task: %s - %s\n\nImplement the feature AND write corresponding unit/integration tests immediately in a single pass. Ensure both code and tests are created together.", task.Title, task.Description)
		o.updateTaskProgress(ctx, taskID, 50)
		o.RunGeneratorAgent(ctx, *task, taskState, fileContexts, "", singlePassPrompt)

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
		fixSinglePassPrompt := fmt.Sprintf("Execute task: %s - %s\n\nFix both the implementation and the tests to resolve previous failures and ensure all tests pass. MANDATE: If an error or dependency issue persists, force a working solution (even if simplified) so the code compiles cleanly and tests pass. Leaving non-compiling code is unacceptable.", task.Title, task.Description)
		o.updateTaskProgress(ctx, taskID, 50)
		o.RunGeneratorAgent(ctx, *task, taskState, fileContexts, "", fixSinglePassPrompt)

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
