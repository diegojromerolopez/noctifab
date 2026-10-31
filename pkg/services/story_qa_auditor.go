package services

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/diegojromerolopez/noctifab/pkg/domain"
	"github.com/diegojromerolopez/noctifab/pkg/infrastructure/config"
	"github.com/diegojromerolopez/noctifab/pkg/infrastructure/telemetry"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

// StoryQAResult encapsulates the QA evaluation of code completeness against a user story.
type StoryQAResult struct {
	Passed          bool     `json:"passed"`
	Summary         string   `json:"summary"`
	MissingFeatures []string `json:"missing_features,omitempty"`
}

// StoryQAAuditor reviews the generated codebase against the active User Story requirements.
type StoryQAAuditor struct {
	llmClient        domain.LLMClient
	runner           Sandbox
	acceptanceRunner *QAAcceptanceRunner
	e2eCmd           string
	e2eMode          string
	defaultTestCmd   string
	allowedCommands  []string
}

// NewStoryQAAuditor creates a StoryQAAuditor instance.
func NewStoryQAAuditor(client domain.LLMClient, runner ...Sandbox) *StoryQAAuditor {
	var r Sandbox
	if len(runner) > 0 {
		r = runner[0]
	}
	return &StoryQAAuditor{
		llmClient:        client,
		runner:           r,
		acceptanceRunner: NewQAAcceptanceRunner(client, r),
	}
}

// ConfigureFromConfig attaches sandbox policies and configured test commands for QA pre-flight validation.
func (a *StoryQAAuditor) ConfigureFromConfig(cfg *config.Config) {
	if cfg != nil {
		a.defaultTestCmd = cfg.Sandbox.TestCommand
		a.allowedCommands = cfg.Sandbox.AllowedCommands
		a.e2eMode = cfg.Sandbox.GetE2EMode()
		if cmd := cfg.Sandbox.GetE2ECommand(); cmd != "" {
			a.e2eCmd = cmd
		}
		if a.acceptanceRunner != nil {
			a.acceptanceRunner.ConfigureFromConfig(cfg)
		}
	}
}

// SetDefaultTestCommand sets the custom default workspace test command.
func (a *StoryQAAuditor) SetDefaultTestCommand(cmd string) {
	a.defaultTestCmd = cmd
}

// SetAllowedCommands sets the whitelisted binaries allowed by the sandbox policy.
func (a *StoryQAAuditor) SetAllowedCommands(cmds []string) {
	a.allowedCommands = cmds
}

// SetRunner attaches a sandbox runner for E2E validation.
func (a *StoryQAAuditor) SetRunner(runner Sandbox) {
	a.runner = runner
	if a.acceptanceRunner != nil {
		a.acceptanceRunner.runner = runner
	}
}

// SetAcceptanceRunner overrides the independent QA acceptance runner.
func (a *StoryQAAuditor) SetAcceptanceRunner(runner *QAAcceptanceRunner) {
	a.acceptanceRunner = runner
}

// SetE2ECommand sets the custom E2E command.
func (a *StoryQAAuditor) SetE2ECommand(cmd string) {
	a.e2eCmd = cmd
}

// SetE2EMode sets the E2E mode ("docker" or "native").
func (a *StoryQAAuditor) SetE2EMode(mode string) {
	a.e2eMode = mode
}

// SetE2EConfig sets the E2E configuration mode and command.
func (a *StoryQAAuditor) SetE2EConfig(e2e config.E2EConfig) {
	a.e2eMode = e2e.Mode
	if e2e.Command != "" {
		a.e2eCmd = e2e.Command
	}
}

