package cli

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/diegojromerolopez/noctifab/pkg/domain"
	"github.com/diegojromerolopez/noctifab/pkg/infrastructure/config"
	"github.com/diegojromerolopez/noctifab/pkg/services"
)

// SovereignRescueOptions parameters for autonomous whole-project sovereign recovery.
type SovereignRescueOptions struct {
	TargetDir          string
	Cfg                *config.Config
	Repo               domain.StateRepository
	GitClient          *services.GitClient
	StoryFiles         []string
	FailedStories      []string
	AcceptanceGaps     []string
	PostValidationFunc func(ctx context.Context, state *domain.State) (bool, string)
	LLMClient          domain.LLMClient
	ToolRegistry       *services.ToolRegistry
	Validator          *services.TestValidator
	SandboxRunner      services.Sandbox
	MaxTurns           int
	TurnTimeout        time.Duration
	ToolchainStrategy  string
}

// DispatchSovereignRescue prepares execution parameters, ensures a viable context runway
// (decoupling from expired iteration loop timeouts), and triggers sovereign project rescue.
func DispatchSovereignRescue(ctx context.Context, opts SovereignRescueOptions) error {
	if opts.Cfg != nil {
		rescueCfg := opts.Cfg.GetSovereignRescue()
		if !rescueCfg.IsEnabled() {
			fmt.Fprintf(os.Stderr, "ℹ [Sovereign Rescue] Sovereign rescue is disabled in configuration.\n")
			return errors.New("sovereign rescue is disabled in configuration")
		}
		if opts.MaxTurns <= 0 {
			opts.MaxTurns = rescueCfg.GetMaxTurns()
		}
		if opts.TurnTimeout <= 0 {
			opts.TurnTimeout = rescueCfg.GetTimeout()
		}
		if opts.ToolchainStrategy == "" {
			opts.ToolchainStrategy = rescueCfg.GetMissingToolchainStrategy()
		}
	} else {
		if opts.MaxTurns <= 0 {
			opts.MaxTurns = 10
		}
		if opts.TurnTimeout <= 0 {
			opts.TurnTimeout = 5 * time.Minute
		}
		if opts.ToolchainStrategy == "" {
			opts.ToolchainStrategy = "auto"
		}
	}

	rescueCtx := ctx
	// Emergency Context Runway:
	// If the parent loop context already timed out or has negligible runway remaining (< 2m),
	// allocate a dedicated sovereign rescue timeout from background so that emergency unblocking
	// is not paralyzed by prior loop budget exhaustion.
	needsFreshRunway := false
	if ctx.Err() != nil {
		if errors.Is(ctx.Err(), context.DeadlineExceeded) || errors.Is(ctx.Err(), context.Canceled) {
			needsFreshRunway = true
		}
	} else if dl, ok := ctx.Deadline(); ok && time.Until(dl) < 2*time.Minute {
		needsFreshRunway = true
	}

	if needsFreshRunway {
		totalRunway := opts.TurnTimeout * time.Duration(opts.MaxTurns)
		if totalRunway < 10*time.Minute {
			totalRunway = 10 * time.Minute
		}
		fmt.Fprintf(os.Stderr, "ℹ [Sovereign Rescue] Loop context deadline exhausted; engaging dedicated %v sovereign rescue runway.\n", totalRunway)
		var cancel context.CancelFunc
		rescueCtx, cancel = context.WithTimeout(context.Background(), totalRunway)
		defer cancel()
	}

	return runSovereignProjectRescue(rescueCtx, opts)
}

