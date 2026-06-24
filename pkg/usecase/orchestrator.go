package usecase

import (
	"context"
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
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

	// Switch to base branch first to branch off clean state
	if _, err := o.git.Run(ctx, true, "checkout", baseBranch); err != nil {
		fmt.Fprintf(os.Stderr, "Orchestrator: Failed to checkout base branch %s: %v\n", baseBranch, err)
		o.markTaskFailed(ctx, taskID, fmt.Sprintf("Failed to checkout base branch: %v", err))
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

	// Run Generator (simplified LLM completion call)
	prompt := fmt.Sprintf("Execute task: %s - %s", task.Title, task.Description)
	genCtx := context.WithValue(ctx, AgentRoleKey, "generator")
	resp, err := o.llmClient.Complete(genCtx, prompt)
	if err == nil {
		// Run tool actions
		for _, action := range resp.Actions {
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
		// Push branch to remote and submit/merge Pull Request
		_, pushErr := o.git.Run(ctx, true, "push", "origin", branchName)
		if pushErr == nil {
			prURL, prErr := o.vcsClient.CreatePullRequest(ctx, "Resolve task "+taskID, "Automated Level 4 merge for task: "+task.Title, branchName, "main")
			if prErr == nil {
				_ = o.vcsClient.MergePullRequest(ctx, prURL)
			}
		}

		// Merge back sequentially using RebaseQueue
		_ = o.rebaseQueue.Push(ctx, branchName, baseBranch)
	} else {
		fmt.Printf("Orchestrator: Task %s failed holdout tests. Retrying or marking FAILED.\n", taskID)
	}

	o.scheduler.ReleaseLocks(taskID)

	// Clean up branch
	_, _ = o.git.Run(ctx, true, "checkout", baseBranch)
	_, _ = o.git.Run(ctx, true, "branch", "-D", branchName)
}

func (o *Orchestrator) syncWorkspaceFiles(ctx context.Context, state *domain.State) error {
	var files []domain.FileInfo

	// Try git-aware scanning first
	out, err := o.git.Run(ctx, false, "ls-files", "-co", "--exclude-standard")
	if err == nil {
		lines := strings.Split(out, "\n")
		for _, rel := range lines {
			rel = strings.TrimSpace(rel)
			if rel == "" {
				continue
			}
			parts := strings.Split(rel, string(filepath.Separator))
			ignored := false
			for _, part := range parts {
				if part == ".noctifab" || part == ".git" || part == "node_modules" || part == "vendor" {
					ignored = true
					break
				}
			}
			if ignored {
				continue
			}
			abs := filepath.Join(state.ProjectPath, rel)
			info, err := os.Stat(abs)
			if err == nil && !info.IsDir() {
				files = append(files, domain.FileInfo{
					Path:         rel,
					Size:         info.Size(),
					LastModified: info.ModTime(),
				})
			}
		}
		state.Files = files
		return o.repo.Save(ctx, state)
	}

	// Fallback to WalkDir if git command fails or is not a repo
	err = filepath.WalkDir(state.ProjectPath, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		rel, err := filepath.Rel(state.ProjectPath, path)
		if err != nil {
			return nil
		}
		if rel == "." || rel == ".." {
			return nil
		}
		parts := strings.Split(rel, string(filepath.Separator))
		for _, part := range parts {
			if part == ".noctifab" || part == ".git" || part == "node_modules" || part == "vendor" {
				if d.IsDir() {
					return filepath.SkipDir
				}
				return nil
			}
		}
		if !d.IsDir() {
			info, err := d.Info()
			if err == nil {
				files = append(files, domain.FileInfo{
					Path:         rel,
					Size:         info.Size(),
					LastModified: info.ModTime(),
				})
			}
		}
		return nil
	})

	if err != nil {
		return err
	}
	state.Files = files
	return o.repo.Save(ctx, state)
}