// AuditStoryCompleteness verifies whether the codebase fulfills all requirements of the active story.
func (a *StoryQAAuditor) AuditStoryCompleteness(ctx context.Context, state *domain.State, storyPath string) (*StoryQAResult, error) {
	if state == nil || strings.TrimSpace(state.ProjectPath) == "" {
		return &StoryQAResult{Passed: true, Summary: "No project path; story QA audit skipped"}, nil
	}

	var executionLogs []string
	var hasFailure bool

	_ = PrepareTestEnvironment(state.ProjectPath)

	// 1. QA Pre-Flight & Partial Deliverable E2E Verification Gate:
	// Every completed story must execute its partial deliverable E2E verification.
	if ok, e2eLog := a.runPartialDeliverableE2E(ctx, state); !ok {
		hasFailure = true
		executionLogs = append(executionLogs, e2eLog)
	}

	// 2. QA Pre-Flight & Whole-Workspace Regression Guarding Gate:
	// Execute full project test suite to guarantee that modifications didn't introduce regressions to other modules.
	e2eCmd := a.detectE2ECommand(state.ProjectPath)
	testCmd := a.detectWorkspaceTestCommand(state.ProjectPath)
	if testCmd != "" && testCmd != e2eCmd && a.runner != nil {
		if !a.isCommandAllowed(testCmd) {
			fmt.Printf("⚠️  [Story QA Pre-Flight] Skipping workspace regression command %q: binary not in sandbox allowed_commands\n", testCmd)
		} else {
			fmt.Printf("🔍 [Story QA] Running Whole-Workspace Regression Guard: %q...\n", testCmd)
			testOut, testErr := a.runner.RunCommand(ctx, state.ProjectPath, testCmd, "")
			if testErr != nil {
				if isSandboxViolation(testErr, testOut) {
					fmt.Printf("⚠️  [Story QA Pre-Flight] Sandbox policy restriction on test command %q (%v); skipping workspace regression check.\n", testCmd, testErr)
				} else {
					fmt.Printf("❌ [Story QA] Whole-workspace regression detected: %v\n", testErr)
					hasFailure = true
					executionLogs = append(executionLogs, fmt.Sprintf("Whole-workspace regression detected (%s): %v\n%s", testCmd, testErr, capText(testOut, 1500)))
				}
			} else {
				fmt.Printf("✅ [Story QA] Whole-workspace regression check passed.\n")
			}
		}
	}

	// 3. Independent QA Acceptance Test Gate (True User / Zero Tricks):
	// Black-box tests in tests/acceptance/ verifying external behavior against SPEC.ste.md.
	if a.acceptanceRunner != nil {
		if !a.acceptanceRunner.HasAcceptanceTests(state.ProjectPath) {
			_, _ = a.acceptanceRunner.SynthesizeAcceptanceTests(ctx, state)
		}
		if a.acceptanceRunner.HasAcceptanceTests(state.ProjectPath) {
			accReport, accErr := a.acceptanceRunner.RunAcceptanceTests(ctx, state.ProjectPath)
			if accErr != nil || (accReport != nil && !accReport.Passed) {
				hasFailure = true
				var failureSummary string
				if accReport != nil && len(accReport.Failures) > 0 {
					failureSummary = fmt.Sprintf("Independent QA Acceptance Test Suite FAILED (%s):\n - %s",
						accReport.AcceptanceCommand, strings.Join(accReport.Failures, "\n - "))
				} else if accErr != nil {
					failureSummary = fmt.Sprintf("Independent QA Acceptance Test Suite execution error: %v", accErr)
				} else {
					failureSummary = "Independent QA Acceptance Test Suite FAILED"
				}
				fmt.Printf("❌ [Story QA] %s\n", failureSummary)
				executionLogs = append(executionLogs, failureSummary)
			}
		}
	}

	ctx, span := telemetry.Tracer().Start(ctx, "AuditStoryCompleteness",
		trace.WithAttributes(
			attribute.String("project_path", state.ProjectPath),
			attribute.String("story_path", storyPath),
		))
	defer span.End()

	targetStoryPath := storyPath
	if targetStoryPath == "" {
		targetStoryPath = state.Metadata.InputPath
	}
	if targetStoryPath != "" && !filepath.IsAbs(targetStoryPath) {
		targetStoryPath = filepath.Join(state.ProjectPath, targetStoryPath)
	}

	if targetStoryPath == "" || !storyFileExists(targetStoryPath) {
		// Fallback: look for user stories in roadmap/user-stories/*.md
		storiesDir := filepath.Join(state.ProjectPath, "roadmap", "user-stories")
		if matches, err := filepath.Glob(filepath.Join(storiesDir, "*.md")); err == nil && len(matches) > 0 {
			targetStoryPath = matches[0]
		}
	}

	var storyContent string
	if targetStoryPath != "" && storyFileExists(targetStoryPath) {
		if storyBytes, err := os.ReadFile(targetStoryPath); err == nil {
			storyContent = strings.TrimSpace(string(storyBytes))
		}
	}
	if storyContent == "" {
		if state.Metadata.FeatureName != "" {
			storyContent = fmt.Sprintf("User Story %s", state.Metadata.FeatureName)
		} else {
			return &StoryQAResult{Passed: true, Summary: "No user story content; story QA audit skipped"}, nil
		}
	}

	// Deterministic Machine-Readable Public Contract & Definition of Done Gate
	if storyContent != "" && a.runner != nil {
		if strings.Contains(storyContent, "```noctifab-contract") {
			contract, cErr := ParseStoryContract(targetStoryPath, storyContent)
			if cErr != nil || len(contract.PublicContracts) == 0 {
				errDesc := "malformed ```noctifab-contract block"
				if cErr != nil {
					errDesc = cErr.Error()
				}
				fmt.Printf("❌ [Story QA] Deterministic Public Contract parse error: %s\n", errDesc)
				hasFailure = true
				executionLogs = append(executionLogs, fmt.Sprintf("Public Contract Parse Error: %s", errDesc))
			} else {
				dodValidator := NewStoryDoDValidator(a.runner, 30*time.Second)
				dodReport, dodErr := dodValidator.ValidateStoryContracts(ctx, state.ProjectPath, contract)
				if dodErr == nil && dodReport != nil && !dodReport.Passed && len(dodReport.Failures) > 0 {
					fmt.Printf("❌ [Story QA] Deterministic Public Contract / DoD Verification Failed (%d failure(s))\n", len(dodReport.Failures))
					hasFailure = true
					executionLogs = append(executionLogs, fmt.Sprintf("Public Contract / DoD Verification Failed (%d contract(s) executed):\n - %s",
						dodReport.ExecutedContracts, strings.Join(dodReport.Failures, "\n - ")))
				} else if dodErr == nil && dodReport != nil && dodReport.Passed && dodReport.ExecutedContracts > 0 {
					fmt.Printf("✅ [Story QA] Deterministic Public Contract / DoD Verification Passed (%d contract(s) verified)\n", dodReport.ExecutedContracts)
				}
			}
		}
	}

	if a.llmClient == nil {
		if hasFailure {
			return &StoryQAResult{
				Passed:          false,
				Summary:         strings.Join(executionLogs, "\n"),
				MissingFeatures: executionLogs,
			}, nil
		}
		return &StoryQAResult{Passed: true, Summary: "No LLM client configured; story QA audit skipped"}, nil
	}

	// Collect accumulated git diff and workspace files
	git := NewGitClient(state.ProjectPath)
	baseBranch := ResolveBaseBranch(ctx, git, state.Metadata.BaseBranch)
	diffContext, _ := git.Run(ctx, false, "diff", baseBranch)
	if strings.TrimSpace(diffContext) == "" {
		diffContext, _ = git.Run(ctx, false, "diff", "HEAD~1")
	}

	var targetFiles []string
	for _, t := range state.Tasks {
		targetFiles = append(targetFiles, t.TargetFiles...)
	}
	workspaceSnapshot := CollectWorkspaceSourceSnapshot(ctx, state.ProjectPath, targetFiles, 20, 1500)
	_, containerContext := collectE2EContainerFiles(state.ProjectPath)
	prompt := a.buildPrompt(storyContent, workspaceSnapshot, diffContext, strings.Join(executionLogs, "\n"), containerContext, state)

	auditCtx := context.WithValue(ctx, AgentRoleKey, "qa")
	resp, err := a.llmClient.Complete(auditCtx, prompt)
	if err != nil {
		if hasFailure {
			return &StoryQAResult{
				Passed:          false,
				Summary:         strings.Join(executionLogs, "\n"),
				MissingFeatures: executionLogs,
			}, nil
		}
		return nil, fmt.Errorf("story QA audit LLM call failed: %w", err)
	}

	result := a.parseAuditResponse(resp)
	if hasFailure {
		result.Passed = false
		if len(result.MissingFeatures) == 0 {
			result.MissingFeatures = executionLogs
		}
		if result.Summary == "" || result.Summary == "Story QA audit evaluated" || result.Summary == "Empty LLM response received" {
			result.Summary = strings.Join(executionLogs, "\n")
		}
	}
	return result, nil
}

