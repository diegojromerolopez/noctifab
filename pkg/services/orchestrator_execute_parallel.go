package services

import (
	"context"
	"fmt"
	"sync"

	"github.com/diegojromerolopez/noctifab/pkg/domain"
)

// executeParallelCoSynthesisTurn executes the generator and tester agents simultaneously
// in parallel goroutines during Turn 1, leveraging contract-first interface consensus.
func (o *Orchestrator) executeParallelCoSynthesisTurn(
	ctx context.Context,
	task *domain.Task,
	taskState *domain.State,
	taskGit *GitClient,
	fileContexts []string,
	taskID string,
) string {
	// 1. Phase A: Ensure minimal compilation stub files exist before running agents
	o.ensureTargetStubFilesExist(taskState.ProjectPath, task)

	// 2. Phase B: Dispatch Tester Agent and Generator Agent simultaneously in parallel goroutines
	o.updateTaskProgress(ctx, taskID, 35)
	fmt.Printf("⚡ [Parallel Co-Synthesis] Task %s: Launching Generator and Tester agents simultaneously in parallel goroutines...\n", taskID)

	var wg sync.WaitGroup
	wg.Add(2)

	go func() {
		defer wg.Done()
		o.RunTesterAgent(ctx, *task, taskState, fileContexts, "write", "")
	}()

	go func() {
		defer wg.Done()
		o.RunGeneratorAgent(ctx, *task, taskState, fileContexts, "", "implement")
	}()

	wg.Wait()

	// Commit co-synthesized test and implementation artifacts
	_ = o.stageAndCommit(ctx, taskGit, taskID, "feat(core): co-synthesize implementation and tests for task %s - %s", task.Title)

	// Post-CoSynthesis Anti-Stub Audit on generator output
	if violations := o.auditGeneratorFunctionalOutput(taskState.ProjectPath, task.TargetFiles); len(violations) > 0 {
		fmt.Printf("⚠️  [Co-Synthesis Gate] Task %s: Generator produced %d non-functional stub(s). Triggering remediation turn...\n", taskID, len(violations))
		remediationCtx := append(fileContexts, o.formatAntiStubViolations(violations))
		o.RunGeneratorAgent(ctx, *task, taskState, remediationCtx, "", "fix")
		_ = o.stageAndCommit(ctx, taskGit, taskID, "feat(core): remediate non-functional stubs for task %s - %s", task.Title)
	}

	// Post-CoSynthesis Anti-Vacuous Audit on test suite
	if testViolations := o.auditTesterTestOutput(taskState.ProjectPath); len(testViolations) > 0 {
		fmt.Printf("⚠️  [Co-Synthesis Gate] Task %s: Tester produced %d vacuous/tautological test violation(s). Triggering remediation turn...\n", taskID, len(testViolations))
		remediationCtx := append(fileContexts, o.formatTesterAntiStubViolations(testViolations))
		o.RunTesterAgent(ctx, *task, taskState, remediationCtx, "fix", "")
		_ = o.stageAndCommit(ctx, taskGit, taskID, "test(core): remediate vacuous tests for task %s - %s", task.Title)
	}

	// Fast Exit on Verified Green: If tests are already 100% passing after co-synthesis,
	// skip the redundant Generator Refactor turn!
	if o.evaluator != nil {
		if passed, _, err := o.evaluator.ValidateTask(ctx, taskState, *task); passed && err == nil {
			fmt.Printf("🚀 [Fast Exit on Verified Green] Task %s: co-synthesized tests are 100%% green! Skipping refactor turn.\n", taskID)
			_ = o.stageAndCommit(ctx, taskGit, taskID, "chore(core): sync workspace state after fast-exit green for task %s", task.Title)
			return ""
		}
	}

	// Read recently written tests from git to pass to the Generator Agent for the Refactor phase
	recentTestsContext := o.collectRecentTestsContext(ctx, taskGit, taskState.ProjectPath)

	// Refactor Turn: Generator Agent improves implementation to pass tests
	o.updateTaskProgress(ctx, taskID, 75)
	generatorHeadBefore, generatorHeadErr := taskGit.Run(ctx, false, "rev-parse", "HEAD")
	o.RunGeneratorAgent(ctx, *task, taskState, fileContexts, recentTestsContext, "refactor")

	qaBlocked := ""
	if commitErr := o.stageAndCommit(ctx, taskGit, taskID, "feat(core): refactor implementation for task %s - %s", task.Title); commitErr != nil {
		qaBlocked = "generator_commit_failed"
	}
	generatorHeadAfter, generatorAfterErr := taskGit.Run(ctx, false, "rev-parse", "HEAD")
	if qaBlocked == "" {
		qaBlocked = o.evaluateGeneratorTurnResult(ctx, taskState, task, generatorHeadBefore, generatorHeadAfter, generatorHeadErr, generatorAfterErr)
	}
	return qaBlocked
}