// runSovereignProjectRescue executes an emergency sovereign single-agent takeover
// of the entire workspace when specialized multi-agent stories fail or stall.
func runSovereignProjectRescue(ctx context.Context, opts SovereignRescueOptions) error {
	if opts.LLMClient == nil || opts.ToolRegistry == nil || opts.Validator == nil {
		return fmt.Errorf("cannot execute sovereign rescue: missing required LLM, registry, or validator dependencies")
	}

	maxTurns := opts.MaxTurns
	if maxTurns <= 0 {
		maxTurns = 10
	}

	turnTimeout := opts.TurnTimeout
	if turnTimeout <= 0 {
		turnTimeout = 3 * time.Minute
	}

	fmt.Fprintf(os.Stderr, "\n%s\n", strings.Repeat("=", 80))
	fmt.Fprintf(os.Stderr, "🚨 [SOVEREIGN RESCUE TAKEOVER ACTIVATED]\n")
	fmt.Fprintf(os.Stderr, "Standard multi-agent pipeline concluded with %d failed/incomplete story(ies).\n", len(opts.FailedStories))
	fmt.Fprintf(os.Stderr, "Dissolving all architectural boundaries, story divisions, and worker roles.\n")
	fmt.Fprintf(os.Stderr, "Engaging direct Sovereign LLM Agent to complete and verify the project...\n")
	fmt.Fprintf(os.Stderr, "%s\n\n", strings.Repeat("=", 80))

	// 1. Purge any stale Git or worktree locks
	if opts.GitClient != nil {
		opts.GitClient.CleanStaleLocks(ctx)
	}

	// 2. Read SPEC.md ground-truth requirements
	specBytes, _ := os.ReadFile(filepath.Join(opts.TargetDir, "SPEC.md"))
	specContent := string(specBytes)
	if strings.TrimSpace(specContent) == "" {
		specContent = "Implement a working application based on the repository user stories."
	}

	// 3. Obtain initial failure diagnostics
	dummyTask := domain.Task{
		ID:          "sovereign-project-rescue",
		Title:       "Whole-Project Sovereign Rescue",
		Description: "Direct sovereign unblocking and completion of the full codebase",
	}

	var state *domain.State
	if opts.Repo != nil {
		state, _ = opts.Repo.Load(ctx)
	}
	if state == nil {
		state = &domain.State{
			ProjectPath: opts.TargetDir,
		}
	}

	_, lastFailureLog, _ := opts.Validator.ValidateTask(ctx, state, dummyTask)
	diagnostics := CollectSovereignDiagnostics(opts.TargetDir, state, opts.FailedStories, opts.AcceptanceGaps, lastFailureLog)

	resolvedStrategy := ResolveToolchainStrategy(ctx, opts.ToolchainStrategy, nil)
	bestCommit := captureSovereignBaselineCommit(ctx, opts.GitClient)

	// 4. Multi-Turn Sovereign Rescue Loop
	for turn := 1; turn <= maxTurns; turn++ {
		fmt.Printf("🔧 [Sovereign Rescue] Turn %d/%d: Prompting direct sovereign LLM agent...\n", turn, maxTurns)

		detectedMissing := DetectMissingToolchainIndicator(diagnostics)
		prompt := buildSovereignRescuePrompt(specContent, opts.FailedStories, opts.AcceptanceGaps, diagnostics, turn, maxTurns, resolvedStrategy, detectedMissing)

		turnCtx, cancel := context.WithTimeout(ctx, turnTimeout)
		turnCtx = domain.WithRoleContext(turnCtx, string(domain.AgentRoleFallback))
		turnCtx = context.WithValue(turnCtx, services.AgentRoleKey, "fallback")
		resp, err := opts.LLMClient.Complete(turnCtx, prompt)
		cancel()

		if err != nil {
			fmt.Fprintf(os.Stderr, "⚠ [Sovereign Rescue] Turn %d LLM invocation error: %v\n", turn, err)
			continue
		}

		var toolErrors []string
		actionsExecuted := 0
		if resp != nil && len(resp.Actions) > 0 {
			for _, action := range resp.Actions {
				if action.Tool == "noop" {
					continue
				}
				tool, ok := opts.ToolRegistry.Get(action.Tool)
				if ok {
					out, execErr := tool.Execute(ctx, state, action.Args)
					if execErr != nil {
						errMsg := fmt.Sprintf("tool %s failed: %v", action.Tool, execErr)
						fmt.Fprintf(os.Stderr, "⚠ [Sovereign Tool Failed] %s\n", errMsg)
						toolErrors = append(toolErrors, errMsg)
					} else {
						actionsExecuted++
						fmt.Printf("   ✓ Action %s succeeded\n", action.Tool)
					}
					if state != nil {
						actionRecord := domain.Action{
							Timestamp: time.Now().UTC(),
							Tool:      action.Tool,
							Args:      action.Args,
							Reasoning: fmt.Sprintf("[Sovereign Rescue Turn %d/%d] %s", turn, maxTurns, resp.Reasoning),
							Result:    out,
							Success:   execErr == nil,
						}
						if execErr != nil {
							actionRecord.Result = execErr.Error()
						}
						state.LastActions = append(state.LastActions, actionRecord)
					}
				} else {
					errMsg := fmt.Sprintf("tool %q is not registered in ToolRegistry", action.Tool)
					fmt.Fprintf(os.Stderr, "⚠ [Sovereign Tool Failed] %s\n", errMsg)
					toolErrors = append(toolErrors, errMsg)
				}
			}
			fmt.Printf("🔧 [Sovereign Rescue] Turn %d: executed %d workspace actions\n", turn, actionsExecuted)

			// Stage and commit rescued code
			if opts.GitClient != nil {
				_, _ = opts.GitClient.Run(ctx, true, "add", "-A")
				_, _ = opts.GitClient.Run(ctx, true, "commit", "-m",
					fmt.Sprintf("fix(sovereign-rescue): direct unblock turn %d/%d", turn, maxTurns))
			}
		} else {
			toolErrors = append(toolErrors, "response contained 0 actionable tool invocations; you must invoke write_file, write_files, edit_file, or install_package")
		}

		// Log and persist corrective step in database
		correctiveSummary := resp.Reasoning
		if correctiveSummary == "" {
			correctiveSummary = fmt.Sprintf("Turn %d executed %d actions", turn, actionsExecuted)
		}
		fmt.Printf("📝 [Sovereign Rescue] Turn %d/%d Corrective Step: %s\n", turn, maxTurns, correctiveSummary)

		if opts.Repo != nil {
			stepAction := domain.Action{
				Timestamp: time.Now().UTC(),
				Tool:      "sovereign_rescue_step",
				Reasoning: fmt.Sprintf("[Turn %d/%d] %s", turn, maxTurns, correctiveSummary),
				Success:   len(toolErrors) == 0 && actionsExecuted > 0,
				Result:    fmt.Sprintf("actions=%d, tool_errors=%d", actionsExecuted, len(toolErrors)),
			}
			if len(toolErrors) > 0 {
				stepAction.Result = fmt.Sprintf("actions=%d, errors: %s", actionsExecuted, strings.Join(toolErrors, "; "))
			}
			state.LastActions = append(state.LastActions, stepAction)
			_ = opts.Repo.Save(ctx, state)
		}

		// Re-evaluate verification gate
		unitPassed, newLog, _ := opts.Validator.ValidateTask(ctx, state, dummyTask)
		passed := unitPassed
		if passed && opts.SandboxRunner != nil {
			var e2eErrLog string
			passed, e2eErrLog = verifySovereignE2E(ctx, opts.TargetDir, opts.SandboxRunner, opts.Cfg)
			if !passed {
				newLog = fmt.Sprintf("Unit tests passed, but E2E verification gate failed:\n%s", e2eErrLog)
			}
		}
		if passed {
			var antiStubErr string
			passed, antiStubErr = auditSovereignAntiStub(opts.TargetDir)
			if !passed {
				newLog = fmt.Sprintf("Unit tests and E2E passed, but Anti-Stub/Tautology Quality Gate failed:\n%s", antiStubErr)
			}
		}
		if passed && opts.PostValidationFunc != nil {
			var postErr string
			passed, postErr = opts.PostValidationFunc(ctx, state)
			if !passed {
				newLog = fmt.Sprintf("Unit tests, E2E, and Anti-Stub passed, but Acceptance Audit failed:\n%s", postErr)
			}
		}

		if passed {
			fmt.Printf("✨ [Sovereign Rescue] Turn %d/%d succeeded! All project gates, builds, and test assertions passed.\n", turn, maxTurns)

			// Mark stories and state as completed in repo
			if opts.Repo != nil {
				_ = updateRescueSuccessState(ctx, opts.Repo, opts.StoryFiles, opts.FailedStories, opts.AcceptanceGaps)
			}
			return nil
		}

		if unitPassed {
			if curCommit := captureSovereignBaselineCommit(ctx, opts.GitClient); curCommit != "" {
				bestCommit = curCommit
			}
		} else if bestCommit != "" && len(toolErrors) > 0 {
			shortCommit := bestCommit
			if len(shortCommit) > 8 {
				shortCommit = shortCommit[:8]
			}
			fmt.Fprintf(os.Stderr, "⚠ [Sovereign Rescue] Turn %d broke unit tests with tool errors. Reverting partial mutations to baseline %s...\n", turn, shortCommit)
			_ = rollbackSovereignTurn(ctx, opts.GitClient, bestCommit)
		}

		lastFailureLog = newLog
		if len(toolErrors) > 0 {
			lastFailureLog = fmt.Sprintf("WORKSPACE TOOL EXECUTION ERRORS IN TURN %d:\n- %s\n\nVALIDATION OUTPUT:\n%s", turn, strings.Join(toolErrors, "\n- "), newLog)
		}
		diagnostics = CollectSovereignDiagnostics(opts.TargetDir, state, opts.FailedStories, opts.AcceptanceGaps, lastFailureLog)
		fmt.Printf("⚠️ [Sovereign Rescue] Turn %d verification failed. Feeding diagnostics into turn %d...\n", turn, turn+1)
	}

	if bestCommit != "" && opts.GitClient != nil {
		if curCommit := captureSovereignBaselineCommit(ctx, opts.GitClient); curCommit != bestCommit {
			unitPassed, _, _ := opts.Validator.ValidateTask(ctx, state, dummyTask)
			if !unitPassed {
				shortCommit := bestCommit
				if len(shortCommit) > 8 {
					shortCommit = shortCommit[:8]
				}
				fmt.Fprintf(os.Stderr, "ℹ [Sovereign Rescue] Restoring last verified green baseline commit %s at conclusion of rescue loop.\n", shortCommit)
				_ = rollbackSovereignTurn(ctx, opts.GitClient, bestCommit)
			}
		}
	}

	return fmt.Errorf("sovereign rescue exhausted %d turns without passing all validation gates. Last error:\n%s", maxTurns, lastFailureLog)
}

