package usecase

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/diegojromerolopez/noctifab/pkg/domain"
)

type OrchestratorConfig struct {
	PollInterval time.Duration
	MaxRetries   int
	Concurrency  int
	MaxBudgetUSD float64
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

	// 3. Dispatch ready tasks
	for _, task := range ready {
		go o.executeTask(ctx, state.ID, task.ID)
	}

	return nil
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

	// Update task status
	task.Status = domain.TaskInProgress
	task.UpdatedAt = time.Now()
	_ = o.repo.Save(ctx, state)

	branchName := fmt.Sprintf("noctifab/task-%s-worker", task.ID)

	// Create worker branch
	_, err = o.git.Run(ctx, true, "checkout", "-b", branchName)
	if err != nil {
		task.Status = domain.TaskFailed
		task.UpdatedAt = time.Now()
		_ = o.repo.Save(ctx, state)
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
				task.Status = domain.TaskFailed
				state.LastActions = append(state.LastActions, domain.Action{
					Timestamp: time.Now(),
					Tool:      action.Tool,
					Success:   false,
					Result:    reason,
				})
				_ = o.repo.Save(ctx, state)
				o.scheduler.ReleaseLocks(task.ID)
				_, _ = o.git.Run(ctx, true, "checkout", "main")
				_, _ = o.git.Run(ctx, true, "branch", "-D", branchName)
				return
			}

			tool, ok := o.registry.Get(action.Tool)
			if ok {
				_, _ = tool.Execute(genCtx, state, action.Args)
			}
		}
	}

	// Run Evaluator holdout tests
	passed, logMsg, _ := o.evaluator.EvaluateTask(ctx, state, *task, "tests/holdout")

	state, _ = o.repo.Load(ctx)
	for i := range state.Tasks {
		if state.Tasks[i].ID == task.ID {
			task = &state.Tasks[i]
			break
		}
	}

	if passed {
		task.Status = domain.TaskSuccess
		
		// Push branch to remote and submit/merge Pull Request
		_, pushErr := o.git.Run(ctx, true, "push", "origin", branchName)
		if pushErr == nil {
			prURL, prErr := o.vcsClient.CreatePullRequest(ctx, "Resolve task "+task.ID, "Automated Level 4 merge for task: "+task.Title, branchName, "main")
			if prErr == nil {
				_ = o.vcsClient.MergePullRequest(ctx, prURL)
			}
		}

		// Merge back sequentially using RebaseQueue
		_ = o.rebaseQueue.Push(ctx, branchName, "main")
	} else {
		task.Retries++
		if task.Retries >= task.MaxRetries {
			task.Status = domain.TaskFailed
		} else {
			task.Status = domain.TaskPending
		}
	}

	task.UpdatedAt = time.Now()
	state.LastActions = append(state.LastActions, domain.Action{
		Timestamp: time.Now(),
		Tool:      "evaluate",
		Success:   passed,
		Result:    logMsg,
	})

	_ = o.repo.Save(ctx, state)
	o.scheduler.ReleaseLocks(task.ID)

	// Clean up branch
	_, _ = o.git.Run(ctx, true, "checkout", "main")
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
