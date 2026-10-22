package services

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"github.com/diegojromerolopez/noctifab/pkg/domain"
	"github.com/diegojromerolopez/noctifab/pkg/infrastructure/config"
)

// HypothesisResult stores the empirical outcome of a parallel hypothesis execution.
type HypothesisResult struct {
	Index       int
	WorktreeDir string
	TaskGit     *GitClient
	Cleanup     func()
	Passed      bool
	PassRate    float64
	LogMsg      string
	Err         error
	Diff        string
}

// ExecuteSinglePassHypotheses executes single-pass task co-synthesis using parallel hypothesis worktrees.
// When Hypotheses is enabled with >= 2 agents, it generates and tests N candidates in parallel,
// resolving the final output via either "correct" (best scoring candidate) or "mixture" (synthesized reconciliation).
func (o *Orchestrator) ExecuteSinglePassHypotheses(
	ctx context.Context,
	task *domain.Task,
	taskState *domain.State,
	taskGit *GitClient,
	fileContexts []string,
	taskID string,
	action string,
) {
	hypCfg := o.cfg.Hypotheses
	agentCount := hypCfg.GetAgents()

	if !hypCfg.IsEnabled() || agentCount <= 1 || !o.cfg.UseWorktrees {
		// Single hypothesis fast path
		o.RunGeneratorAgent(ctx, *task, taskState, fileContexts, "", action)
		if violations := o.auditGeneratorFunctionalOutput(taskState.ProjectPath, task.TargetFiles); len(violations) > 0 {
			fmt.Printf("⚠️  [Single-Pass Gate] Task %s: Generator produced %d non-functional stub(s). Triggering remediation turn...\n", taskID, len(violations))
			remediationCtx := append(fileContexts, o.formatAntiStubViolations(violations))
			o.RunGeneratorAgent(ctx, *task, taskState, remediationCtx, "", "single_pass_fix")
		}
		_ = o.stageAndCommit(ctx, taskGit, taskID, "feat(core): implement functionality and tests for task %s - %s", task.Title)
		return
	}

	fmt.Printf("🚀 [Multi-Hypothesis Worktrees] Task %s: Launching %d parallel hypothesis worktrees (Resolution: %s)...\n",
		taskID, agentCount, hypCfg.GetResolution())

	hypCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	var wg sync.WaitGroup
	results := make([]HypothesisResult, agentCount)
	var winnerOnce sync.Once

	for i := 0; i < agentCount; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			res := o.runSingleHypothesis(hypCtx, task, taskState, fileContexts, taskID, action, idx+1)
			results[idx] = res

			// Short-circuit hypothesis racing: under "correct" resolution, if any candidate
			// achieves 100% test pass rate with zero errors, cancel lagging candidates immediately.
			if hypCfg.GetResolution() == config.ResolutionCorrect && res.Passed && res.PassRate >= 100.0 && res.Err == nil {
				winnerOnce.Do(func() {
					fmt.Printf("⚡ [Multi-Hypothesis Racing] Hypothesis %d finished with 100%% green! Short-circuiting lagging candidates...\n", res.Index)
					cancel()
				})
			}
		}(i)
	}

	wg.Wait()
	defer func() {
		for _, r := range results {
			if r.Cleanup != nil {
				r.Cleanup()
			}
		}
	}()

	// Resolve winning hypothesis
	o.resolveHypothesesOutcome(ctx, task, taskState, taskGit, fileContexts, taskID, results, hypCfg)
}

