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
		genPrompt := fmt.Sprintf("Execute task in Breadth-First Generation mode: %s - %s\n\n"+
			"Focus on implementing the ~80%% core happy-path functionality to make the feature functional end-to-end. "+
			"Non-critical linter formatting nitpicks, style guidelines, and obscure corner cases will be refined in subsequent passes. "+
			"Ensure the code compiles cleanly without fatal runtime crashes.", task.Title, task.Description)
		o.updateTaskProgress(ctx, taskID, 25)
		o.RunGeneratorAgent(ctx, *task, taskState, fileContexts, "", genPrompt)

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
		testerPrompt := fmt.Sprintf("Write tests for task in Breadth-First Generation mode: %s - %s\n\n"+
			"Write baseline acceptance tests verifying the primary happy-path scenarios for the core functionality. "+
			"Focus on functional correctness of main workflows.", task.Title, task.Description)
		o.updateTaskProgress(ctx, taskID, 50)
		o.RunTesterAgent(ctx, *task, taskState, fileContexts, testerPrompt)

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
		fixPrompt := fmt.Sprintf("Refine implementation and tests for task: %s - %s\n\n"+
			"Address previous execution failures or missing edge cases. Ensure all existing happy-path tests continue to pass with zero regressions.", task.Title, task.Description)
		o.updateTaskProgress(ctx, taskID, 50)
		o.RunGeneratorAgent(ctx, *task, taskState, fileContexts, "", fixPrompt)

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
