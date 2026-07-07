package services

import (
	"context"
	"fmt"
	"os"

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
	baseBranch := state.Metadata.BaseBranch
	if baseBranch == "" {
		baseBranch = "main"
	}

	anySuccess := false
	for _, t := range state.Tasks {
		if t.Status == domain.TaskSuccess {
			anySuccess = true
			break
		}
	}

	if !anySuccess {
		fmt.Printf("⚠ Story %s: no tasks completed successfully — skipping PR creation.\n", state.Metadata.FeatureName)
		return nil
	}

	// Ensure integration branch exists locally before bumping
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

	// Bump version and update CHANGELOG.md
	nextVersion, bumpErr := BumpVersion(state.ProjectPath, state.Tasks)
	if bumpErr == nil {
		_ = UpdateChangelog(state.ProjectPath, nextVersion, state.Tasks)
		_, _ = o.git.Run(ctx, true, "add", "VERSION", "CHANGELOG.md")
		_, _ = o.git.Run(ctx, true, "commit", "-m",
			fmt.Sprintf("chore(release): bump version to %s for story %s", nextVersion, state.Metadata.FeatureName))
	}

	// Push integration branch
	_, pushErr := o.git.Run(ctx, true, "push", "origin", integrationBranch)
	if pushErr != nil {
		fmt.Fprintf(os.Stderr, "⚠ Failed to push integration branch %s: %v\n", integrationBranch, pushErr)
		return fmt.Errorf("push failed for %s: %w", integrationBranch, pushErr)
	}

	// Create pull request
	if o.cfg.AutoCreatePR {
		prTitle := fmt.Sprintf("feat: %s", state.Metadata.FeatureName)
		prBody := buildPRBody(state)
		_, prErr := o.vcsClient.CreatePullRequest(ctx, prTitle, prBody, integrationBranch, baseBranch)
		if prErr != nil {
			fmt.Fprintf(os.Stderr, "⚠ Failed to create Pull Request for story %s: %v\n", state.Metadata.FeatureName, prErr)
			return fmt.Errorf("PR creation failed: %w", prErr)
		}
	} else {
		fmt.Printf("Skipping PR creation for story %s (auto_create is false)\n", state.Metadata.FeatureName)
	}

	// Prominent completion feedback to operator
	printStoryCompletionBanner(state)
	return nil
}

// buildPRBody assembles a markdown pull request description from the completed state.
func buildPRBody(state *domain.State) string {
	body := fmt.Sprintf("## Automated Pull Request — %s\n\n", state.Metadata.FeatureName)
	body += fmt.Sprintf("**Source:** %s\n", state.Metadata.InputPath)
	body += fmt.Sprintf("**Branch:** `%s` → `%s`\n\n", state.Metadata.IntegrationBranch, state.Metadata.BaseBranch)
	body += "### Tasks\n\n"
	for _, t := range state.Tasks {
		icon := "✅"
		if t.Status == domain.TaskFailed {
			icon = "❌"
		}
		body += fmt.Sprintf("- %s **%s** — %s\n", icon, t.Title, string(t.Status))
	}
	return body
}

// printStoryCompletionBanner prints a clearly visible terminal banner when a user story finishes.
func printStoryCompletionBanner(state *domain.State) {
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
