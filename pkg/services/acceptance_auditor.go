package services

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/diegojromerolopez/noctifab/pkg/domain"
	"github.com/diegojromerolopez/noctifab/pkg/infrastructure/config"
	"github.com/diegojromerolopez/noctifab/pkg/infrastructure/prompts"
	"github.com/diegojromerolopez/noctifab/pkg/infrastructure/telemetry"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

// AcceptanceAuditResult encapsulates the whole-project audit against SPEC.md.
type AcceptanceAuditResult struct {
	Passed  bool     `json:"passed"`
	Summary string   `json:"summary"`
	Gaps    []string `json:"gaps,omitempty"`
}

// AcceptanceAuditor compares the implemented codebase against the root SPEC.md.
type AcceptanceAuditor struct {
	llmClient       domain.LLMClient
	renderer        PromptRenderer
	runner          Sandbox
	e2eCmd          string
	defaultTestCmd  string
	allowedCommands []string
}

// NewAcceptanceAuditor instantiates an AcceptanceAuditor service.
func NewAcceptanceAuditor(client domain.LLMClient, renderer PromptRenderer, runner ...Sandbox) *AcceptanceAuditor {
	if renderer == nil {
		renderer = prompts.NewDefaultRenderer()
	}
	var r Sandbox
	if len(runner) > 0 {
		r = runner[0]
	}
	return &AcceptanceAuditor{
		llmClient: client,
		renderer:  renderer,
		runner:    r,
	}
}

// ConfigureFromConfig attaches sandbox policies and configured test commands.
func (a *AcceptanceAuditor) ConfigureFromConfig(cfg *config.Config) {
	if cfg != nil {
		a.defaultTestCmd = cfg.Sandbox.TestCommand
		a.allowedCommands = cfg.Sandbox.AllowedCommands
	}
}

// SetRunner sets the sandbox runner.
func (a *AcceptanceAuditor) SetRunner(runner Sandbox) {
	a.runner = runner
}

// SetE2ECommand sets the custom E2E command.
func (a *AcceptanceAuditor) SetE2ECommand(cmd string) {
	a.e2eCmd = cmd
}

// SetDefaultTestCommand sets the default test command.
func (a *AcceptanceAuditor) SetDefaultTestCommand(cmd string) {
	a.defaultTestCmd = cmd
}

// SetAllowedCommands sets the whitelisted binaries allowed by the sandbox policy.
func (a *AcceptanceAuditor) SetAllowedCommands(cmds []string) {
	a.allowedCommands = cmds
}