func (a *StoryQAAuditor) buildPrompt(storyContent, workspaceSnapshot, diffContext, executionLogs, containerContext string, state *domain.State) string {
	var sb strings.Builder
	sb.WriteString("You are the QA Acceptance Agent. Your task is to verify whether the accumulated generated code and tests completely fulfill all features, acceptance criteria, and Definitions of Done (DoD) defined in the target User Story.\n\n")
	sb.WriteString("TARGET USER STORY:\n```markdown\n")
	sb.WriteString(capText(storyContent, 8000))
	sb.WriteString("\n```\n\n")

	if strings.TrimSpace(executionLogs) != "" {
		sb.WriteString("TEST & E2E EXECUTION LOGS:\n```\n")
		sb.WriteString(capText(executionLogs, 4000))
		sb.WriteString("\n```\n\n")
	}

	if strings.TrimSpace(containerContext) != "" {
		sb.WriteString("CONTAINER & HARNESS CONFIGURATION FILES (docker-compose, Dockerfile, scripts):\n")
		sb.WriteString(containerContext)
		sb.WriteString("\n\n")
	}

	sb.WriteString("WORKSPACE SOURCE FILES:\n```\n")
	sb.WriteString(capText(workspaceSnapshot, 6000))
	sb.WriteString("\n```\n\n")

	if strings.TrimSpace(diffContext) != "" {
		sb.WriteString("ACCUMULATED GIT DIFF:\n```diff\n")
		sb.WriteString(capText(diffContext, 6000))
		sb.WriteString("\n```\n\n")
	}

	sb.WriteString("TASKS COMPLETED IN THIS STORY:\n")
	for _, t := range state.Tasks {
		fmt.Fprintf(&sb, "- [%s] %s (Target Files: %v)\n", t.ID, t.Title, t.TargetFiles)
	}
	sb.WriteString("\n")

	sb.WriteString(`AUDIT RULES:
1. Inspect every feature, command, CLI option, network wire behavior, error envelope, and edge case required by the User Story.
2. Verify that each requirement is genuinely implemented in production code and verified by real tests (no stubs, no mocks in production code, no empty pass blocks).
3. If test execution or containerized E2E verification failed, analyze the failure logs, error traces, Dockerfile, and docker-compose files above. Explain the root cause in "summary" and provide concrete, actionable fixes in "missing_features" (e.g. docker-compose service names, missing dependencies, Dockerfile commands, or code fixes) so the Generator and Tester agents can directly repair them.
4. If ANY required feature from the user story is missing, incomplete, or test execution failed, set "passed": false and enumerate each missing item in "missing_features".
5. If all features and acceptance criteria are fully met and tests pass, set "passed": true and "missing_features": [].

Respond ONLY with a JSON object in this exact schema:
{
  "actions": [
    {
      "tool": "submit_story_qa_audit",
      "args": {
        "passed": true,
        "summary": "Detailed assessment of story completeness",
        "missing_features": []
      }
    }
  ]
}
`)
	return sb.String()
}