func (o *Orchestrator) runSingleHypothesis(
	ctx context.Context,
	task *domain.Task,
	state *domain.State,
	fileContexts []string,
	taskID string,
	action string,
	hypIndex int,
) HypothesisResult {
	if ctx.Err() != nil {
		return HypothesisResult{Index: hypIndex, Err: ctx.Err()}
	}

	branchName := fmt.Sprintf("noctifab/task-%s-hyp-%d", taskID, hypIndex)
	worktreeDir := filepath.Join(state.ProjectPath, ".noctifab", "worktrees", fmt.Sprintf("task-%s-hyp-%d", taskID, hypIndex))
	_, _ = o.git.Run(context.Background(), true, "worktree", "remove", "--force", worktreeDir)
	_ = os.RemoveAll(worktreeDir)
	_, _ = o.git.Run(context.Background(), true, "worktree", "prune")
	_ = os.MkdirAll(filepath.Dir(worktreeDir), 0755)

	cleanup := func() {
		_, _ = o.git.Run(context.Background(), true, "worktree", "remove", "--force", worktreeDir)
		_ = os.RemoveAll(worktreeDir)
		_, _ = o.git.Run(context.Background(), true, "branch", "-D", branchName)
		_, _ = o.git.Run(context.Background(), true, "worktree", "prune")
	}

	if ctx.Err() != nil {
		return HypothesisResult{Index: hypIndex, WorktreeDir: worktreeDir, Cleanup: cleanup, Err: ctx.Err()}
	}

	_, _ = o.git.Run(context.Background(), true, "branch", "-D", branchName)
	_, err := o.git.Run(context.Background(), true, "worktree", "add", "-b", branchName, worktreeDir, "HEAD")
	if err != nil {
		return HypothesisResult{Index: hypIndex, Cleanup: cleanup, Err: err}
	}

	hypGit := NewGitClient(worktreeDir)
	SeedTaskWorktreeWorkspace(state.ProjectPath, worktreeDir)
	_ = WarmupWorktreeToolchain(ctx, worktreeDir)

	if ctx.Err() != nil {
		return HypothesisResult{Index: hypIndex, WorktreeDir: worktreeDir, TaskGit: hypGit, Cleanup: cleanup, Err: ctx.Err()}
	}

	hypState := *state.Clone()
	hypState.ProjectPath = worktreeDir

	// Run Generator Agent in this isolated worktree
	fmt.Printf("⚡ [Multi-Hypothesis] Running Hypothesis %d in isolated worktree %s...\n", hypIndex, worktreeDir)
	o.RunGeneratorAgent(ctx, *task, &hypState, fileContexts, "", action)
	_ = o.stageAndCommit(ctx, hypGit, taskID, "feat(core): hypothesis %d implementation for task %s", hypIndex, task.Title)

	if ctx.Err() != nil {
		return HypothesisResult{Index: hypIndex, WorktreeDir: worktreeDir, TaskGit: hypGit, Cleanup: cleanup, Err: ctx.Err()}
	}

	// Validate against test suite
	passed := false
	logMsg := ""
	passRate := 0.0
	if o.evaluator != nil {
		p, msg, vErr := o.evaluator.ValidateTask(ctx, &hypState, *task)
		passed = p && vErr == nil
		logMsg = msg
		report := ParseTestOutput(msg, 0)
		if report.Total > 0 {
			passRate = (float64(report.Passed) / float64(report.Total)) * 100.0
		} else if passed {
			passRate = 100.0
		}
	}

	diff, _ := hypGit.Run(context.Background(), false, "diff", "HEAD~1")

	return HypothesisResult{
		Index:       hypIndex,
		WorktreeDir: worktreeDir,
		TaskGit:     hypGit,
		Cleanup:     cleanup,
		Passed:      passed,
		PassRate:    passRate,
		LogMsg:      logMsg,
		Diff:        diff,
	}
}

func (o *Orchestrator) resolveHypothesesOutcome(
	ctx context.Context,
	task *domain.Task,
	taskState *domain.State,
	taskGit *GitClient,
	fileContexts []string,
	taskID string,
	results []HypothesisResult,
	hypCfg config.HypothesesConfig,
) {
	// Pick best candidate
	bestIdx := 0
	highestScore := -1.0
	for i, r := range results {
		if r.Err != nil && !r.Passed {
			continue
		}
		score := r.PassRate
		if r.Passed {
			score += 1000.0 // Prioritize clean pass
		}
		if score > highestScore {
			highestScore = score
			bestIdx = i
		}
	}

	best := results[bestIdx]

	if hypCfg.GetResolution() == config.ResolutionMixture && len(results) >= 2 && !best.Passed {
		// Mixture Resolution: synthesize complementary implementations
		fmt.Printf("🧬 [Multi-Hypothesis Mixture] Attempting synthesis of complementary hypotheses %d and %d...\n",
			results[0].Index, results[1].Index)
		mixtureCtx := append(fileContexts,
			fmt.Sprintf("### HYPOTHESIS 1 DIFF:\n```diff\n%s\n```\nLOGS: %s", results[0].Diff, results[0].LogMsg),
			fmt.Sprintf("### HYPOTHESIS 2 DIFF:\n```diff\n%s\n```\nLOGS: %s", results[1].Diff, results[1].LogMsg),
		)
		o.RunGeneratorAgent(ctx, *task, taskState, mixtureCtx, "", "single_pass_fix")
		_ = o.stageAndCommit(ctx, taskGit, taskID, "feat(core): synthesize hypothesis mixture for task %s - %s", task.Title)

		if o.evaluator != nil {
			if passed, _, err := o.evaluator.ValidateTask(ctx, taskState, *task); passed && err == nil {
				fmt.Printf("✨ [Multi-Hypothesis Mixture] Synthesized mixture passed all tests cleanly!\n")
				return
			}
		}
		fmt.Printf("ℹ [Multi-Hypothesis Mixture] Mixture did not reach 100%% green; falling back to best hypothesis %d.\n", best.Index)
	}

	// Apply winning hypothesis into task branch
	fmt.Printf("🏆 [Multi-Hypothesis Selection] Selected Hypothesis %d (Passed: %v, PassRate: %.1f%%)\n",
		best.Index, best.Passed, best.PassRate)
	if best.TaskGit != nil {
		diff, dErr := best.TaskGit.Run(ctx, false, "diff", "HEAD~1")
		if dErr == nil && len(diff) > 0 {
			patchFile := filepath.Join(taskState.ProjectPath, fmt.Sprintf(".hyp_%d.patch", best.Index))
			if pErr := os.WriteFile(patchFile, []byte(diff), 0644); pErr == nil {
				_, _ = taskGit.Run(ctx, false, "apply", "--3way", patchFile)
				_ = os.Remove(patchFile)
			}
		}
	}
	_ = o.stageAndCommit(ctx, taskGit, taskID, "feat(core): apply winning hypothesis %d for task %s - %s", best.Index, task.Title)
}