// AuditProjectAcceptance verifies whether the implemented codebase satisfies SPEC.md.
func (a *AcceptanceAuditor) AuditProjectAcceptance(ctx context.Context, state *domain.State) (*AcceptanceAuditResult, error) {
	if state == nil || strings.TrimSpace(state.ProjectPath) == "" {
		return &AcceptanceAuditResult{Passed: true, Summary: "No project path provided; audit skipped"}, nil
	}

	ctx, span := telemetry.Tracer().Start(ctx, "AuditProjectAcceptance",
		trace.WithAttributes(
			attribute.String("project_path", state.ProjectPath),
			attribute.String("feature_name", state.Metadata.FeatureName),
		))
	defer span.End()

	specPath := filepath.Join(state.ProjectPath, "SPEC.md")
	specData, err := os.ReadFile(specPath)
	if err != nil {
		// If SPEC.md does not exist, acceptance audit passes gracefully
		return &AcceptanceAuditResult{
			Passed:  true,
			Summary: fmt.Sprintf("SPEC.md not found at %s; acceptance audit skipped", specPath),
		}, nil
	}

	specContent := strings.TrimSpace(string(specData))
	if specContent == "" {
		return &AcceptanceAuditResult{
			Passed:  true,
			Summary: "SPEC.md is empty; acceptance audit skipped",
		}, nil
	}

	if a.llmClient == nil {
		return &AcceptanceAuditResult{
			Passed:  true,
			Summary: "No LLM client configured for auditor; acceptance audit skipped",
		}, nil
	}

	// 1. Behavioral E2E Execution Pre-Flight Gate
	var e2eLog string
	e2eCmd := a.detectE2ECommand(state.ProjectPath)
	if e2eCmd != "" && a.runner != nil {
		if !a.isCommandAllowed(e2eCmd) {
			e2eLog = fmt.Sprintf("⚠️  E2E command %q skipped: binary not in sandbox allowed_commands", e2eCmd)
		} else {
			fmt.Printf("🔍 [Acceptance Gate] Running E2E verification command: %q...\n", e2eCmd)
			e2eOut, e2eErr := a.runner.RunCommand(ctx, state.ProjectPath, e2eCmd, "")
			if e2eErr != nil {
				if isSandboxViolation(e2eErr, e2eOut) {
					e2eLog = fmt.Sprintf("⚠️  Sandbox policy restriction on E2E command %q (%v); skipped.", e2eCmd, e2eErr)
				} else {
					e2eLog = fmt.Sprintf("❌ E2E test execution FAILED (%s):\n%s\nError: %v", e2eCmd, capText(e2eOut, 2000), e2eErr)
				}
			} else {
				e2eLog = fmt.Sprintf("✅ E2E test execution PASSED (%s):\n%s", e2eCmd, capText(e2eOut, 2000))
			}
		}
	} else if e2eCmd == "" {
		e2eLog = "⚠️  No E2E test target detected (no 'e2e:' in Makefile or docker-compose.yml)."
	}

	workspaceFiles := a.collectWorkspaceSnapshot(ctx, state.ProjectPath)
	storyContracts := a.formatStoryContracts(state)
	taskSummaries := a.formatTaskSummaries(state)

	promptData := prompts.AcceptanceAuditPromptData{
		Spec:            capText(specContent, 25000),
		WorkspaceFiles:  workspaceFiles,
		StoryContracts:  storyContracts,
		PublicContracts: a.formatPublicContracts(state),
		TaskSummaries:   taskSummaries,
		E2ELog:          e2eLog,
	}

	rendered, err := a.renderer.Render(prompts.AgentAuditor, "acceptance_audit", promptData)
	if err != nil {
		return nil, fmt.Errorf("failed to render acceptance audit prompt: %w", err)
	}

	auditCtx := context.WithValue(ctx, AgentRoleKey, "auditor")
	auditCtx = domain.WithUncompactableTail(auditCtx, len(rendered.Contract))

	resp, err := a.llmClient.Complete(auditCtx, rendered.Full())
	if err != nil {
		return nil, fmt.Errorf("acceptance audit LLM call failed: %w", err)
	}

	return a.parseAuditResponse(resp), nil
}

func (a *AcceptanceAuditor) detectE2ECommand(projectPath string) string {
	if a.e2eCmd != "" {
		return a.e2eCmd
	}
	if _, err := os.Stat(filepath.Join(projectPath, "docker-compose.e2e.yml")); err == nil {
		return "docker compose -f docker-compose.e2e.yml up --build --exit-code-from test-runner"
	}
	if _, err := os.Stat(filepath.Join(projectPath, "docker-compose.yml")); err == nil {
		if content, rErr := os.ReadFile(filepath.Join(projectPath, "docker-compose.yml")); rErr == nil && strings.Contains(string(content), "e2e:") {
			return "docker compose up --build --exit-code-from e2e"
		}
	}
	if _, err := os.Stat(filepath.Join(projectPath, "Makefile")); err == nil {
		content, rErr := os.ReadFile(filepath.Join(projectPath, "Makefile"))
		if rErr == nil && strings.Contains(string(content), "e2e:") {
			return "make e2e"
		}
	}
	return ""
}

func (a *AcceptanceAuditor) isCommandAllowed(cmdStr string) bool {
	allowed := a.allowedCommands
	if len(allowed) == 0 {
		return true
	}
	parts := strings.Fields(cmdStr)
	if len(parts) == 0 {
		return false
	}
	base := filepath.Base(parts[0])
	for _, a := range allowed {
		if a == parts[0] || a == base {
			return true
		}
	}
	return false
}

