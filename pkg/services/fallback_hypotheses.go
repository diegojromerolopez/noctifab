package services

import (
	"context"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/diegojromerolopez/noctifab/pkg/domain"
	"github.com/diegojromerolopez/noctifab/pkg/infrastructure/prompts"
	"github.com/diegojromerolopez/noctifab/pkg/infrastructure/telemetry"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

// RepairHypothesisKind identifies the repair strategy for multi-hypothesis racing.
type RepairHypothesisKind string

const (
	HypothesisSurgicalFix   RepairHypothesisKind = "surgical_fix"
	HypothesisArchitectural RepairHypothesisKind = "architectural_alternative"
)

// CandidateRepair holds the result of a single hypothesis LLM completion.
type CandidateRepair struct {
	Kind     RepairHypothesisKind
	Prompt   string
	Response *domain.LLMResponse
	Error    error
	Duration time.Duration
}

// generateParallelRepairHypotheses concurrently generates multiple divergent repair hypotheses
// (minimal surgical fix vs. resilient architectural alternative) to race against test gates.
func (o *Orchestrator) generateParallelRepairHypotheses(
	ctx context.Context,
	basePrompt string,
	effectiveTask domain.Task,
	turnTimeout time.Duration,
) []CandidateRepair {
	ctx, span := telemetry.Tracer().Start(ctx, "generateParallelRepairHypotheses",
		trace.WithAttributes(attribute.String("task.id", effectiveTask.ID)))
	defer span.End()

	hypotheses := []struct {
		kind     RepairHypothesisKind
		guidance string
	}{
		{
			kind: HypothesisSurgicalFix,
			guidance: "\n\n### 🔬 REPAIR STRATEGY: MINIMAL SURGICAL FIX\n" +
				"Prioritize minimal, targeted modifications to failing assertions, off-by-one errors, " +
				"missing imports, or exact syntax mismatches. Do NOT rewrite architecture; surgically patch only the failing lines.",
		},
		{
			kind: HypothesisArchitectural,
			guidance: "\n\n### 🏛️ REPAIR STRATEGY: RESILIENT ARCHITECTURAL ALTERNATIVE\n" +
				"Prioritize architectural resilience, defensive fallback error handling, and robust alternative " +
				"implementations to overcome structural design deadlocks or rigid coupling.",
		},
	}

	candidates := make([]CandidateRepair, len(hypotheses))
	var wg sync.WaitGroup
	wg.Add(len(hypotheses))

	for i, hyp := range hypotheses {
		go func(idx int, kind RepairHypothesisKind, guidance string) {
			defer wg.Done()
			start := time.Now()
			promptBody := basePrompt + guidance

			llmCtx, cancel := context.WithTimeout(ctx, turnTimeout)
			defer cancel()
			llmCtx = domain.WithRoleContext(llmCtx, string(domain.AgentRoleFallback))
			llmCtx = context.WithValue(llmCtx, AgentRoleKey, "fallback")
			contractLen := len(prompts.Contract(prompts.AgentFallback))
			llmCtx = domain.WithUncompactableTail(llmCtx, contractLen)
			if len(promptBody) > contractLen {
				llmCtx = domain.WithCacheablePrefix(llmCtx, len(promptBody)-contractLen)
			}

			resp, err := o.llmClient.Complete(llmCtx, promptBody)
			o.recordTokenUsage(ctx, promptBody, resp)

			candidates[idx] = CandidateRepair{
				Kind:     kind,
				Prompt:   promptBody,
				Response: resp,
				Error:    err,
				Duration: time.Since(start),
			}
		}(i, hyp.kind, hyp.guidance)
	}

	wg.Wait()
	return candidates
}

// completeFallbackTurn executes a single sequential LLM completion turn for fallback recovery.
func (o *Orchestrator) completeFallbackTurn(
	ctx context.Context,
	promptBody string,
	turnTimeout time.Duration,
) (*domain.LLMResponse, error) {
	llmCtx, cancel := context.WithTimeout(ctx, turnTimeout)
	defer cancel()
	llmCtx = domain.WithRoleContext(llmCtx, string(domain.AgentRoleFallback))
	llmCtx = context.WithValue(llmCtx, AgentRoleKey, "fallback")
	contractLen := len(prompts.Contract(prompts.AgentFallback))
	llmCtx = domain.WithUncompactableTail(llmCtx, contractLen)
	if len(promptBody) > contractLen {
		llmCtx = domain.WithCacheablePrefix(llmCtx, len(promptBody)-contractLen)
	}

	resp, err := o.llmClient.Complete(llmCtx, promptBody)
	o.recordTokenUsage(ctx, promptBody, resp)
	return resp, err
}

// raceAndApplyCandidateHypotheses applies and evaluates candidate repair hypotheses against
// the workspace test suite, rolling back failed attempts and keeping the winning patch.
func (o *Orchestrator) raceAndApplyCandidateHypotheses(
	ctx context.Context,
	candidates []CandidateRepair,
	effectiveTask domain.Task,
	taskState *domain.State,
	taskGit *GitClient,
	turn, maxTurns int,
	turnTimeout time.Duration,
) (passed bool, currentLog string, hasMutatingAction bool) {
	for _, cand := range candidates {
		if cand.Error != nil || cand.Response == nil || len(cand.Response.Actions) == 0 {
			continue
		}

		fmt.Printf("⚡ [Parallel Fallback Racing] Evaluating candidate hypothesis %q (generated in %v)...\n",
			cand.Kind, cand.Duration)

		// 1. Apply candidate actions
		appliedMutating := false
		for _, action := range cand.Response.Actions {
			if action.Tool == "noop" {
				continue
			}
			if IsMutatingTool(action.Tool) {
				appliedMutating = true
				hasMutatingAction = true
			}
			if o.registry != nil {
				tool, ok := o.registry.Get(action.Tool)
				if ok {
					_, execErr := tool.Execute(ctx, taskState, action.Args)
					if execErr != nil {
						fmt.Fprintf(os.Stderr, "⚠ [Fallback Tool Failed (%s)] %s: %v\n", cand.Kind, action.Tool, execErr)
					}
				}
			}
		}

		// 2. Stage and commit candidate patch if mutating
		if appliedMutating && taskGit != nil {
			_ = o.stageAndCommit(ctx, taskGit, effectiveTask.ID,
				"fix(fallback): candidate %s for task %s [turn %d/%d]", cand.Kind, effectiveTask.Title, turn, maxTurns)
		}

		// 3. Evaluate tests against candidate patch
		if o.evaluator != nil {
			evalPassed, evalLog, _ := o.evaluator.ValidateTask(ctx, taskState, effectiveTask)

			// E2E Verification Gate
			if evalPassed && o.evaluator.Runner != nil && taskState != nil && taskState.ProjectPath != "" && o.evaluator.shouldValidateE2E(taskState, effectiveTask) {
				evalPassed, evalLog = o.runFallbackE2EGate(ctx, taskState, effectiveTask, turnTimeout, evalPassed, evalLog)
			}

			// Anti-Stub Quality Gate
			if evalPassed && taskState != nil && taskState.ProjectPath != "" {
				evalPassed, evalLog = o.runFallbackAntiStubGate(taskState.ProjectPath, effectiveTask, evalPassed, evalLog)
			}

			if evalPassed {
				fmt.Printf("🏆 [Parallel Fallback Winner] Candidate hypothesis %q PASSED all quality gates!\n", cand.Kind)
				return true, evalLog, true
			}

			currentLog = evalLog
			fmt.Printf("⚠️  [Parallel Fallback] Candidate hypothesis %q failed tests. Rolling back workspace...\n", cand.Kind)

			// Rollback workspace to try next candidate
			if taskGit != nil {
				_, _ = taskGit.Run(ctx, false, "reset", "--hard", "HEAD~1")
				_, _ = taskGit.Run(ctx, false, "clean", "-fd")
			}
		}
	}

	return false, currentLog, hasMutatingAction
}

func (o *Orchestrator) runFallbackE2EGate(
	ctx context.Context,
	taskState *domain.State,
	effectiveTask domain.Task,
	turnTimeout time.Duration,
	passed bool,
	logMsg string,
) (bool, string) {
	e2eMode := o.cfg.E2E.Mode
	e2eCmd := o.cfg.E2E.Command
	detectedE2E := DetectE2ECommand(taskState.ProjectPath, e2eMode, e2eCmd)
	if detectedE2E == "" {
		return passed, logMsg
	}

	e2eTimeout := 5 * time.Minute
	if turnTimeout > 0 {
		e2eTimeout = turnTimeout
	}
	e2eCtx, e2eCancel := context.WithTimeout(ctx, e2eTimeout)
	defer e2eCancel()

	e2eOut, e2eErr := o.evaluator.Runner.RunCommand(e2eCtx, taskState.ProjectPath, detectedE2E, "")
	if e2eErr != nil {
		if isCommandToolMissing(o.evaluator.Runner, detectedE2E, e2eOut+" "+e2eErr.Error()) {
			return passed, logMsg
		}
		if !isE2EFailureInScope(taskState, effectiveTask, e2eOut+"\n"+e2eErr.Error()) {
			return passed, logMsg
		}
		return false, fmt.Sprintf("Unit tests passed, but E2E verification failed (%s):\n%s\n%v", detectedE2E, e2eOut, e2eErr)
	}
	return passed, logMsg
}

func (o *Orchestrator) runFallbackAntiStubGate(
	projectPath string,
	effectiveTask domain.Task,
	passed bool,
	logMsg string,
) (bool, string) {
	antiStub := NewAntiStubValidator()
	violations, _ := antiStub.ValidateWorkspace(projectPath, effectiveTask.TargetFiles)
	if len(violations) > 0 {
		var sb strings.Builder
		fmt.Fprintf(&sb, "Anti-stub / anti-gaming validation failed with %d violation(s):\n", len(violations))
		for _, v := range violations {
			fmt.Fprintf(&sb, "- %s:%d: [%s] %s\n", v.Path, v.Line, v.Rule, v.Snippet)
		}
		return false, sb.String()
	}
	return passed, logMsg
}
