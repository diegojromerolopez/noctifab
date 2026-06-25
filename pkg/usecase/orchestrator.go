package usecase

import (
	"context"
	"errors"
	"fmt"
	"math"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/diegojromerolopez/noctifab/pkg/domain"
)

type OrchestratorConfig struct {
	PollInterval     time.Duration
	MaxRetries       int
	Concurrency      int
	MaxBudgetUSD     float64
	OCCMaxRetries    int
	OCCBackoffBase   time.Duration
	OCCBackoffFactor float64
}

type Orchestrator struct {
	repo        domain.StateRepository
	registry    Registry
	llmClient   domain.LLMClient
	validator   Validator
	scheduler   *Scheduler
	git         *GitClient
	rebaseQueue *RebaseQueue
	evaluator   *HoldoutEvaluator
	vcsClient   domain.VCSClient
	cfg         OrchestratorConfig
}

func NewOrchestrator(
	repo domain.StateRepository,
	reg Registry,
	client domain.LLMClient,
	val Validator,
	sched *Scheduler,
	git *GitClient,
	queue *RebaseQueue,
	eval *HoldoutEvaluator,
	vcsClient domain.VCSClient,
	cfg OrchestratorConfig,
) *Orchestrator {
	return &Orchestrator{
		repo:        repo,
		registry:    reg,
		llmClient:   client,
		validator:   val,
		scheduler:   sched,
		git:         git,
		rebaseQueue: queue,
		evaluator:   eval,
		vcsClient:   vcsClient,
		cfg:         cfg,
	}
}

// Start runs the polling loop
func (o *Orchestrator) Start(ctx context.Context) error {
	ticker := time.NewTicker(o.cfg.PollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			if err := o.RunOnce(ctx); err != nil {
				fmt.Fprintf(os.Stderr, "Orchestrator error: %v\n", err)
			}
		}
	}
}

// RunOnce runs a single cycle of the event loop
func (o *Orchestrator) RunOnce(ctx context.Context) error {
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
	if len(ready) == 0 {
		// If all tasks are completed and build status is still UNKNOWN,
		// delegate to FinalizeUserStory to bump version, push branch, and create PR.
		if o.allTasksFinished(state) && state.BuildStatus == domain.BuildUnknown {
			if finalErr := o.FinalizeUserStory(ctx, state); finalErr != nil {
				fmt.Fprintf(os.Stderr, "Orchestrator: finalization failed: %v\n", finalErr)
			}
			_ = o.updateStateWithRetry(ctx, func(st *domain.State) error {
				st.BuildStatus = domain.BuildPassing
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
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(sleepDur):
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
	// Spawns worker branch, performs checkouts, updates task PENDING -> IN_PROGRESS,
	// runs Generator & Evaluator, merges back if success, and cleans worktrees.
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

	// Ensure integration branch exists
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

	// Force delete worker branch if left over from a previous crashed run
	_, _ = o.git.Run(ctx, true, "branch", "-D", branchName)

	// Create worker branch
	_, err = o.git.Run(ctx, true, "checkout", "-b", branchName)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Orchestrator: Failed to create worker branch %s: %v\n", branchName, err)
		o.markTaskFailed(ctx, taskID, fmt.Sprintf("Failed to create worker branch: %v", err))
		return
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

	prompt := fmt.Sprintf("Execute task: %s - %s", task.Title, task.Description)
	if len(fileContexts) > 0 {
		prompt = fmt.Sprintf("Execute task: %s - %s\n\nExisting files context:\n%s", task.Title, task.Description, strings.Join(fileContexts, "\n\n"))
	}

	genCtx := context.WithValue(ctx, AgentRoleKey, "generator")
	resp, err := o.llmClient.Complete(genCtx, prompt)
	if err == nil {
		fmt.Printf("Orchestrator: Task %s LLM reasoning: %s\n", taskID, resp.Reasoning)
		// Run tool actions
		for _, action := range resp.Actions {
			fmt.Printf("Orchestrator: Task %s LLM action: tool=%s args=%+v\n", taskID, action.Tool, action.Args)
			domainAction := domain.Action{
				Tool: action.Tool,
				Args: action.Args,
			}
			valRes, valErr := o.validator.Validate(genCtx, domainAction, state)
			if valErr != nil || (valRes != nil && !valRes.Allowed) {
				reason := "validation policy blocked execution"
				if valRes != nil && valRes.Reason != "" {
					reason = valRes.Reason
				}
				fmt.Fprintf(os.Stderr, "Orchestrator: Task %s action %s validation failed: %s\n", taskID, action.Tool, reason)
				o.markTaskFailed(ctx, taskID, reason)
				o.scheduler.ReleaseLocks(taskID)
				_, _ = o.git.Run(ctx, true, "checkout", baseBranch)
				_, _ = o.git.Run(ctx, true, "branch", "-D", branchName)
				return
			}

			fmt.Printf("Orchestrator: Task %s executing tool: %s\n", taskID, action.Tool)
			tool, ok := o.registry.Get(action.Tool)
			if ok {
				_, _ = tool.Execute(genCtx, state, action.Args)
			}
		}
	} else {
		fmt.Fprintf(os.Stderr, "Orchestrator: Task %s LLM completion failed: %v\n", taskID, err)
	}

	// Stage and commit changes if any exist
	statusOut, _ := o.git.Run(ctx, false, "status", "--porcelain")
	if strings.TrimSpace(statusOut) != "" {
		_, _ = o.git.Run(ctx, true, "add", "--all", "--", ":!.noctifab")
		// Check again if staged changes exist to avoid empty commits
		stagedOut, _ := o.git.Run(ctx, false, "diff", "--cached", "--name-only")
		if strings.TrimSpace(stagedOut) != "" {
			commitMsg := fmt.Sprintf("feat(core): resolve task %s - %s", taskID, task.Title)
			_, commitErr := o.git.Run(ctx, true, "commit", "-m", commitMsg)
			if commitErr != nil {
				fmt.Fprintf(os.Stderr, "Orchestrator: Git commit failed for task %s: %v\n", taskID, commitErr)
			}
		}
	}

	fmt.Printf("Orchestrator: Task %s running holdout tests...\n", taskID)
	// Run Evaluator holdout tests
	passed, logMsg, _ := o.evaluator.EvaluateTask(ctx, state, *task, "tests/holdout")

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
		} else {
			targetTask.Retries++
			if targetTask.Retries >= targetTask.MaxRetries {
				targetTask.Status = domain.TaskFailed
			} else {
				targetTask.Status = domain.TaskPending
			}
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
	} else {
		fmt.Printf("Orchestrator: Task %s failed holdout tests. Retrying or marking FAILED.\n", taskID)
	}

	o.scheduler.ReleaseLocks(taskID)

	// Clean up branch
	_, _ = o.git.Run(ctx, true, "checkout", integrationBranch)
	_, _ = o.git.Run(ctx, true, "branch", "-D", branchName)
}
