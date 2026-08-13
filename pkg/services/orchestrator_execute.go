package services

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"time"

	"github.com/diegojromerolopez/noctifab/pkg/domain"
	"github.com/diegojromerolopez/noctifab/pkg/infrastructure/telemetry"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

func (o *Orchestrator) executeTask(ctx context.Context, stateID, taskID string) {
	atomic.AddInt64(&o.totalActions, 1)

	if o.observer != nil {
		ctx = domain.WithObserver(ctx, o.observer)
	}

	ctx, span := telemetry.Tracer().Start(ctx, "executeTask",
		trace.WithAttributes(attribute.String("task.id", taskID)))
	defer span.End()

	ctx, cancel := context.WithTimeout(ctx, 30*time.Minute)
	defer cancel()

	state, err := o.repo.Load(ctx)
	if err != nil {
		return
	}

	var task *domain.Task
	for i := range state.Tasks {
		if state.Tasks[i].ID == taskID {
			task = &state.Tasks[i]
			break
		}
	}
	if task == nil {
		return
	}

	task.TargetFiles = collectTargetFilesRecursively(*task, state.Tasks)

	fmt.Printf("Orchestrator: Task %s (%s) is starting...\n", taskID, task.Title)

	taskStart := time.Now()
	if o.observer != nil {
		o.observer.Observe(ctx, domain.ExecutionEvent{
			Kind:   domain.EventTaskAttemptStarted,
			TaskID: taskID,
			Name:   task.Title,
			At:     taskStart.UTC(),
		})
	}
	defer func() {
		durMS := time.Since(taskStart).Milliseconds()
		if o.observer != nil {
			outcome := domain.OutcomeSuccess
			if err != nil {
				outcome = domain.OutcomeFailed
			}
			o.observer.Observe(ctx, domain.ExecutionEvent{
				Kind:           domain.EventTaskAttemptFinished,
				TaskID:         taskID,
				Name:           task.Title,
				At:             time.Now().UTC(),
				DurationMillis: &durMS,
				Outcome:        outcome,
			})
		}
	}()

	baseBranch := state.Metadata.BaseBranch
	if baseBranch == "" {
		baseBranch = "main"
	}
	integrationBranch := state.Metadata.IntegrationBranch
	if integrationBranch == "" {
		integrationBranch = fmt.Sprintf("noctifab/feature-%s", state.ID[:8])
	}

	err = o.updateStateWithRetry(ctx, func(st *domain.State) error {
		for i := range st.Tasks {
			if st.Tasks[i].ID == taskID {
				st.Tasks[i].Status = domain.TaskInProgress
				st.Tasks[i].Progress = 10
				st.Tasks[i].UpdatedAt = time.Now()
				return nil
			}
		}
		return fmt.Errorf("task %s not found in state", taskID)
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "Orchestrator: Failed to update task status to IN_PROGRESS for task %s: %v\n", taskID, err)
		return
	}

	branchName := fmt.Sprintf("noctifab/task-%s-worker", taskID)
	var statusOut, stagedOut string

	if !o.cfg.UseWorktrees {
		_, _ = o.git.Run(ctx, true, "reset", "--hard")
		_, _ = o.git.Run(ctx, true, "clean", "-fd")
	}

	isFreshStart := true
	for _, t := range state.Tasks {
		if t.Status == domain.TaskSuccess {
			isFreshStart = false
			break
		}
	}

	if isFreshStart {
		_, err = o.git.Run(ctx, false, "show-ref", "--verify", "--quiet", "refs/heads/"+integrationBranch)
		if err == nil {
			_, _ = o.git.Run(ctx, true, "checkout", baseBranch)
			_, _ = o.git.Run(ctx, true, "branch", "-f", integrationBranch, baseBranch)
		}
	}

	_, err = o.git.Run(ctx, false, "show-ref", "--verify", "--quiet", "refs/heads/"+integrationBranch)
	if err != nil {
		if _, err := o.git.Run(ctx, true, "checkout", baseBranch); err != nil {
			fmt.Fprintf(os.Stderr, "Orchestrator: Failed to checkout base branch %s: %v\n", baseBranch, err)
			_ = o.markTaskFailed(ctx, taskID, fmt.Sprintf("Failed to checkout base branch: %v", err))
			return
		}
		if _, err := o.git.Run(ctx, true, "checkout", "-b", integrationBranch); err != nil {
			_, errVerify := o.git.Run(ctx, false, "show-ref", "--verify", "--quiet", "refs/heads/"+integrationBranch)
			if errVerify != nil {
				fmt.Fprintf(os.Stderr, "Orchestrator: Failed to create integration branch %s: %v\n", integrationBranch, err)
				_ = o.markTaskFailed(ctx, taskID, fmt.Sprintf("Failed to create integration branch: %v", err))
				return
			}
		}
	}

	worktreeDir := state.ProjectPath
	var taskGit *GitClient
	if o.cfg.UseWorktrees {
		worktreeDir = filepath.Join(state.ProjectPath, ".noctifab", "worktrees", fmt.Sprintf("task-%s", taskID))
		_, _ = o.git.Run(ctx, true, "worktree", "remove", "--force", worktreeDir)
		_ = os.RemoveAll(worktreeDir)
		_ = os.MkdirAll(filepath.Dir(worktreeDir), 0755)

		_, err = o.git.Run(ctx, true, "worktree", "add", "-b", branchName, worktreeDir, integrationBranch)
		if err != nil {
			_, _ = o.git.Run(ctx, true, "worktree", "add", worktreeDir, branchName)
		}
		taskGit = NewGitClient(worktreeDir)
		syncRootManifests(state.ProjectPath, worktreeDir)

		defer func() {
			_, _ = o.git.Run(ctx, true, "worktree", "remove", "--force", worktreeDir)
			_ = os.RemoveAll(worktreeDir)
			_, _ = o.git.Run(ctx, true, "worktree", "prune")
		}()
	} else {
		taskGit = o.git
		if _, err := o.git.Run(ctx, true, "checkout", integrationBranch); err != nil {
			fmt.Fprintf(os.Stderr, "Orchestrator: Failed to checkout integration branch %s: %v\n", integrationBranch, err)
			_ = o.markTaskFailed(ctx, taskID, fmt.Sprintf("Failed to checkout integration branch: %v", err))
			return
		}

		branchExists := false
		_, err = o.git.Run(ctx, false, "show-ref", "--verify", "--quiet", "refs/heads/"+branchName)
		if err == nil {
			branchExists = true
		}

		if branchExists && task.Retries > 0 {
			// Checkout existing worker branch
			if _, err := o.git.Run(ctx, true, "checkout", branchName); err != nil {
				fmt.Fprintf(os.Stderr, "Orchestrator: Failed to checkout existing worker branch %s: %v\n", branchName, err)
				return
			}
			// Preserving previous commits on retry to enable incremental state retention
		} else {
			// Force delete worker branch if left over from a previous crashed run or not a retry
			_, _ = o.git.Run(ctx, true, "branch", "-D", branchName)

			// Create worker branch from integrationBranch
			_, err = o.git.Run(ctx, true, "checkout", "-b", branchName)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Orchestrator: Failed to create worker branch %s: %v\n", branchName, err)
				_ = o.markTaskFailed(ctx, taskID, fmt.Sprintf("Failed to create worker branch: %v", err))
				return
			}
		}
	}

	taskState := *state.Clone()
	if o.cfg.UseWorktrees {
		taskState.ProjectPath = worktreeDir
	}

	var fileContexts []string
	for _, file := range task.TargetFiles {
		fullPath, err := resolveSandboxPath(taskState.ProjectPath, file)
		if err == nil {
			if content, err := os.ReadFile(fullPath); err == nil {
				fileContexts = append(fileContexts, fmt.Sprintf("File %s:\n```\n%s\n```", file, capText(string(content), fileContextCapChars)))
			}
		}
	}

	arch := strings.ToLower(strings.TrimSpace(o.cfg.Architecture))
	qaBlocked := ""
	if arch == "single_pass" || arch == "single_pass_execution" || arch == "spe" {
		o.executeTaskSinglePass(ctx, task, &taskState, taskGit, fileContexts, taskID)
	} else if arch == "breadth_first" || arch == "breadth_first_generation" || arch == "bfg" || arch == "big" {
		o.executeTaskBreadthFirst(ctx, task, &taskState, taskGit, fileContexts, taskID)
	} else if task.Retries == 0 {
		// 1. Run Generator Agent (role "generator") to implement minimal functionality
		o.updateTaskProgress(ctx, taskID, 25)
		o.RunGeneratorAgent(ctx, *task, &taskState, fileContexts, "", "implement")

		// Stage and commit minimal implementation
		statusOut, _ = taskGit.Run(ctx, false, "status", "--porcelain")
		if strings.TrimSpace(statusOut) != "" {
			_, _ = taskGit.Run(ctx, true, "add", "--all", "--", ":!.noctifab")
			stagedOut, _ = taskGit.Run(ctx, false, "diff", "--cached", "--name-only")
			if strings.TrimSpace(stagedOut) != "" {
				commitMsg := fmt.Sprintf("feat(core): implement minimal functionality for task %s - %s", taskID, task.Title)
				_, commitErr := taskGit.Run(ctx, true, "commit", "-m", commitMsg)
				if commitErr != nil {
					fmt.Fprintf(os.Stderr, "Orchestrator: Git commit failed for task %s minimal implementation: %v\n", taskID, commitErr)
				}
			}
		}

		// 2. Run Test Writer Agent (role "tester") to write tests against the minimal implementation
		o.updateTaskProgress(ctx, taskID, 50)
		o.RunTesterAgent(ctx, *task, &taskState, fileContexts, "write", "")

		// Stage and commit tests
		statusOut, _ = taskGit.Run(ctx, false, "status", "--porcelain")
		if strings.TrimSpace(statusOut) != "" {
			_, _ = taskGit.Run(ctx, true, "add", "--all", "--", ":!.noctifab")
			stagedOut, _ = taskGit.Run(ctx, false, "diff", "--cached", "--name-only")
			if strings.TrimSpace(stagedOut) != "" {
				commitMsg := fmt.Sprintf("test(core): write tests for task %s - %s", taskID, task.Title)
				_, commitErr := taskGit.Run(ctx, true, "commit", "-m", commitMsg)
				if commitErr != nil {
					fmt.Fprintf(os.Stderr, "Orchestrator: Git commit failed for task %s tests: %v\n", taskID, commitErr)
				}
			}
		}
		// Read recently written tests from git to pass to the Generator Agent for the Refactor phase
		recentTestsContext := ""
		diffOut, diffErr := taskGit.Run(ctx, false, "show", "--name-only", "--format=", "HEAD")
		if diffErr == nil {
			var testFileContexts []string
			for _, file := range strings.Split(diffOut, "\n") {
				file = strings.TrimSpace(file)
				if file != "" && (strings.Contains(file, "tests/") || strings.Contains(file, "spec/")) {
					fullPath, err := resolveSandboxPath(taskState.ProjectPath, file)
					if err == nil {
						if content, err := os.ReadFile(fullPath); err == nil {
							testFileContexts = append(testFileContexts, fmt.Sprintf("Test File %s:\n```\n%s\n```", file, capText(string(content), fileContextCapChars)))
						}
					}
				}
			}
			if len(testFileContexts) > 0 {
				recentTestsContext = "\n\nWritten tests context:\n" + strings.Join(testFileContexts, "\n\n")
			}
		}

		// 3. Run Generator Agent (role "generator") to refactor/improve the implementation to pass the tests
		o.updateTaskProgress(ctx, taskID, 75)
		generatorHeadBefore, generatorHeadErr := taskGit.Run(ctx, false, "rev-parse", "HEAD")
		o.RunGeneratorAgent(ctx, *task, &taskState, fileContexts, recentTestsContext, "refactor")

		// Stage and commit refactoring changes
		statusOut, _ = taskGit.Run(ctx, false, "status", "--porcelain")
		if strings.TrimSpace(statusOut) != "" {
			_, _ = taskGit.Run(ctx, true, "add", "--all", "--", ":!.noctifab")
			stagedOut, _ = taskGit.Run(ctx, false, "diff", "--cached", "--name-only")
			if strings.TrimSpace(stagedOut) != "" {
				commitMsg := fmt.Sprintf("feat(core): refactor implementation for task %s - %s", taskID, task.Title)
				_, commitErr := taskGit.Run(ctx, true, "commit", "-m", commitMsg)
				if commitErr != nil {
					fmt.Fprintf(os.Stderr, "Orchestrator: Git commit failed for task %s refactor: %v\n", taskID, commitErr)
					qaBlocked = "generator_commit_failed"
				}
			}
		}
		generatorHeadAfter, generatorAfterErr := taskGit.Run(ctx, false, "rev-parse", "HEAD")
		if o.cfg.QA.Enabled && (generatorHeadErr != nil || generatorAfterErr != nil ||
			strings.TrimSpace(generatorHeadBefore) == strings.TrimSpace(generatorHeadAfter)) {
			qaBlocked = "generator_commit_failed"
		}
	} else {
		// Retries loop: Tester runs first to fix tests, then Generator runs to make them pass

		// 1. Run Test Writer Agent to fix/refactor tests to align with updated implementation/failures
		o.updateTaskProgress(ctx, taskID, 40)
		o.RunTesterAgent(ctx, *task, &taskState, fileContexts, "refactor", "")

		// Stage and commit test fixes
		statusOut, _ = taskGit.Run(ctx, false, "status", "--porcelain")
		if strings.TrimSpace(statusOut) != "" {
			_, _ = taskGit.Run(ctx, true, "add", "--all", "--", ":!.noctifab")
			stagedOut, _ = taskGit.Run(ctx, false, "diff", "--cached", "--name-only")
			if strings.TrimSpace(stagedOut) != "" {
				commitMsg := fmt.Sprintf("test(core): fix/refactor tests for task %s - %s", taskID, task.Title)
				_, commitErr := taskGit.Run(ctx, true, "commit", "-m", commitMsg)
				if commitErr != nil {
					fmt.Fprintf(os.Stderr, "Orchestrator: Git commit failed for task %s fix tests: %v\n", taskID, commitErr)
				}
			}
		}

		// Read recently written/fixed tests from git to pass to the Generator Agent
		recentTestsContext := ""
		diffOut, diffErr := taskGit.Run(ctx, false, "show", "--name-only", "--format=", "HEAD")
		if diffErr == nil {
			var testFileContexts []string
			for _, file := range strings.Split(diffOut, "\n") {
				file = strings.TrimSpace(file)
				if file != "" && (strings.Contains(file, "tests/") || strings.Contains(file, "spec/")) {
					fullPath, err := resolveSandboxPath(taskState.ProjectPath, file)
					if err == nil {
						if content, err := os.ReadFile(fullPath); err == nil {
							testFileContexts = append(testFileContexts, fmt.Sprintf("Test File %s:\n```\n%s\n```", file, capText(string(content), fileContextCapChars)))
						}
					}
				}
			}
			if len(testFileContexts) > 0 {
				recentTestsContext = "\n\nWritten/Fixed tests context:\n" + strings.Join(testFileContexts, "\n\n")
			}
		}

		// 2. Run Generator Agent to fix/refactor implementation to pass the tests
		o.updateTaskProgress(ctx, taskID, 70)
		generatorHeadBefore, generatorHeadErr := taskGit.Run(ctx, false, "rev-parse", "HEAD")
		o.RunGeneratorAgent(ctx, *task, &taskState, fileContexts, recentTestsContext, "fix")

		// Stage and commit fixes
		statusOut, _ = taskGit.Run(ctx, false, "status", "--porcelain")
		if strings.TrimSpace(statusOut) != "" {
			_, _ = taskGit.Run(ctx, true, "add", "--all", "--", ":!.noctifab")
			stagedOut, _ = taskGit.Run(ctx, false, "diff", "--cached", "--name-only")
			if strings.TrimSpace(stagedOut) != "" {
				commitMsg := fmt.Sprintf("feat(core): fix/refactor implementation for task %s - %s", taskID, task.Title)
				_, commitErr := taskGit.Run(ctx, true, "commit", "-m", commitMsg)
				if commitErr != nil {
					fmt.Fprintf(os.Stderr, "Orchestrator: Git commit failed for task %s fix implementation: %v\n", taskID, commitErr)
					qaBlocked = "generator_commit_failed"
				}
			}
		}
		generatorHeadAfter, generatorAfterErr := taskGit.Run(ctx, false, "rev-parse", "HEAD")
		if o.cfg.QA.Enabled && (generatorHeadErr != nil || generatorAfterErr != nil ||
			strings.TrimSpace(generatorHeadBefore) == strings.TrimSpace(generatorHeadAfter)) {
			qaBlocked = "generator_commit_failed"
		}
	}

	fmt.Printf("Orchestrator: Task %s running test validation...\n", taskID)
	// Run test suite validation
	passed, logMsg, _ := o.evaluator.ValidateTask(ctx, &taskState, *task)
	if passed && qaBlocked == "" {
		qaBlocked = o.runQAGate(ctx, &taskState, *task, taskGit, fileContexts)
	}
	if qaBlocked != "" {
		passed, logMsg = false, "QA blocked task: "+qaBlocked
	}

	var permanentlyFailed bool
	err = o.updateStateWithRetry(ctx, func(st *domain.State) error {
		var targetTask *domain.Task
		for i := range st.Tasks {
			if st.Tasks[i].ID == taskID {
				targetTask = &st.Tasks[i]
				break
			}
		}
		if targetTask == nil {
			return fmt.Errorf("task %s not found in state", taskID)
		}

		if passed {
			targetTask.Status = domain.TaskSuccess
			targetTask.Progress = 100
			targetTask.FailureLog = ""
			fmt.Printf("✅ [Validation Passed] Task %s (%s) passed test validation\n", taskID, task.Title)
		} else {
			category := CategorizeFailureLog(logMsg)
			if category == FailureSandbox {
				fmt.Printf("❌ [Unrecoverable Environment Failure] Task %s fast aborting: %s\n", taskID, logMsg)
				targetTask.Status = domain.TaskFailed
				targetTask.FailureLog = fmt.Sprintf("Unrecoverable environment error (%s): %s", category.String(), logMsg)
				targetTask.Progress = 0
				permanentlyFailed = true
			} else {
				targetTask.Retries++
				targetTask.FailureLog = logMsg
				targetTask.Progress = 0
				if targetTask.Retries >= targetTask.MaxRetries {
					fmt.Printf("❌ [Task Failed Permanently] Task %s (%s) reached max retries (%d/%d)\n", taskID, task.Title, targetTask.Retries, targetTask.MaxRetries)
					targetTask.Status = domain.TaskFailed
					permanentlyFailed = true
				} else {
					fmt.Printf("⚠️  [Task Retry] Task %s (%s) validation failed (attempt %d/%d). Re-queueing...\n", taskID, task.Title, targetTask.Retries, targetTask.MaxRetries)
					targetTask.Status = domain.TaskPending
				}
			}
			st.BuildStatus = domain.BuildFailing
		}
		targetTask.UpdatedAt = time.Now()

		domain.AppendAction(st, domain.Action{
			Timestamp: time.Now(),
			Tool:      "evaluate",
			Success:   passed,
			Result:    logMsg,
		})
		return nil
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "Orchestrator: Failed to save task execution outcome for task %s: %v\n", taskID, err)
	}

	// Clean up worktree before merging/finalizing branches in the main repo
	if o.cfg.UseWorktrees {
		_, _ = o.git.Run(ctx, true, "worktree", "remove", "--force", worktreeDir)
		_ = os.RemoveAll(worktreeDir)
		_, _ = o.git.Run(ctx, true, "worktree", "prune")
	}

	if passed {
		fmt.Printf("Orchestrator: Task %s completed successfully!\n", taskID)
		o.metrics().RecordCommit()
		// Merge back sequentially into integrationBranch using RebaseQueue.
		// A failed merge-back must not stay invisible: the task is already
		// marked SUCCESS, so surface the error loudly for the unblocker/logs.
		if pushErr := o.rebaseQueue.Push(ctx, branchName, integrationBranch); pushErr != nil {
			fmt.Fprintf(os.Stderr, "Orchestrator: merge-back of %s into %s failed for task %s: %v\n", branchName, integrationBranch, taskID, pushErr)
		}

		// Clean up branch
		if !o.cfg.UseWorktrees {
			_, _ = o.git.Run(ctx, true, "checkout", integrationBranch)
			_, _ = o.git.Run(ctx, true, "branch", "-D", branchName)
		} else {
			_, _ = o.git.Run(ctx, true, "branch", "-D", branchName)
		}
	} else {
		o.metrics().RecordRetry()
		fmt.Fprintf(os.Stderr, "Orchestrator: Task %s failed test validation. Retrying or marking FAILED. Failure log:\n%s\n", taskID, logMsg)
		if !o.cfg.UseWorktrees {
			if permanentlyFailed {
				_, _ = o.git.Run(ctx, true, "checkout", integrationBranch)
				_, _ = o.git.Run(ctx, true, "branch", "-D", branchName)
			} else {
				_, _ = o.git.Run(ctx, true, "checkout", integrationBranch)
			}
		} else {
			if permanentlyFailed {
				_, _ = o.git.Run(ctx, true, "branch", "-D", branchName)
			}
		}
	}

	metricsPath := filepath.Join(state.ProjectPath, ".noctifab", "data", "metrics.json")
	_ = o.metrics().ExportJSON(metricsPath)

	o.scheduler.ReleaseLocks(taskID)

	// Signal main loop to evaluate ready tasks immediately
	select {
	case o.taskCompletedChan <- struct{}{}:
	default:
	}
}
