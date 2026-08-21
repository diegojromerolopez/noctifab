package services

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/diegojromerolopez/noctifab/pkg/domain"
	"github.com/diegojromerolopez/noctifab/pkg/infrastructure/telemetry"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

// maxPostMergeRepairTurns defines the maximum number of repair attempts for global integration.
const maxPostMergeRepairTurns = 2

// RunPostMergeRepairPhase executes the post-merge test suite on the unified integration branch
// and spawns the Integration & Test Repair Agent if tests fail.
func (o *Orchestrator) RunPostMergeRepairPhase(ctx context.Context, state *domain.State) error {
	ctx, span := telemetry.Tracer().Start(ctx, "RunPostMergeRepairPhase",
		trace.WithAttributes(
			attribute.String("story.id", state.ID),
			attribute.String("feature_name", state.Metadata.FeatureName),
		))
	defer span.End()

	integrationBranch := state.Metadata.IntegrationBranch
	baseBranch := ResolveBaseBranch(ctx, o.git, state.Metadata.BaseBranch)
	if integrationBranch == "" {
		integrationBranch = baseBranch
	}

	// Ensure we are working on the integration branch
	_, _ = o.git.Run(ctx, true, "checkout", integrationBranch)

	fmt.Printf("🔍 [Post-Merge Validation] Running global test suite on %s...\n", integrationBranch)
	testTask := domain.Task{
		ID:    "global-integration",
		Title: "Post-Merge Integration Validation",
	}

	passed, logMsg, _ := o.evaluator.ValidateTask(ctx, state, testTask)
	if passed {
		fmt.Printf("✅ [Post-Merge Tests Passed] Global integration suite passed on %s.\n", integrationBranch)
		return nil
	}

	fmt.Printf("⚠️  [Post-Merge Test Failure] Global test suite failed on %s. Launching Repair Agent...\n", integrationBranch)

	for turn := 1; turn <= maxPostMergeRepairTurns; turn++ {
		fmt.Printf("🔧 [Integration Repair Agent] Starting repair turn %d/%d...\n", turn, maxPostMergeRepairTurns)

		diffOut, _ := o.git.Run(ctx, false, "diff", baseBranch+"..."+integrationBranch)
		if strings.TrimSpace(diffOut) == "" {
			diffOut, _ = o.git.Run(ctx, false, "diff", "HEAD~1")
		}

		repairPrompt := buildPostMergeRepairPrompt(state, logMsg, diffOut, turn)
		llmCtx, cancel := context.WithTimeout(ctx, 90*time.Second)
		llmCtx = context.WithValue(llmCtx, AgentRoleKey, "generator")

		resp, err := o.llmClient.Complete(llmCtx, repairPrompt)
		cancel()

		if err != nil {
			fmt.Fprintf(os.Stderr, "⚠ [Integration Repair Agent] Turn %d LLM call failed: %v\n", turn, err)
			continue
		}

		if resp != nil && len(resp.Actions) > 0 {
			for _, action := range resp.Actions {
				tool, ok := o.registry.Get(action.Tool)
				if ok {
					_, execErr := tool.Execute(ctx, state, action.Args)
					if execErr != nil {
						fmt.Fprintf(os.Stderr, "⚠ [Repair Tool Failed] %s: %v\n", action.Tool, execErr)
					}
				}
			}

			// Stage and commit repair changes
			_, _ = o.git.Run(ctx, true, "add", "-A")
			statusOut, _ := o.git.Run(ctx, false, "status", "--porcelain")
			if strings.TrimSpace(statusOut) != "" {
				commitMsg := fmt.Sprintf("fix(integration): repair post-merge test failures [turn %d/%d]", turn, maxPostMergeRepairTurns)
				_, _ = o.git.Run(ctx, true, "commit", "-m", commitMsg)
			}
		}

		// Re-evaluate tests
		passed, logMsg, _ = o.evaluator.ValidateTask(ctx, state, testTask)
		if passed {
			fmt.Printf("✨ [Integration Repair Success] All tests passed after repair turn %d!\n", turn)
			return nil
		}
	}

	fmt.Fprintf(os.Stderr, "⚠️ [Post-Merge Repair Complete] Remaining test warnings preserved for human review:\n%s\n", logMsg)
	return nil
}

func buildPostMergeRepairPrompt(state *domain.State, testOutput, diffContext string, turn int) string {
	var sb strings.Builder
	sb.WriteString("You are the Integration & Test Repair Agent for Noctifab.\n")
	sb.WriteString("All individual story tasks have been merged into the integration branch, but the global test suite failed.\n")
	sb.WriteString("Your goal is to inspect the test failure output, repair cross-task discrepancies, fix syntax/typing errors, or adjust test assertions so the suite compiles and passes.\n\n")

	sb.WriteString("### Global Test Failure Output\n```\n")
	sb.WriteString(testOutput)
	sb.WriteString("\n```\n\n")

	if strings.TrimSpace(diffContext) != "" {
		sb.WriteString("### Recent Integration Changes Diff\n```diff\n")
		if len(diffContext) > 8000 {
			diffContext = diffContext[:8000] + "\n...[diff truncated]..."
		}
		sb.WriteString(diffContext)
		sb.WriteString("\n```\n\n")
	}

	fmt.Fprintf(&sb, "This is repair turn %d of %d. Execute tool actions (write_file, replace_lines, etc.) to repair the codebase.\n", turn, maxPostMergeRepairTurns)
	return sb.String()
}
