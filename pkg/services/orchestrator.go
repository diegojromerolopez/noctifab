package services

import (
	"context"
	"errors"
	"fmt"
	"math"
	"os"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/diegojromerolopez/noctifab/pkg/domain"
	"github.com/diegojromerolopez/noctifab/pkg/infrastructure/telemetry"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

// RepairHandler defines the contract for automatic repair of hung/failed test suites.
type RepairHandler interface {
	AttemptRepair(ctx context.Context, state *domain.State, task domain.Task, watchdogOutput string, watchdogErr error) (*RepairResult, error)
}

type OrchestratorConfig struct {
	PollInterval     time.Duration
	MaxRetries       int
	Concurrency      int
	MaxBudgetUSD     float64
	OCCMaxRetries    int
	OCCBackoffBase   time.Duration
	OCCBackoffFactor float64
	MaxDuration      time.Duration
}

type Orchestrator struct {
	repo           domain.StateRepository
	registry       Registry
	llmClient      domain.LLMClient
	validator      Validator
	scheduler      *Scheduler
	git            *GitClient
	rebaseQueue    *RebaseQueue
	evaluator      *TestValidator
	vcsClient      domain.VCSClient
	cfg            OrchestratorConfig
	mailbox        *CommandMailbox
	watchdogRepair RepairHandler
	storyStartedAt time.Time
}

func NewOrchestrator(
	repo domain.StateRepository,
	reg Registry,
	client domain.LLMClient,
	val Validator,
	sched *Scheduler,
	git *GitClient,
	queue *RebaseQueue,
	eval *TestValidator,
	vcsClient domain.VCSClient,
	cfg OrchestratorConfig,
	mailbox *CommandMailbox,
	watchdogRepair RepairHandler,
) *Orchestrator {
	return &Orchestrator{
		repo:           repo,
		registry:       reg,
		llmClient:      client,
		validator:      val,
		scheduler:      sched,
		git:            git,
		rebaseQueue:    queue,
		evaluator:      eval,
		vcsClient:      vcsClient,
		cfg:            cfg,
		mailbox:        mailbox,
		watchdogRepair: watchdogRepair,
	}
}

// Start runs the polling loop
func (o *Orchestrator) Start(ctx context.Context) error {
	for {
		if err := o.RunOnce(ctx); err != nil {
			fmt.Fprintf(os.Stderr, "Orchestrator error: %v\n", err)
		}
		var wakeup <-chan struct{}
		if o.mailbox != nil {
			wakeup = o.mailbox.Wakeup()
		}
		if err := SleepWithInterrupt(ctx, o.cfg.PollInterval, wakeup); err != nil {
			if errors.Is(err, ErrInterrupted) {
				continue
			}
			return err
		}
	}
}

// RunOnce runs a single cycle of the event loop
func (o *Orchestrator) RunOnce(ctx context.Context) error {
	ctx, span := telemetry.Tracer().Start(ctx, "RunOnce",
		trace.WithAttributes(
			attribute.Int("concurrency", o.cfg.Concurrency),
			attribute.Int("occ_max_retries", o.cfg.OCCMaxRetries),
			attribute.String("poll_interval", o.cfg.PollInterval.String()),
		))
	defer span.End()

	state, err := o.repo.Load(ctx)
	if err != nil {
		return err
	}

	// 1. Observe Phase: File indexing and sync
	if err := o.syncWorkspaceFiles(ctx, state); err != nil {
		return err
	}

	// 2. Scheduler check: find ready tasks
	ready := o.scheduler.GetReadyTasks(state, o.cfg.Concurrency)

	// 2a. Story-level wall clock enforcement. When max_duration is configured
	// (> 0) and the story has been running longer than the limit, fail every
	// non-finished task and mark the story as FAILED so the daemon stops
	// spending LLM budget on a stuck story. The start time is the first cycle
	// in which any task became ready.
	if o.cfg.MaxDuration > 0 && len(state.Tasks) > 0 {
		if o.storyStartedAt.IsZero() && len(ready) > 0 {
			o.storyStartedAt = time.Now()
		}
		if !o.storyStartedAt.IsZero() && time.Since(o.storyStartedAt) > o.cfg.MaxDuration && state.StoryStatus == domain.StoryIdle {
			elapsed := time.Since(o.storyStartedAt)
			fmt.Printf("Orchestrator: story exceeded max_duration %s (elapsed %s); failing remaining tasks and aborting story.\n", o.cfg.MaxDuration, elapsed.Truncate(time.Second))
			_ = o.updateStateWithRetry(ctx, func(st *domain.State) error {
				for i := range st.Tasks {
					if st.Tasks[i].Status != domain.TaskSuccess && st.Tasks[i].Status != domain.TaskFailed {
						st.Tasks[i].Status = domain.TaskFailed
						st.Tasks[i].FailureLog = fmt.Sprintf("story exceeded max_duration %s (elapsed %s)", o.cfg.MaxDuration, elapsed.Truncate(time.Second))
						st.Tasks[i].UpdatedAt = time.Now()
					}
				}
				st.BuildStatus = domain.BuildFailing
				st.StoryStatus = domain.StoryFailed
				return nil
			})
			return nil
		}
	}

	if len(ready) == 0 {
		// Finalize exactly once: guarded by StoryStatus (not BuildStatus, which
		// may have been set to FAILING mid-run by a failing task and could
		// recover on retry). StoryStatus transitions Idle -> Success/Failed
		// exactly once when all tasks are finished.
		if o.allTasksFinished(state) && state.StoryStatus == domain.StoryIdle {
			buildOK := o.allTasksSucceeded(state)
			if buildOK {
				if finalErr := o.FinalizeUserStory(ctx, state); finalErr != nil {
					fmt.Fprintf(os.Stderr, "Orchestrator: finalization failed: %v\n", finalErr)
				}
			} else {
				fmt.Printf("Orchestrator: one or more tasks failed test validation; marking build as FAILING and skipping release finalization.\n")
			}
			_ = o.updateStateWithRetry(ctx, func(st *domain.State) error {
				if buildOK {
					st.BuildStatus = domain.BuildPassing
					st.StoryStatus = domain.StorySuccess
				} else {
					st.BuildStatus = domain.BuildFailing
					st.StoryStatus = domain.StoryFailed
				}
				return nil
			})
		}
		return nil
	}

	fmt.Printf("Orchestrator: Found %d ready task(s) to execute in this cycle\n", len(ready))

	var wg sync.WaitGroup
	// 3. Dispatch ready tasks
	for _, task := range ready {
		wg.Add(1)
		go func(t domain.Task) {
			defer wg.Done()
			o.executeTask(ctx, state.ID, t.ID)
		}(task)
	}

	wg.Wait()
	return nil
}

func (o *Orchestrator) updateStateWithRetry(ctx context.Context, updateFn func(state *domain.State) error) error {
	maxRetries := o.cfg.OCCMaxRetries
	if maxRetries <= 0 {
		maxRetries = 5
	}
	backoff := o.cfg.OCCBackoffBase
	if backoff <= 0 {
		backoff = 50 * time.Millisecond
	}
	factor := o.cfg.OCCBackoffFactor
	if factor <= 0 {
		factor = 2.0
	}

	for attempt := 0; attempt <= maxRetries; attempt++ {
		state, err := o.repo.Load(ctx)
		if err != nil {
			return err
		}

		if err := updateFn(state); err != nil {
			return err
		}

		err = o.repo.Save(ctx, state)
		if err == nil {
			return nil
		}

		if !errors.Is(err, domain.ErrVersionConflict) {
			return err
		}

		if attempt == maxRetries {
			return fmt.Errorf("state update failed after %d retries due to OCC conflict: %w", maxRetries, err)
		}

		sleepDur := time.Duration(float64(backoff) * math.Pow(factor, float64(attempt)))

		var wakeup <-chan struct{}
		if o.mailbox != nil {
			wakeup = o.mailbox.Wakeup()
		}
		if err := SleepWithInterrupt(ctx, sleepDur, wakeup); err != nil {
			if errors.Is(err, ErrInterrupted) {
				return fmt.Errorf("state update interrupted by incoming command: %w", err)
			}
			return err
		}
	}
	return nil
}

func (o *Orchestrator) markTaskFailed(ctx context.Context, taskID, reason string) {
	_ = o.updateStateWithRetry(ctx, func(st *domain.State) error {
		for i := range st.Tasks {
			if st.Tasks[i].ID == taskID {
				st.Tasks[i].Status = domain.TaskFailed
				st.Tasks[i].UpdatedAt = time.Now()
				break
			}
		}
		st.LastActions = append(st.LastActions, domain.Action{
			Timestamp: time.Now(),
			Tool:      "execute",
			Success:   false,
			Result:    reason,
		})
		return nil
	})
}

func (o *Orchestrator) executeTask(ctx context.Context, stateID, taskID string) {
	ctx, span := telemetry.Tracer().Start(ctx, "executeTask",
		trace.WithAttributes(attribute.String("task.id", taskID)))
	defer span.End()

	// Spawns worker branch, performs checkouts, updates task PENDING -> IN_PROGRESS,
	// runs Tester & Generator, validates tests, merges back if success, and cleans worktrees.
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

	// Inherit target files recursively from dependencies
	task.TargetFiles = collectTargetFilesRecursively(*task, state.Tasks)

	fmt.Printf("Orchestrator: Task %s (%s) is starting...\n", taskID, task.Title)

	baseBranch := state.Metadata.BaseBranch
	if baseBranch == "" {
		baseBranch = "main"
	}
	integrationBranch := state.Metadata.IntegrationBranch
	if integrationBranch == "" {
		integrationBranch = fmt.Sprintf("noctifab/feature-%s", state.ID[:8])
	}

	// Update task status
	err = o.updateStateWithRetry(ctx, func(st *domain.State) error {
		for i := range st.Tasks {
			if st.Tasks[i].ID == taskID {
				st.Tasks[i].Status = domain.TaskInProgress
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

	// Ensure integration branch exists
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
		// Does not exist, create it from base branch
		if _, err := o.git.Run(ctx, true, "checkout", baseBranch); err != nil {
			fmt.Fprintf(os.Stderr, "Orchestrator: Failed to checkout base branch %s: %v\n", baseBranch, err)
			o.markTaskFailed(ctx, taskID, fmt.Sprintf("Failed to checkout base branch: %v", err))
			return
		}
		if _, err := o.git.Run(ctx, true, "checkout", "-b", integrationBranch); err != nil {
			fmt.Fprintf(os.Stderr, "Orchestrator: Failed to create integration branch %s: %v\n", integrationBranch, err)
			o.markTaskFailed(ctx, taskID, fmt.Sprintf("Failed to create integration branch: %v", err))
			return
		}
	}

	// Switch to integration branch first to branch off accumulative state
	if _, err := o.git.Run(ctx, true, "checkout", integrationBranch); err != nil {
		fmt.Fprintf(os.Stderr, "Orchestrator: Failed to checkout integration branch %s: %v\n", integrationBranch, err)
		o.markTaskFailed(ctx, taskID, fmt.Sprintf("Failed to checkout integration branch: %v", err))
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
		// Revert refactor/fix commits (both test and feat) to start retry from base minimal implementation
		for {
			lastMsg, err := o.git.Run(ctx, false, "log", "-1", "--pretty=%s")
			if err != nil {
				break
			}
			lastMsg = strings.TrimSpace(lastMsg)
			if strings.HasPrefix(lastMsg, "test(core): refactor tests") ||
				strings.HasPrefix(lastMsg, "test(core): fix/refactor tests") ||
				strings.HasPrefix(lastMsg, "feat(core): refactor implementation") ||
				strings.HasPrefix(lastMsg, "feat(core): fix/refactor implementation") ||
				strings.HasPrefix(lastMsg, "feat(core): refactor/resolve task") {
				_, _ = o.git.Run(ctx, true, "reset", "--hard", "HEAD~1")
			} else {
				break
			}
		}
	} else {
		// Force delete worker branch if left over from a previous crashed run or not a retry
		_, _ = o.git.Run(ctx, true, "branch", "-D", branchName)

		// Create worker branch from integrationBranch
		_, err = o.git.Run(ctx, true, "checkout", "-b", branchName)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Orchestrator: Failed to create worker branch %s: %v\n", branchName, err)
			o.markTaskFailed(ctx, taskID, fmt.Sprintf("Failed to create worker branch: %v", err))
			return
		}
	}

	// Inject target files context
	var fileContexts []string
	for _, file := range task.TargetFiles {
		fullPath, err := resolveSandboxPath(state.ProjectPath, file)
		if err == nil {
			if content, err := os.ReadFile(fullPath); err == nil {
				fileContexts = append(fileContexts, fmt.Sprintf("File %s:\n```\n%s\n```", file, string(content)))
			}
		}
	}

	if task.Retries == 0 {
		// 1. Run Generator Agent (role "generator") to implement minimal functionality
		minimalGenPrompt := fmt.Sprintf("Execute task: %s - %s\n\nFocus on creating the minimal implementation/functionality to fulfill the task requirements. The tests will be written in a later phase.", task.Title, task.Description)
		o.RunGeneratorAgent(ctx, *task, state, fileContexts, "", minimalGenPrompt)

		// Stage and commit minimal implementation
		statusOut, _ = o.git.Run(ctx, false, "status", "--porcelain")
		if strings.TrimSpace(statusOut) != "" {
			_, _ = o.git.Run(ctx, true, "add", "--all", "--", ":!.noctifab")
			stagedOut, _ = o.git.Run(ctx, false, "diff", "--cached", "--name-only")
			if strings.TrimSpace(stagedOut) != "" {
				commitMsg := fmt.Sprintf("feat(core): implement minimal functionality for task %s - %s", taskID, task.Title)
				_, commitErr := o.git.Run(ctx, true, "commit", "-m", commitMsg)
				if commitErr != nil {
					fmt.Fprintf(os.Stderr, "Orchestrator: Git commit failed for task %s minimal implementation: %v\n", taskID, commitErr)
				}
			}
		}

		// 2. Run Test Writer Agent (role "tester") to write tests against the minimal implementation
		testerPrompt := fmt.Sprintf("Write tests for task: %s - %s\n\nThe minimal implementation has already been created. Write tests to verify this implementation, including unit and integration tests as specified in the guidelines.", task.Title, task.Description)
		o.RunTesterAgent(ctx, *task, state, fileContexts, testerPrompt)

		// Stage and commit tests
		statusOut, _ = o.git.Run(ctx, false, "status", "--porcelain")
		if strings.TrimSpace(statusOut) != "" {
			_, _ = o.git.Run(ctx, true, "add", "--all", "--", ":!.noctifab")
			stagedOut, _ = o.git.Run(ctx, false, "diff", "--cached", "--name-only")
			if strings.TrimSpace(stagedOut) != "" {
				commitMsg := fmt.Sprintf("test(core): write tests for task %s - %s", taskID, task.Title)
				_, commitErr := o.git.Run(ctx, true, "commit", "-m", commitMsg)
				if commitErr != nil {
					fmt.Fprintf(os.Stderr, "Orchestrator: Git commit failed for task %s tests: %v\n", taskID, commitErr)
				}
			}
		}

		// Read recently written tests from git to pass to the Generator Agent for the Refactor phase
		recentTestsContext := ""
		diffOut, diffErr := o.git.Run(ctx, false, "show", "--name-only", "--format=", "HEAD")
		if diffErr == nil {
			var testFileContexts []string
			for _, file := range strings.Split(diffOut, "\n") {
				file = strings.TrimSpace(file)
				if file != "" && strings.Contains(file, "tests/") {
					fullPath, err := resolveSandboxPath(state.ProjectPath, file)
					if err == nil {
						if content, err := os.ReadFile(fullPath); err == nil {
							testFileContexts = append(testFileContexts, fmt.Sprintf("Test File %s:\n```\n%s\n```", file, string(content)))
						}
					}
				}
			}
			if len(testFileContexts) > 0 {
				recentTestsContext = "\n\nWritten tests context:\n" + strings.Join(testFileContexts, "\n\n")
			}
		}

		// 3a. Run Generator Agent (role "generator") to refactor/improve the implementation and tests to pass
		refactorGenPrompt := fmt.Sprintf("Execute task: %s - %s\n\nRefactor the implementation to make the code better and prepare it to pass all tests. You may update both the implementation files and the test files if needed.", task.Title, task.Description)
		o.RunGeneratorAgent(ctx, *task, state, fileContexts, recentTestsContext, refactorGenPrompt)

		// Stage and commit refactoring changes
		statusOut, _ = o.git.Run(ctx, false, "status", "--porcelain")
		if strings.TrimSpace(statusOut) != "" {
			_, _ = o.git.Run(ctx, true, "add", "--all", "--", ":!.noctifab")
			stagedOut, _ = o.git.Run(ctx, false, "diff", "--cached", "--name-only")
			if strings.TrimSpace(stagedOut) != "" {
				commitMsg := fmt.Sprintf("feat(core): refactor implementation for task %s - %s", taskID, task.Title)
				_, commitErr := o.git.Run(ctx, true, "commit", "-m", commitMsg)
				if commitErr != nil {
					fmt.Fprintf(os.Stderr, "Orchestrator: Git commit failed for task %s refactor: %v\n", taskID, commitErr)
				}
			}
		}

		// 3b. Run Test Writer Agent (role "tester") to refactor/improve/align the tests
		refactorTestPrompt := fmt.Sprintf("Write/Refactor tests for task: %s - %s\n\nThe implementation has been refactored/improved. Refactor the tests to align them with the updated code, improve coverage, and ensure they are correct.", task.Title, task.Description)
		o.RunTesterAgent(ctx, *task, state, fileContexts, refactorTestPrompt)

		// Stage and commit refactored tests
		statusOut, _ = o.git.Run(ctx, false, "status", "--porcelain")
		if strings.TrimSpace(statusOut) != "" {
			_, _ = o.git.Run(ctx, true, "add", "--all", "--", ":!.noctifab")
			stagedOut, _ = o.git.Run(ctx, false, "diff", "--cached", "--name-only")
			if strings.TrimSpace(stagedOut) != "" {
				commitMsg := fmt.Sprintf("test(core): refactor tests for task %s - %s", taskID, task.Title)
				_, commitErr := o.git.Run(ctx, true, "commit", "-m", commitMsg)
				if commitErr != nil {
					fmt.Fprintf(os.Stderr, "Orchestrator: Git commit failed for task %s test refactor: %v\n", taskID, commitErr)
				}
			}
		}

	} else {
		// Retries loop: Generator always runs first, followed by Tester, then Validator

		// 1. Run Generator Agent to fix/refactor implementation
		fixGenPrompt := fmt.Sprintf("Execute task: %s - %s\n\nRefactor and fix the implementation to resolve the previous failures.", task.Title, task.Description)
		o.RunGeneratorAgent(ctx, *task, state, fileContexts, "", fixGenPrompt)

		// Stage and commit fixes
		statusOut, _ = o.git.Run(ctx, false, "status", "--porcelain")
		if strings.TrimSpace(statusOut) != "" {
			_, _ = o.git.Run(ctx, true, "add", "--all", "--", ":!.noctifab")
			stagedOut, _ = o.git.Run(ctx, false, "diff", "--cached", "--name-only")
			if strings.TrimSpace(stagedOut) != "" {
				commitMsg := fmt.Sprintf("feat(core): fix/refactor implementation for task %s - %s", taskID, task.Title)
				_, commitErr := o.git.Run(ctx, true, "commit", "-m", commitMsg)
				if commitErr != nil {
					fmt.Fprintf(os.Stderr, "Orchestrator: Git commit failed for task %s fix implementation: %v\n", taskID, commitErr)
				}
			}
		}

		// 2. Run Test Writer Agent to fix/refactor tests to align with updated implementation
		fixTestPrompt := fmt.Sprintf("Write/Refactor tests for task: %s - %s\n\nRefactor and fix the tests to resolve the previous failures and align them with the updated code.", task.Title, task.Description)
		o.RunTesterAgent(ctx, *task, state, fileContexts, fixTestPrompt)

		// Stage and commit test fixes
		statusOut, _ = o.git.Run(ctx, false, "status", "--porcelain")
		if strings.TrimSpace(statusOut) != "" {
			_, _ = o.git.Run(ctx, true, "add", "--all", "--", ":!.noctifab")
			stagedOut, _ = o.git.Run(ctx, false, "diff", "--cached", "--name-only")
			if strings.TrimSpace(stagedOut) != "" {
				commitMsg := fmt.Sprintf("test(core): fix/refactor tests for task %s - %s", taskID, task.Title)
				_, commitErr := o.git.Run(ctx, true, "commit", "-m", commitMsg)
				if commitErr != nil {
					fmt.Fprintf(os.Stderr, "Orchestrator: Git commit failed for task %s fix tests: %v\n", taskID, commitErr)
				}
			}
		}
	}

	fmt.Printf("Orchestrator: Task %s running test validation...\n", taskID)
	// Run test suite validation
	passed, logMsg, _ := o.evaluator.ValidateTask(ctx, state, *task)

	if !passed && o.watchdogRepair != nil {
		category := CategorizeFailureLog(logMsg)
		if category == FailureTimeout {
			fmt.Printf("Orchestrator: Task %s detected timeout failure. Attempting repair...\n", taskID)
			repairCtx, repairCancel := context.WithTimeout(ctx, 10*time.Minute)
			result, repairErr := o.watchdogRepair.AttemptRepair(repairCtx, state, *task, logMsg, fmt.Errorf("test validation failed: timeout"))
			repairCancel()
			if repairErr == nil && result != nil && result.Success {
				fmt.Printf("Orchestrator: Task %s repaired successfully. Re-running validation...\n", taskID)
				passed, logMsg, _ = o.evaluator.ValidateTask(ctx, state, *task)
			} else {
				if repairErr != nil {
					fmt.Fprintf(os.Stderr, "Orchestrator: Task %s repair failed: %v\n", taskID, repairErr)
				} else {
					fmt.Fprintf(os.Stderr, "Orchestrator: Task %s repair exhausted %d attempts without success\n", taskID, result.Attempts)
				}
			}
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
			targetTask.FailureLog = ""
		} else {
			targetTask.Retries++
			targetTask.FailureLog = logMsg
			if targetTask.Retries >= targetTask.MaxRetries {
				targetTask.Status = domain.TaskFailed
			} else {
				targetTask.Status = domain.TaskPending
			}
			// A failing validation means the build does not compile or tests do not
			// pass. Flip the build status to FAILING immediately so the operator
			// and the auto-merge policy see the red signal instead of UNKNOWN.
			st.BuildStatus = domain.BuildFailing
		}
		targetTask.UpdatedAt = time.Now()

		st.LastActions = append(st.LastActions, domain.Action{
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

	if passed {
		fmt.Printf("Orchestrator: Task %s completed successfully!\n", taskID)
		// Merge back sequentially into integrationBranch using RebaseQueue
		_ = o.rebaseQueue.Push(ctx, branchName, integrationBranch)

		// Clean up branch
		_, _ = o.git.Run(ctx, true, "checkout", integrationBranch)
		_, _ = o.git.Run(ctx, true, "branch", "-D", branchName)
	} else {
		fmt.Printf("Orchestrator: Task %s failed test validation. Retrying or marking FAILED.\n", taskID)
		if task.Retries+1 >= task.MaxRetries {
			_, _ = o.git.Run(ctx, true, "checkout", integrationBranch)
			_, _ = o.git.Run(ctx, true, "branch", "-D", branchName)
		} else {
			_, _ = o.git.Run(ctx, true, "checkout", integrationBranch)
		}
	}

	o.scheduler.ReleaseLocks(taskID)
}

func collectTargetFilesRecursively(task domain.Task, tasks []domain.Task) []string {
	// Build map of ID/Title to Task
	taskMap := make(map[string]domain.Task)
	for _, t := range tasks {
		taskMap[t.ID] = t
		taskMap[t.Title] = t
	}

	visited := make(map[string]bool)
	var files []string
	var visit func(t domain.Task)
	visit = func(t domain.Task) {
		if visited[t.ID] {
			return
		}
		visited[t.ID] = true
		// Add target files of this task
		files = append(files, t.TargetFiles...)
		// Recurse on dependencies
		for _, dep := range t.DependsOn {
			if parent, exists := taskMap[dep]; exists {
				visit(parent)
			}
		}
	}

	visit(task)

	// Deduplicate
	uniqueFiles := make([]string, 0, len(files))
	seen := make(map[string]bool)
	for _, f := range files {
		if !seen[f] {
			seen[f] = true
			uniqueFiles = append(uniqueFiles, f)
		}
	}
	sort.Strings(uniqueFiles)
	return uniqueFiles
}
