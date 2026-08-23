package services

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/diegojromerolopez/noctifab/pkg/domain"
)

// executeTaskCodeFirst orchestrates the standard iterative tester/generator turns for a task.
func (o *Orchestrator) executeTaskCodeFirst(
	ctx context.Context,
	task *domain.Task,
	taskState *domain.State,
	taskGit *GitClient,
	fileContexts []string,
	taskID string,
) string {
	qaBlocked := ""
	if task.Retries == 0 {
		execOrder := strings.ToLower(strings.TrimSpace(o.cfg.TaskExecutionOrder))
		if execOrder == "tester_first" {
			qaBlocked = o.executeTesterFirstTurn(ctx, task, taskState, taskGit, fileContexts, taskID)
		} else {
			qaBlocked = o.executeGeneratorFirstTurn(ctx, task, taskState, taskGit, fileContexts, taskID)
		}
	} else {
		qaBlocked = o.executeRetryTurn(ctx, task, taskState, taskGit, fileContexts, taskID)
	}
	return qaBlocked
}

func (o *Orchestrator) executeTesterFirstTurn(
	ctx context.Context,
	task *domain.Task,
	taskState *domain.State,
	taskGit *GitClient,
	fileContexts []string,
	taskID string,
) string {
	// Ensure minimal compilation stub files exist before running tester on Turn 1
	o.ensureTargetStubFilesExist(taskState.ProjectPath, task)

	// 1. Run Test Writer Agent (role "tester") to write tests first
	o.updateTaskProgress(ctx, taskID, 25)
	o.RunTesterAgent(ctx, *task, taskState, fileContexts, "write", "")
	_ = o.stageAndCommit(ctx, taskGit, taskID, "test(core): write tests for task %s - %s", task.Title)

	// 2. Run Generator Agent (role "generator") to implement feature functionality to pass tests
	o.updateTaskProgress(ctx, taskID, 50)
	o.RunGeneratorAgent(ctx, *task, taskState, fileContexts, "", "implement")
	_ = o.stageAndCommit(ctx, taskGit, taskID, "feat(core): implement minimal functionality for task %s - %s", task.Title)

	// Read recently written tests from git to pass to the Generator Agent for the Refactor phase
	recentTestsContext := o.collectRecentTestsContext(ctx, taskGit, taskState.ProjectPath)

	// 3. Run Generator Agent (role "generator") to refactor/improve the implementation to pass the tests
	o.updateTaskProgress(ctx, taskID, 75)
	generatorHeadBefore, generatorHeadErr := taskGit.Run(ctx, false, "rev-parse", "HEAD")
	o.RunGeneratorAgent(ctx, *task, taskState, fileContexts, recentTestsContext, "refactor")

	qaBlocked := ""
	if commitErr := o.stageAndCommit(ctx, taskGit, taskID, "feat(core): refactor implementation for task %s - %s", task.Title); commitErr != nil {
		qaBlocked = "generator_commit_failed"
	}
	generatorHeadAfter, generatorAfterErr := taskGit.Run(ctx, false, "rev-parse", "HEAD")
	if o.cfg.QA.Enabled && (generatorHeadErr != nil || generatorAfterErr != nil ||
		strings.TrimSpace(generatorHeadBefore) == strings.TrimSpace(generatorHeadAfter)) {
		qaBlocked = "generator_commit_failed"
	}
	return qaBlocked
}

func (o *Orchestrator) executeGeneratorFirstTurn(
	ctx context.Context,
	task *domain.Task,
	taskState *domain.State,
	taskGit *GitClient,
	fileContexts []string,
	taskID string,
) string {
	// 1. Run Generator Agent (role "generator") to implement minimal functionality
	o.updateTaskProgress(ctx, taskID, 25)
	o.RunGeneratorAgent(ctx, *task, taskState, fileContexts, "", "implement")
	_ = o.stageAndCommit(ctx, taskGit, taskID, "feat(core): implement minimal functionality for task %s - %s", task.Title)

	// 2. Run Test Writer Agent (role "tester") to write tests against the minimal implementation
	o.updateTaskProgress(ctx, taskID, 50)
	o.RunTesterAgent(ctx, *task, taskState, fileContexts, "write", "")
	_ = o.stageAndCommit(ctx, taskGit, taskID, "test(core): write tests for task %s - %s", task.Title)

	// Read recently written tests from git to pass to the Generator Agent for the Refactor phase
	recentTestsContext := o.collectRecentTestsContext(ctx, taskGit, taskState.ProjectPath)

	// 3. Run Generator Agent (role "generator") to refactor/improve the implementation to pass the tests
	o.updateTaskProgress(ctx, taskID, 75)
	generatorHeadBefore, generatorHeadErr := taskGit.Run(ctx, false, "rev-parse", "HEAD")
	o.RunGeneratorAgent(ctx, *task, taskState, fileContexts, recentTestsContext, "refactor")

	qaBlocked := ""
	if commitErr := o.stageAndCommit(ctx, taskGit, taskID, "feat(core): refactor implementation for task %s - %s", task.Title); commitErr != nil {
		qaBlocked = "generator_commit_failed"
	}
	generatorHeadAfter, generatorAfterErr := taskGit.Run(ctx, false, "rev-parse", "HEAD")
	if o.cfg.QA.Enabled && (generatorHeadErr != nil || generatorAfterErr != nil ||
		strings.TrimSpace(generatorHeadBefore) == strings.TrimSpace(generatorHeadAfter)) {
		qaBlocked = "generator_commit_failed"
	}
	return qaBlocked
}

