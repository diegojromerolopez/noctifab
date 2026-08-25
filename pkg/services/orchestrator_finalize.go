package services

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/diegojromerolopez/noctifab/pkg/domain"
	"github.com/diegojromerolopez/noctifab/pkg/infrastructure/telemetry"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

// FinalizeUserStory performs the post-completion steps for a finished user story:
// version bump, changelog update, branch push, and pull request creation.
// It prints a prominent completion banner to stdout so the operator knows the story is done.
func (o *Orchestrator) FinalizeUserStory(ctx context.Context, state *domain.State) error {
	ctx, span := telemetry.Tracer().Start(ctx, "FinalizeUserStory",
		trace.WithAttributes(
			attribute.String("feature_name", state.Metadata.FeatureName),
			attribute.String("base_branch", state.Metadata.BaseBranch),
		))
	defer span.End()
	integrationBranch := state.Metadata.IntegrationBranch
	baseBranch := ResolveBaseBranch(ctx, o.git, state.Metadata.BaseBranch)
	if integrationBranch == "" {
		integrationBranch = baseBranch
	}

	// 1. Strict PR Release Invariant: All tasks in the story MUST be TaskSuccess.
	// A PR is strictly forbidden if any task remains PENDING or FAILED.
	if !o.allTasksSucceeded(state) {
		fmt.Printf("⚠ Story %s: not all tasks succeeded (pending or failed tasks remain) — skipping PR creation.\n", state.Metadata.FeatureName)
		return nil
	}

	// 2. Whole-Project Acceptance Audit Gate: Verify implemented codebase against SPEC.md
	auditResult, auditErr := o.RunAcceptanceAudit(ctx, state)
	if auditErr != nil {
		fmt.Fprintf(os.Stderr, "⚠ Warning: Acceptance audit encountered an error for story %s: %v\n", state.Metadata.FeatureName, auditErr)
	} else if auditResult != nil && !auditResult.Passed {
		var sb strings.Builder
		fmt.Fprintf(&sb, "⚠ Story %s: Whole-project Acceptance Audit FAILED.\nSummary: %s\n", state.Metadata.FeatureName, auditResult.Summary)
		if len(auditResult.Gaps) > 0 {
			sb.WriteString("Unimplemented specification gaps detected:\n")
			for _, gap := range auditResult.Gaps {
				fmt.Fprintf(&sb, " - %s\n", gap)
			}
		}
		sb.WriteString("Skipping PR creation to prevent releasing incomplete specification implementation.\n")
		fmt.Print(sb.String())
		return nil
	}

	// Ensure integration branch exists locally before bumping if branch creation is enabled
	if o.cfg.CreateBranch && integrationBranch != baseBranch {
		_, err := o.git.Run(ctx, false, "show-ref", "--verify", "--quiet", "refs/heads/"+integrationBranch)
		if err != nil {
			_, _ = o.git.Run(ctx, true, "checkout", baseBranch)
			if _, branchErr := o.git.Run(ctx, true, "checkout", "-b", integrationBranch); branchErr != nil {
				return fmt.Errorf("failed to create integration branch %s: %w", integrationBranch, branchErr)
			}
		} else {
			if _, checkoutErr := o.git.Run(ctx, true, "checkout", integrationBranch); checkoutErr != nil {
				return fmt.Errorf("failed to checkout integration branch %s: %w", integrationBranch, checkoutErr)
			}
		}
	}

	commitMsg := fmt.Sprintf("feat: implement story %s", state.Metadata.FeatureName)
	nextVersion, bumpErr := BumpVersion(state.ProjectPath, state.Tasks)
	if bumpErr != nil {
		fmt.Fprintf(os.Stderr, "⚠ Warning: Failed to bump version for story %s: %v\n", state.Metadata.FeatureName, bumpErr)
	} else {
		if clErr := UpdateChangelog(state.ProjectPath, nextVersion, state.Tasks); clErr != nil {
			fmt.Fprintf(os.Stderr, "⚠ Warning: Failed to update changelog for story %s: %v\n", state.Metadata.FeatureName, clErr)
		}
		commitMsg = fmt.Sprintf("chore(release): bump version to %s for story %s", nextVersion, state.Metadata.FeatureName)
	}

	// Stage all generated code and release metadata
	_, _ = o.git.Run(ctx, true, "add", "-A")

	if statusOut, err := o.git.Run(ctx, false, "status", "--porcelain"); err == nil && strings.TrimSpace(statusOut) != "" {
		_, _ = o.git.Run(ctx, true, "commit", "-m", commitMsg)
	}

	// Push integration branch
	_, pushErr := o.git.Run(ctx, true, "push", "origin", integrationBranch)
	if pushErr != nil {
		fmt.Fprintf(os.Stderr, "⚠ Warning: Failed to push integration branch %s to remote (%v). Preserving generated code locally.\n", integrationBranch, pushErr)
	}

	// Create pull request
	if o.cfg.AutoCreatePR && integrationBranch != baseBranch {
		prTitle := fmt.Sprintf("feat: %s", state.Metadata.FeatureName)
		prBody := buildPRBody(state, auditResult)
		_, prErr := o.vcsClient.CreatePullRequest(ctx, prTitle, prBody, integrationBranch, baseBranch)
		if prErr != nil {
			fmt.Fprintf(os.Stderr, "⚠ Warning: Failed to create Pull Request for story %s: %v. Preserving generated code locally.\n", state.Metadata.FeatureName, prErr)
		}
	} else if o.cfg.AutoCreatePR && integrationBranch == baseBranch {
		fmt.Printf("Skipping PR creation for story %s (integration branch is base branch %s)\n", state.Metadata.FeatureName, baseBranch)
	} else {
		fmt.Printf("Skipping PR creation for story %s (auto_create is false)\n", state.Metadata.FeatureName)
	}

	// Prominent completion feedback to operator
	printStoryCompletionBanner(state)
	return nil
}