func (a *AcceptanceAuditor) collectWorkspaceSnapshot(ctx context.Context, projectPath string) string {
	files, err := ListWorkspaceSourceFiles(ctx, projectPath, nil)
	if err != nil || len(files) == 0 {
		return "Workspace File Tree: (empty)"
	}

	var codeSnippets []string
	for _, rel := range files {
		lower := strings.ToLower(rel)
		if strings.Contains(lower, "command") || strings.Contains(lower, "main") ||
			strings.Contains(lower, "cli") || strings.Contains(lower, "server") ||
			strings.Contains(lower, "app") || strings.HasSuffix(lower, "makefile") ||
			strings.Contains(lower, "compose") || strings.HasSuffix(lower, ".sh") ||
			strings.HasSuffix(lower, "pyproject.toml") || strings.HasSuffix(lower, "go.mod") ||
			strings.HasSuffix(lower, "cargo.toml") || strings.Contains(lower, "test") ||
			strings.Contains(lower, "spec") || strings.Contains(lower, "store") ||
			strings.Contains(lower, "resp") || strings.Contains(lower, "persist") {
			fullPath := filepath.Join(projectPath, rel)
			if content, readErr := os.ReadFile(fullPath); readErr == nil {
				snippet := capText(string(content), 3000)
				codeSnippets = append(codeSnippets, fmt.Sprintf("--- %s ---\n%s\n", rel, snippet))
			}
		}
	}

	var sb strings.Builder
	sb.WriteString("Workspace File Tree:\n")
	for _, f := range files {
		sb.WriteString("- ")
		sb.WriteString(f)
		sb.WriteString("\n")
	}
	if len(codeSnippets) > 0 {
		sb.WriteString("\nKey Source & Test Implementations:\n")
		for _, s := range codeSnippets {
			sb.WriteString(s)
			sb.WriteString("\n")
		}
	}
	return sb.String()
}

func (a *AcceptanceAuditor) formatStoryContracts(state *domain.State) string {
	if state == nil || len(state.StoryContracts) == 0 {
		return "None"
	}
	var sb strings.Builder
	for _, c := range state.StoryContracts {
		fmt.Fprintf(&sb, "Story ID: %s (File: %s)\n", c.StoryID, c.SourcePath)
		for _, pc := range c.PublicContracts {
			fmt.Fprintf(&sb, " - Contract [%s]: interface=%s allowed_executables=%v\n", pc.ID, pc.Interface, pc.AllowedExecutables)
		}
	}
	return sb.String()
}

func (a *AcceptanceAuditor) formatPublicContracts(state *domain.State) string {
	if state == nil || len(state.StoryContracts) == 0 {
		return "None"
	}
	var contracts []string
	for _, c := range state.StoryContracts {
		for _, pc := range c.PublicContracts {
			contracts = append(contracts, fmt.Sprintf("%s (%s)", pc.ID, pc.Interface))
		}
	}
	return strings.Join(contracts, ", ")
}

func (a *AcceptanceAuditor) formatTaskSummaries(state *domain.State) string {
	if state == nil || len(state.Tasks) == 0 {
		return "None"
	}
	var sb strings.Builder
	for _, t := range state.Tasks {
		fmt.Fprintf(&sb, "- [%s] %s: %s (files: %v)\n", t.ID, t.Status, t.Title, t.TargetFiles)
	}
	return sb.String()
}

func (a *AcceptanceAuditor) parseAuditResponse(resp *domain.LLMResponse) *AcceptanceAuditResult {
	if resp == nil {
		return &AcceptanceAuditResult{Passed: false, Summary: "Empty LLM response received"}
	}

	for _, act := range resp.Actions {
		if act.Tool == "submit_acceptance_audit" {
			passed, _ := act.Args["passed"].(bool)
			summary, _ := act.Args["summary"].(string)
			var gaps []string
			if rawGaps, ok := act.Args["gaps"].([]any); ok {
				for _, g := range rawGaps {
					if gs, ok := g.(string); ok && strings.TrimSpace(gs) != "" {
						gaps = append(gaps, strings.TrimSpace(gs))
					}
				}
			}
			return &AcceptanceAuditResult{
				Passed:  passed && len(gaps) == 0,
				Summary: summary,
				Gaps:    gaps,
			}
		}
	}

	// Fallback JSON parsing from reasoning or content
	if strings.Contains(resp.Reasoning, "submit_acceptance_audit") || strings.Contains(resp.Reasoning, "\"passed\"") {
		var parsed struct {
			Passed  bool     `json:"passed"`
			Summary string   `json:"summary"`
			Gaps    []string `json:"gaps"`
		}
		start := strings.Index(resp.Reasoning, "{")
		end := strings.LastIndex(resp.Reasoning, "}")
		if start >= 0 && end > start {
			if err := json.Unmarshal([]byte(resp.Reasoning[start:end+1]), &parsed); err == nil {
				return &AcceptanceAuditResult{
					Passed:  parsed.Passed && len(parsed.Gaps) == 0,
					Summary: parsed.Summary,
					Gaps:    parsed.Gaps,
				}
			}
		}
	}

	summary := strings.TrimSpace(resp.Reasoning)
	if summary == "" {
		summary = "Acceptance audit evaluated"
	}
	return &AcceptanceAuditResult{
		Passed:  true,
		Summary: summary,
	}
}
