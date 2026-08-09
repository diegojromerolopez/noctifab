package services

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/diegojromerolopez/noctifab/pkg/domain"
)

// executeTaskBreadthFirst executes a task using the Breadth-First Generation (BFG) architecture.
// In this mode, Generator and Tester agents focus on delivering ~80% happy-path functionality across tasks,
// deferring edge-case assertions and minor linter/formatting warnings to subsequent refinement iterations.
func (o *Orchestrator) executeTaskBreadthFirst(ctx context.Context, task *domain.Task, taskState *domain.State, taskGit *GitClient, fileContexts []string, taskID string) {
	if task.Retries == 0 {
		// 1. Generator Agent turn: 80% happy path implementation
		o.updateTaskProgress(ctx, taskID, 25)
		o.RunGeneratorAgent(ctx, *task, taskState, fileContexts, "", "implement_breadth_first")

		statusOut, _ := taskGit.Run(ctx, false, "status", "--porcelain")
		if strings.TrimSpace(statusOut) != "" {
			_, _ = taskGit.Run(ctx, true, "add", "--all", "--", ":!.noctifab")
			stagedOut, _ := taskGit.Run(ctx, false, "diff", "--cached", "--name-only")
			if strings.TrimSpace(stagedOut) != "" {
				commitMsg := fmt.Sprintf("feat(bfg): implement core happy-path functionality for task %s - %s", taskID, task.Title)
				_, commitErr := taskGit.Run(ctx, true, "commit", "-m", commitMsg)
				if commitErr != nil {
					fmt.Fprintf(os.Stderr, "Orchestrator: Git commit failed for task %s breadth-first implementation: %v\n", taskID, commitErr)
				}
			}
		}

		// 2. Tester Agent turn: Happy path acceptance tests
		o.updateTaskProgress(ctx, taskID, 50)
		o.RunTesterAgent(ctx, *task, taskState, fileContexts, "write_breadth_first", "")

		statusOut, _ = taskGit.Run(ctx, false, "status", "--porcelain")
		if strings.TrimSpace(statusOut) != "" {
			_, _ = taskGit.Run(ctx, true, "add", "--all", "--", ":!.noctifab")
			stagedOut, _ := taskGit.Run(ctx, false, "diff", "--cached", "--name-only")
			if strings.TrimSpace(stagedOut) != "" {
				commitMsg := fmt.Sprintf("test(bfg): write happy-path tests for task %s - %s", taskID, task.Title)
				_, commitErr := taskGit.Run(ctx, true, "commit", "-m", commitMsg)
				if commitErr != nil {
					fmt.Fprintf(os.Stderr, "Orchestrator: Git commit failed for task %s breadth-first tests: %v\n", taskID, commitErr)
				}
			}
		}
	} else {
		// Retries > 0: Benevolent refinement pass addressing defects while preserving zero regressions
		o.updateTaskProgress(ctx, taskID, 50)
		o.RunGeneratorAgent(ctx, *task, taskState, fileContexts, "", "implement_breadth_first_fix")

		statusOut, _ := taskGit.Run(ctx, false, "status", "--porcelain")
		if strings.TrimSpace(statusOut) != "" {
			_, _ = taskGit.Run(ctx, true, "add", "--all", "--", ":!.noctifab")
			stagedOut, _ := taskGit.Run(ctx, false, "diff", "--cached", "--name-only")
			if strings.TrimSpace(stagedOut) != "" {
				commitMsg := fmt.Sprintf("fix(bfg): refine implementation and tests for task %s - %s", taskID, task.Title)
				_, commitErr := taskGit.Run(ctx, true, "commit", "-m", commitMsg)
				if commitErr != nil {
					fmt.Fprintf(os.Stderr, "Orchestrator: Git commit failed for task %s breadth-first refinement: %v\n", taskID, commitErr)
				}
			}
		}
	}
}