func (a *StoryQAAuditor) parseAuditResponse(resp *domain.LLMResponse) *StoryQAResult {
	if resp == nil {
		return &StoryQAResult{Passed: false, Summary: "Empty LLM response received"}
	}

	for _, act := range resp.Actions {
		if act.Tool == "submit_story_qa_audit" {
			passed, _ := act.Args["passed"].(bool)
			summary, _ := act.Args["summary"].(string)
			var missing []string
			if rawMissing, ok := act.Args["missing_features"].([]any); ok {
				for _, m := range rawMissing {
					if ms, ok := m.(string); ok && strings.TrimSpace(ms) != "" {
						missing = append(missing, strings.TrimSpace(ms))
					}
				}
			}
			return &StoryQAResult{
				Passed:          passed && len(missing) == 0,
				Summary:         summary,
				MissingFeatures: missing,
			}
		}
	}

	// Fallback JSON parsing from reasoning
	if strings.Contains(resp.Reasoning, "submit_story_qa_audit") || strings.Contains(resp.Reasoning, "\"passed\"") {
		var parsed struct {
			Passed          bool     `json:"passed"`
			Summary         string   `json:"summary"`
			MissingFeatures []string `json:"missing_features"`
		}
		start := strings.Index(resp.Reasoning, "{")
		end := strings.LastIndex(resp.Reasoning, "}")
		if start >= 0 && end > start {
			if err := json.Unmarshal([]byte(resp.Reasoning[start:end+1]), &parsed); err == nil {
				return &StoryQAResult{
					Passed:          parsed.Passed && len(parsed.MissingFeatures) == 0,
					Summary:         parsed.Summary,
					MissingFeatures: parsed.MissingFeatures,
				}
			}
		}
	}

	summary := strings.TrimSpace(resp.Reasoning)
	if summary == "" {
		summary = "Story QA audit evaluated"
	}
	return &StoryQAResult{
		Passed:  true,
		Summary: summary,
	}
}

func storyFileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}