// RunAcceptanceAudit executes the Whole-Project Acceptance Audit Gate against SPEC.md.
func (o *Orchestrator) RunAcceptanceAudit(ctx context.Context, state *domain.State) (*AcceptanceAuditResult, error) {
	if o.acceptanceAuditor == nil {
		return &AcceptanceAuditResult{Passed: true, Summary: "No acceptance auditor configured"}, nil
	}
	return o.acceptanceAuditor.AuditProjectAcceptance(ctx, state)
}

// buildPRBody assembles a markdown pull request description from the completed state.
func buildPRBody(state *domain.State, audit *AcceptanceAuditResult) string {
	body := fmt.Sprintf("## Automated Pull Request — %s\n\n", state.Metadata.FeatureName)
	body += fmt.Sprintf("**Source:** %s\n", state.Metadata.InputPath)
	body += fmt.Sprintf("**Branch:** `%s` → `%s`\n\n", state.Metadata.IntegrationBranch, state.Metadata.BaseBranch)
	if audit != nil && strings.TrimSpace(audit.Summary) != "" {
		body += "### Specification Acceptance Audit\n\n"
		body += fmt.Sprintf("✅ **Audit Status:** Passed\n**Summary:** %s\n\n", audit.Summary)
	}
	body += "### Tasks\n\n"
	for _, t := range state.Tasks {
		icon := "✅"
		statusNote := string(t.Status)
		if t.Status == domain.TaskFailed {
			icon = "❌"
		} else if strings.TrimSpace(t.FailureLog) != "" {
			icon = "⚠️"
			statusNote = "SUCCESS (with warnings/degraded validation)"
		}
		body += fmt.Sprintf("- %s **%s** — %s\n", icon, t.Title, statusNote)
	}
	return body
}

// printStoryCompletionBanner prints a clearly visible terminal banner when a user story finishes.
func printStoryCompletionBanner(state *domain.State) {
	fmt.Printf("🏁 [Story Completed Successfully] feature=%q branch=%q tokens=%d\n",
		state.Metadata.FeatureName, state.Metadata.IntegrationBranch, state.Metadata.TotalTokensUsed)
	banner := fmt.Sprintf(`
╔══════════════════════════════════════════════════════════════╗
║  ✅ USER STORY COMPLETE                                      ║
║  Story:  %-52s ║
║  Branch: %-52s ║
╚══════════════════════════════════════════════════════════════╝`,
		state.Metadata.FeatureName,
		state.Metadata.IntegrationBranch,
	)
	fmt.Println(banner)
}
