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
	worktreeDir, taskGit, cleanup, setupErr := o.setupTaskWorkspace(ctx, state, task, taskID, baseBranch, integrationBranch, branchName)
	if setupErr != nil {
		fmt.Fprintf(os.Stderr, "Orchestrator: Failed to setup task workspace for task %s: %v\n", taskID, setupErr)
		_ = o.markTaskFailed(ctx, taskID, fmt.Sprintf("Failed to setup workspace: %v", setupErr))
		return
	}
	defer cleanup()

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

	// Inject Black-Box Contract Scenarios if story contract is present
	if state.Metadata.InputPath != "" {
		storyPath := state.Metadata.InputPath
		if !filepath.IsAbs(storyPath) {
			storyPath = filepath.Join(state.ProjectPath, storyPath)
		}
		if storyContent, err := os.ReadFile(storyPath); err == nil {
			if contract, err := ParseStoryContract(relativeStoryPath(state.ProjectPath, storyPath), string(storyContent)); err == nil && contract.StoryID != "" {
				upsertStoryContract(state, contract)
			}
			if contractCtx := FormatContractPromptContext(state.Metadata.InputPath, string(storyContent)); contractCtx != "" {
				fileContexts = append(fileContexts, contractCtx)
			}
		}
	}

	arch := strings.ToLower(strings.TrimSpace(o.cfg.Architecture))
	qaBlocked := ""
	switch arch {
	case "single_pass", "single_pass_execution", "spe":
		o.executeTaskSinglePass(ctx, task, &taskState, taskGit, fileContexts, taskID)
	case "breadth_first", "breadth_first_generation", "bfg", "big":
		o.executeTaskBreadthFirst(ctx, task, &taskState, taskGit, fileContexts, taskID)
	default:
		qaBlocked = o.executeTaskCodeFirst(ctx, task, &taskState, taskGit, fileContexts, taskID)
	}

	fmt.Printf("Orchestrator: Task %s running test validation...\n", taskID)
	// Run test suite validation
	passed, logMsg, _ := o.evaluator.ValidateTask(ctx, &taskState, *task)

	// Issue 7: First-Class Generator Surgical Repair
	initCategory := CategorizeFailureLog(logMsg)
	if !passed && qaBlocked == "" && (initCategory == FailureCompile || initCategory == FailureTestLogic) {
		fmt.Printf("Orchestrator: Task %s attempting single-turn surgical repair for %s...\n", taskID, initCategory)
		o.executeSurgicalRepairTurn(ctx, task, &taskState, taskGit, logMsg)
		passed, logMsg, _ = o.evaluator.ValidateTask(ctx, &taskState, *task)
	}

	if passed && qaBlocked == "" {
		qaBlocked = o.runQAGate(ctx, &taskState, *task, taskGit, fileContexts)
	}
	if qaBlocked != "" {
		passed, logMsg = false, "QA blocked task: "+qaBlocked
	}

	// Last-Resort Agent Escalation: Trigger if task failed and (retries exhausted, sandbox failure, or QA deadlock)
	category := CategorizeFailureLog(logMsg)
	isSandboxFailure := !passed && category == FailureSandbox
	shouldRetry := !passed && !isSandboxFailure && task.Retries < task.MaxRetries && task.MaxRetries > 0

	if !passed && o.cfg.LastResort.Enabled && (!shouldRetry || isSandboxFailure || qaBlocked != "") {
		triggerReason := "retries_exhausted"
		if isSandboxFailure {
			triggerReason = "missing_toolchain_or_sandbox_error"
		} else if qaBlocked != "" {
			triggerReason = "qa_gate_deadlock"
		}
		lraPassed, lraLog := o.RunLastResortAgent(ctx, task, &taskState, taskGit, logMsg, triggerReason)
		if lraPassed {
			passed = true
			logMsg = lraLog
		}
	}

	// Clean up worktree before merging/finalizing branches in the main repo
	if o.cfg.UseWorktrees {
		cleanup()
	}

	if passed {
		// Attempt merge-back into integrationBranch
		if pushErr := o.rebaseQueue.Push(ctx, branchName, integrationBranch); pushErr != nil {
			fmt.Fprintf(os.Stderr, "Orchestrator: merge-back of %s into %s failed for task %s: %v\n", branchName, integrationBranch, taskID, pushErr)
			passed = false
			logMsg = fmt.Sprintf("Failed to merge task branch into integration branch: %v", pushErr)
			// Worker branch is preserved so commits are never lost
		} else {
			if !o.cfg.UseWorktrees {
				_, _ = o.git.Run(ctx, true, "checkout", integrationBranch)
			}
			_, _ = o.git.Run(ctx, true, "branch", "-D", branchName)
		}
	}

	category = CategorizeFailureLog(logMsg)
	isSandboxFailure = !passed && category == FailureSandbox
	shouldRetry = !passed && !isSandboxFailure && task.Retries < task.MaxRetries && task.MaxRetries > 0

	if !passed && !shouldRetry && !isSandboxFailure {
		// Retries exhausted: perform optimistic merge to preserve generated code
		if pushErr := o.rebaseQueue.Push(ctx, branchName, integrationBranch); pushErr != nil {
			fmt.Fprintf(os.Stderr, "Orchestrator: optimistic merge of %s into %s failed for task %s: %v\n", branchName, integrationBranch, taskID, pushErr)
		} else {
			if !o.cfg.UseWorktrees {
				_, _ = o.git.Run(ctx, true, "checkout", integrationBranch)
			}
			_, _ = o.git.Run(ctx, true, "branch", "-D", branchName)
		}
	}

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
			fmt.Printf("✅ [Validation Passed] Task %s (%s) passed test validation and merged into %s\n", taskID, task.Title, integrationBranch)
		} else if isSandboxFailure {
			fmt.Printf("❌ [Unrecoverable Environment Failure] Task %s fast aborting: %s\n", taskID, logMsg)
			targetTask.Status = domain.TaskFailed
			targetTask.FailureLog = fmt.Sprintf("Unrecoverable environment error (%s): %s", category.String(), logMsg)
			targetTask.Progress = 0
			st.BuildStatus = domain.BuildFailing
		} else if shouldRetry {
			targetTask.Retries++
			targetTask.FailureLog = logMsg
			targetTask.Progress = 0
			fmt.Printf("⚠️  [Task Retry] Task %s (%s) validation or merge failed (attempt %d/%d). Re-queueing...\n", taskID, task.Title, targetTask.Retries, targetTask.MaxRetries)
			targetTask.Status = domain.TaskPending
			st.BuildStatus = domain.BuildFailing
		} else {
			// Optimistic completion on retry limit
			targetTask.Status = domain.TaskSuccess
			targetTask.Progress = 100
			targetTask.FailureLog = logMsg
			fmt.Printf("⚠️  [Optimistic Merge] Task %s (%s) merged into %s with warnings (post-merge repair/human review needed)\n", taskID, task.Title, integrationBranch)
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

	if shouldRetry || isSandboxFailure {
		o.metrics().RecordRetry()
		if isSandboxFailure && !o.cfg.UseWorktrees {
			_, _ = o.git.Run(ctx, true, "checkout", integrationBranch)
			_, _ = o.git.Run(ctx, true, "branch", "-D", branchName)
		} else if !o.cfg.UseWorktrees {
			_, _ = o.git.Run(ctx, true, "checkout", integrationBranch)
		}
	} else {
		o.metrics().RecordCommit()
		if !passed {
			o.metrics().RecordRetry()
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
