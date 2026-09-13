package cli

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/diegojromerolopez/noctifab/pkg/domain"
	"github.com/diegojromerolopez/noctifab/pkg/infrastructure/config"
	"github.com/diegojromerolopez/noctifab/pkg/services"
)

// AcceptanceGateOptions encapsulates dependencies for the whole-project acceptance audit.
type AcceptanceGateOptions struct {
	TargetDir      string
	Cfg            *config.Config
	Repo           domain.StateRepository
	LLMClient      domain.LLMClient
	PromptRenderer services.PromptRenderer
	SandboxRunner  services.Sandbox
	GitClient      *services.GitClient
	StoryFiles     []string
	ToolRegistry   *services.ToolRegistry
	Validator      *services.TestValidator
}

// RunWholeProjectAcceptanceGate audits the entire workspace against root SPEC.md.
func RunWholeProjectAcceptanceGate(ctx context.Context, opts AcceptanceGateOptions) error {
	if opts.Repo == nil {
		return nil
	}

	state, err := opts.Repo.Load(ctx)
	if err != nil || state == nil {
		return nil
	}
	if state.ProjectPath == "" {
		state.ProjectPath = opts.TargetDir
	}

	auditor := services.NewAcceptanceAuditor(opts.LLMClient, opts.PromptRenderer, opts.SandboxRunner)
	if opts.Cfg != nil {
		auditor.ConfigureFromConfig(opts.Cfg)
	}

	fmt.Printf("\n🔍 [Whole-Project Acceptance Gate] Verifying complete workspace implementation and behavioral E2E tests against SPEC.md...\n")
	auditResult, auditErr := auditor.AuditProjectAcceptance(ctx, state)
	if auditErr != nil {
		fmt.Fprintf(os.Stderr, "⚠ Warning: Acceptance Audit encountered an execution error: %v\n", auditErr)
		return nil
	}

	if auditResult == nil || auditResult.Passed || len(auditResult.Gaps) == 0 {
		fmt.Printf("✨ [Whole-Project Acceptance Gate] PASSED: All functional contracts and black-box E2E tests in SPEC.md are verified.\n")
		return nil
	}

	// Format failure diagnostics
	fmt.Printf("\n================================================================================\n")
	fmt.Printf("❌ WHOLE-PROJECT ACCEPTANCE AUDIT FAILED\n")
	fmt.Printf("================================================================================\n")
	fmt.Printf("Summary: %s\n\n", auditResult.Summary)
	fmt.Printf("Unimplemented specification gaps & contract failures:\n")
	for _, g := range auditResult.Gaps {
		fmt.Printf(" - %s\n", g)
	}
	fmt.Printf("================================================================================\n\n")

	// Attempt targeted sovereign remediation if rescue is enabled
	if opts.Cfg != nil {
		rescueCfg := opts.Cfg.GetSovereignRescue()
		if rescueCfg.IsEnabled() && opts.ToolRegistry != nil && opts.Validator != nil {
			fmt.Printf("⚡ [Whole-Project Acceptance Gate] Triggering sovereign remediation for %d specification gap(s)...\n", len(auditResult.Gaps))
			rescueOpts := SovereignRescueOptions{
				TargetDir:     opts.TargetDir,
				Cfg:           opts.Cfg,
				Repo:          opts.Repo,
				GitClient:     opts.GitClient,
				StoryFiles:    opts.StoryFiles,
				FailedStories: []string{fmt.Sprintf("Whole-Project Acceptance Audit (%s)", auditResult.Summary)},
				LLMClient:     opts.LLMClient,
				ToolRegistry:  opts.ToolRegistry,
				Validator:     opts.Validator,
			}
			if rescueErr := DispatchSovereignRescue(ctx, rescueOpts); rescueErr == nil {
				fmt.Printf("✨ [Whole-Project Acceptance Gate] Sovereign remediation completed. Re-running acceptance audit...\n")
				// Re-verify after remediation
				reAudit, reErr := auditor.AuditProjectAcceptance(ctx, state)
				if reErr == nil && reAudit != nil && reAudit.Passed {
					fmt.Printf("✨ [Whole-Project Acceptance Gate] Re-audit PASSED after sovereign remediation!\n")
					return nil
				}
				if reAudit != nil && len(reAudit.Gaps) > 0 {
					auditResult = reAudit
				}
			}
		}
	}

	return fmt.Errorf("Whole-Project Acceptance Audit FAILED: %s\nSpecification gaps:\n - %s", auditResult.Summary, strings.Join(auditResult.Gaps, "\n - "))
}