func buildSovereignRescuePrompt(specContent string, failedStories, acceptanceGaps []string, failureLog string, turn, maxTurns int, resolvedStrategy string, detectedMissing bool) string {
	var sb strings.Builder
	sb.WriteString("You are Noctifab's Sovereign Omni-Agent.\n")
	sb.WriteString("The standard multi-agent pipeline encountered an unresolvable bottleneck and could not finish.\n")
	sb.WriteString("All role restrictions, story divisions, and architectural boundaries are DISSOLVED.\n")
	sb.WriteString("You have sovereign, direct authority to inspect, create, and modify ANY file in the workspace.\n\n")

	sb.WriteString("=== GROUND TRUTH SPECIFICATION (SPEC.md) ===\n")
	sb.WriteString(specContent)
	sb.WriteString("\n\n")

	if len(acceptanceGaps) > 0 {
		sb.WriteString("=== WHOLE-PROJECT ACCEPTANCE AUDIT GAPS (REQUIRED REMEDIATION) ===\n")
		sb.WriteString("The project failed the Whole-Project Acceptance Audit with the following concrete specification gaps.\n")
		sb.WriteString("You MUST implement all missing files, commands, schemas, and test scenarios enumerated below:\n")
		for _, gap := range acceptanceGaps {
			sb.WriteString("- ")
			sb.WriteString(gap)
			sb.WriteString("\n")
		}
		sb.WriteString("\n")
	}

	if len(failedStories) > 0 {
		sb.WriteString("=== UNRESOLVED / FAILED ROADMAP STORIES ===\n")
		for _, s := range failedStories {
			sb.WriteString("- ")
			sb.WriteString(s)
			sb.WriteString("\n")
		}
		sb.WriteString("\n")
	}

	fmt.Fprintf(&sb, "=== CURRENT FAILURE DIAGNOSTICS (Turn %d of %d) ===\n", turn, maxTurns)
	sb.WriteString(failureLog)
	sb.WriteString("\n\n")

	if directive := BuildToolchainFallbackDirective(resolvedStrategy, detectedMissing); directive != "" {
		sb.WriteString(directive)
	}

	sb.WriteString("=== DIRECT MANDATE ===\n")
	sb.WriteString("1. Directly write or modify all source code, headers, and configuration files needed to fulfill SPEC.md.\n")
	sb.WriteString("2. Author genuine unit tests under tests/ directory with real assertions (0 tests or empty tests will FAIL).\n")
	sb.WriteString("3. Ensure the project contains a Makefile with three standard recipes:\n")
	sb.WriteString("   - build: compiles all source binaries cleanly without errors.\n")
	sb.WriteString("   - test: executes unit tests and exits with code 1 if 0 tests are found.\n")
	sb.WriteString("   - e2e: executes end-to-end black-box verification of compiled binaries (e2e is strictly validated; if e2e fails or is missing, the turn is rejected).\n")
	sb.WriteString("4. Fix all compilation errors, missing translation units, or syntax issues reported above.\n")
	sb.WriteString("5. Strictly avoid placeholder stubs, error-masking shell tricks, and tautological tests (e.g., 'TODO: implement', 'pass', empty main functions, '|| true', 'assert True'). The anti-stub validator and non-tautological test auditor strictly reject them.\n")
	sb.WriteString("6. You are an autonomous headless dark-factory repair engine. You DO NOT interact with a human user and do not require chat-based interactive tools. The host orchestrator reads your JSON response and executes all file write/edit actions directly onto the workspace filesystem. You MUST provide all source code, tests, Dockerfiles, and configurations directly inside the 'actions' array using 'write_file', 'write_files', 'edit_file', or 'install_package'. Do NOT emit apologies, refusals, or claims that you lack workspace write tools.\n\n")

	sb.WriteString("=== REQUIRED RESPONSE FORMAT ===\n")
	sb.WriteString("You MUST respond ONLY with a single JSON object matching this schema (do NOT wrap in markdown code fences):\n")
	sb.WriteString("{\n")
	sb.WriteString("  \"reasoning\": \"Explanation of fixes applied to unblock the project\",\n")
	sb.WriteString("  \"actions\": [\n")
	sb.WriteString("    {\n")
	sb.WriteString("      \"tool\": \"write_file\",\n")
	sb.WriteString("      \"args\": {\n")
	sb.WriteString("        \"path\": \"src/main.py\",\n")
	sb.WriteString("        \"content\": \"...\"\n")
	sb.WriteString("      }\n")
	sb.WriteString("    },\n")
	sb.WriteString("    {\n")
	sb.WriteString("      \"tool\": \"write_files\",\n")
	sb.WriteString("      \"args\": {\n")
	sb.WriteString("        \"files\": [\n")
	sb.WriteString("          {\"path\": \"tests/test_app.py\", \"content\": \"...\"},\n")
	sb.WriteString("          {\"path\": \"Makefile\", \"content\": \"...\"}\n")
	sb.WriteString("        ]\n")
	sb.WriteString("      }\n")
	sb.WriteString("    }\n")
	sb.WriteString("  ]\n")
	sb.WriteString("}\n")

	return sb.String()
}