func (o *Orchestrator) executeRetryTurn(
	ctx context.Context,
	task *domain.Task,
	taskState *domain.State,
	taskGit *GitClient,
	fileContexts []string,
	taskID string,
) string {
	// 1. Run Test Writer Agent to fix/refactor tests to align with updated implementation/failures
	o.updateTaskProgress(ctx, taskID, 40)
	o.RunTesterAgent(ctx, *task, taskState, fileContexts, "refactor", "")
	_ = o.stageAndCommit(ctx, taskGit, taskID, "test(core): fix/refactor tests for task %s - %s", task.Title)

	// Read recently written/fixed tests from git to pass to the Generator Agent
	recentTestsContext := o.collectRecentTestsContext(ctx, taskGit, taskState.ProjectPath)

	// 2. Run Generator Agent to fix/refactor implementation to pass the tests
	o.updateTaskProgress(ctx, taskID, 70)
	generatorHeadBefore, generatorHeadErr := taskGit.Run(ctx, false, "rev-parse", "HEAD")
	o.RunGeneratorAgent(ctx, *task, taskState, fileContexts, recentTestsContext, "fix")

	qaBlocked := ""
	if commitErr := o.stageAndCommit(ctx, taskGit, taskID, "feat(core): fix/refactor implementation for task %s - %s", task.Title); commitErr != nil {
		qaBlocked = "generator_commit_failed"
	}
	generatorHeadAfter, generatorAfterErr := taskGit.Run(ctx, false, "rev-parse", "HEAD")
	if o.cfg.QA.Enabled && (generatorHeadErr != nil || generatorAfterErr != nil ||
		strings.TrimSpace(generatorHeadBefore) == strings.TrimSpace(generatorHeadAfter)) {
		qaBlocked = "generator_commit_failed"
	}
	return qaBlocked
}

func (o *Orchestrator) executeSurgicalRepairTurn(
	ctx context.Context,
	task *domain.Task,
	taskState *domain.State,
	taskGit *GitClient,
	failureLog string,
) {
	o.updateTaskProgress(ctx, task.ID, 85)
	summary := summarizeFailureLog(failureLog)
	errorContext := fmt.Sprintf("### 🎯 TARGET FAILURE LOG TRACE FOR SURGICAL REPAIR:\n```\n%s\n```\nFix this specific error using minimal edits in 'edit_file'. Do not rewrite working code.", summary)
	o.RunGeneratorAgent(ctx, *task, taskState, nil, errorContext, "surgical_repair")
	_ = o.stageAndCommit(ctx, taskGit, task.ID, "fix(core): surgical repair for task %s - %s", task.Title)
}

func (o *Orchestrator) stageAndCommit(ctx context.Context, taskGit *GitClient, taskID string, format string, args ...interface{}) error {
	statusOut, _ := taskGit.Run(ctx, false, "status", "--porcelain")
	if strings.TrimSpace(statusOut) == "" {
		return nil
	}

	// Issue 6: Zero-token auto-formatting before git commit
	if o.evaluator != nil && o.evaluator.FormatterCommand != "" && o.evaluator.Runner != nil && taskGit != nil && taskGit.Dir() != "" {
		_, _ = o.evaluator.Runner.RunCommand(ctx, taskGit.Dir(), o.evaluator.FormatterCommand, "")
	}

	_, _ = taskGit.Run(ctx, true, "add", "--all", "--", ":!.noctifab")
	stagedOut, _ := taskGit.Run(ctx, false, "diff", "--cached", "--name-only")
	if strings.TrimSpace(stagedOut) == "" {
		return nil
	}
	commitMsg := fmt.Sprintf(format, append([]interface{}{taskID}, args...)...)
	_, commitErr := taskGit.Run(ctx, true, "commit", "-m", commitMsg)
	if commitErr != nil {
		fmt.Fprintf(os.Stderr, "Orchestrator: Git commit failed for task %s: %v\n", taskID, commitErr)
		return commitErr
	}
	return nil
}

func (o *Orchestrator) collectRecentTestsContext(ctx context.Context, taskGit *GitClient, projectPath string) string {
	diffOut, diffErr := taskGit.Run(ctx, false, "show", "--name-only", "--format=", "HEAD")
	if diffErr != nil {
		return ""
	}
	var testFileContexts []string
	for _, file := range strings.Split(diffOut, "\n") {
		file = strings.TrimSpace(file)
		if file != "" && (strings.Contains(file, "tests/") || strings.Contains(file, "spec/")) {
			fullPath, err := resolveSandboxPath(projectPath, file)
			if err == nil {
				if content, err := os.ReadFile(fullPath); err == nil {
					testFileContexts = append(testFileContexts, fmt.Sprintf("Test File %s:\n```\n%s\n```", file, capText(string(content), fileContextCapChars)))
				}
			}
		}
	}
	if len(testFileContexts) > 0 {
		return "\n\nWritten/Fixed tests context:\n" + strings.Join(testFileContexts, "\n\n")
	}
	return ""
}
