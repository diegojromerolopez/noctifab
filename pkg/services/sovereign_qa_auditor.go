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
	"github.com/diegojromerolopez/noctifab/pkg/infrastructure/telemetry"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

// FailurePrediction represents a forecasted latent defect or future failure risk.
type FailurePrediction struct {
	RiskLevel       string `json:"risk_level"`       // "critical", "high", "medium", "low"
	Category        string `json:"category"`         // e.g. "buffer_mutation", "connection_lifecycle", "concurrency", "resource_leak", "unhandled_error"
	Description     string `json:"description"`      // Explanation of the latent flaw
	AffectedFile    string `json:"affected_file"`    // Relative path of vulnerable file
	TriggerScenario string `json:"trigger_scenario"` // How this will fail in future/production runs
	Mitigation      string `json:"mitigation"`       // Concrete code fix required
}

// SovereignQAResult encapsulates both empirical probe execution and predictive code/log analysis.
type SovereignQAResult struct {
	Passed                 bool                `json:"passed"`
	Summary                string              `json:"summary"`
	ProbeCommand           string              `json:"probe_command,omitempty"`
	ProbeOutput            string              `json:"probe_output,omitempty"`
	ImmediateErrors        []string            `json:"immediate_errors,omitempty"`
	PredictedFailures      []FailurePrediction `json:"predicted_failures,omitempty"`
	ActionableRequirements []string            `json:"actionable_requirements,omitempty"`
	Fixes                  []ProposedFix       `json:"fixes,omitempty"`
}

// SovereignQAAuditor verifies interface compliance through empirical execution and forecasts future failures.
type SovereignQAAuditor struct {
	llmClient       domain.LLMClient
	runner          Sandbox
	defaultTestCmd  string
	e2eCmd          string
	e2eMode         string
	allowedCommands []string
}

// NewSovereignQAAuditor creates a new SovereignQAAuditor instance.
func NewSovereignQAAuditor(client domain.LLMClient, runner ...Sandbox) *SovereignQAAuditor {
	var r Sandbox
	if len(runner) > 0 {
		r = runner[0]
	}
	return &SovereignQAAuditor{
		llmClient: client,
		runner:    r,
	}
}

// ConfigureFromConfig loads sandbox configurations and commands.
func (a *SovereignQAAuditor) ConfigureFromConfig(cfg *config.Config) {
	if cfg != nil {
		a.defaultTestCmd = cfg.Sandbox.TestCommand
		a.allowedCommands = cfg.Sandbox.AllowedCommands
		a.e2eMode = cfg.Sandbox.GetE2EMode()
		if cmd := cfg.Sandbox.GetE2ECommand(); cmd != "" {
			a.e2eCmd = cmd
		}
	}
}

// SetRunner sets the sandbox runner.
func (a *SovereignQAAuditor) SetRunner(runner Sandbox) {
	a.runner = runner
}

// SetE2ECommand sets the custom E2E command.
func (a *SovereignQAAuditor) SetE2ECommand(cmd string) {
	a.e2eCmd = cmd
}

// SetAllowedCommands sets the command allowlist.
func (a *SovereignQAAuditor) SetAllowedCommands(cmds []string) {
	a.allowedCommands = cmds
}

// DetectProbeCommand discovers the appropriate interface verification command.
func (a *SovereignQAAuditor) DetectProbeCommand(projectPath string) string {
	if detected := DetectE2ECommand(projectPath, a.e2eMode, a.e2eCmd); detected != "" {
		return detected
	}
	candidates := []string{
		"tests/e2e/run_tests.sh",
		"tests/run_tests.sh",
		"tests/e2e_scenarios.sh",
		"scripts/run_e2e.sh",
	}
	for _, rel := range candidates {
		full := filepath.Join(projectPath, rel)
		if info, err := os.Stat(full); err == nil && !info.IsDir() {
			return "sh " + rel
		}
	}
	return ""
}

// RunInterfaceProbe executes the interface verification command against the running workspace.
func (a *SovereignQAAuditor) RunInterfaceProbe(ctx context.Context, projectPath string) (string, string, error) {
	cmd := a.DetectProbeCommand(projectPath)
	if cmd == "" || a.runner == nil {
		return "", "", nil
	}

	guard := NewContainerTeardownGuard(nil, nil)
	_ = guard.PreFlightClean(ctx, projectPath)
	defer func() {
		_ = guard.PostRunClean(ctx, projectPath)
	}()

	out, err := a.runner.RunCommand(ctx, projectPath, cmd, "")
	return cmd, out, err
}

// Audit analyzes execution logs and code snapshot to check for immediate failures and predict future ones.
func (a *SovereignQAAuditor) Audit(ctx context.Context, state *domain.State, probeCmd, probeOut string, probeErr error) (*SovereignQAResult, error) {
	if state == nil || strings.TrimSpace(state.ProjectPath) == "" {
		return &SovereignQAResult{Passed: true, Summary: "No project path provided; audit skipped"}, nil
	}

	ctx, span := telemetry.Tracer().Start(ctx, "SovereignQAAuditor.Audit",
		trace.WithAttributes(
			attribute.String("project_path", state.ProjectPath),
			attribute.String("probe_command", probeCmd),
		))
	defer span.End()

	var immediateErrors []string
	if probeErr != nil {
		immediateErrors = append(immediateErrors, fmt.Sprintf("Interface probe %q failed: %v\nOutput: %s", probeCmd, probeErr, capText(probeOut, 1500)))
	}

	heuristics := a.runDeterministicPredictionHeuristics(state.ProjectPath)
	var predictedFailures []FailurePrediction
	predictedFailures = append(predictedFailures, heuristics...)

	codeSnapshot := a.collectCodebaseSnapshot(ctx, state.ProjectPath)
	specPath := ResolveSpecPath(state.ProjectPath)
	specContent, _ := os.ReadFile(specPath)

	if a.llmClient == nil {
		passed := len(immediateErrors) == 0 && !hasCriticalPredictions(predictedFailures)
		summary := "Deterministic Sovereign QA Audit completed"
		if !passed {
			summary = fmt.Sprintf("Sovereign QA detected %d immediate error(s) and %d predicted failure risk(s)", len(immediateErrors), len(predictedFailures))
		}
		var reqs []string
		reqs = append(reqs, immediateErrors...)
		for _, p := range predictedFailures {
			reqs = append(reqs, fmt.Sprintf("[PREDICTED FAILURE] %s: %s (Trigger: %s)", p.AffectedFile, p.Description, p.TriggerScenario))
		}
		return &SovereignQAResult{
			Passed:                 passed,
			Summary:                summary,
			ProbeCommand:           probeCmd,
			ProbeOutput:            probeOut,
			ImmediateErrors:        immediateErrors,
			PredictedFailures:      predictedFailures,
			ActionableRequirements: reqs,
		}, nil
	}

	prompt := a.buildAuditPrompt(string(specContent), probeCmd, probeOut, probeErr, immediateErrors, predictedFailures, codeSnapshot)
	auditCtx := context.WithValue(ctx, AgentRoleKey, "qa")
	resp, err := a.llmClient.Complete(auditCtx, prompt)
	if err != nil {
		if len(immediateErrors) > 0 || hasCriticalPredictions(predictedFailures) {
			return &SovereignQAResult{
				Passed:            false,
				Summary:           "Interface probe execution or heuristic prediction failed (LLM call failed)",
				ProbeCommand:      probeCmd,
				ProbeOutput:       probeOut,
				ImmediateErrors:   immediateErrors,
				PredictedFailures: predictedFailures,
			}, nil
		}
		return nil, fmt.Errorf("sovereign QA LLM call failed: %w", err)
	}

	result := a.parseAuditResponse(resp, probeCmd, probeOut, immediateErrors, predictedFailures)
	return result, nil
}

// ExecuteAndAudit runs the interface probe and analyzes logs and code in a single workflow.
func (a *SovereignQAAuditor) ExecuteAndAudit(ctx context.Context, state *domain.State) (*SovereignQAResult, error) {
	cmd, out, err := a.RunInterfaceProbe(ctx, state.ProjectPath)
	return a.Audit(ctx, state, cmd, out, err)
}

func (a *SovereignQAAuditor) runDeterministicPredictionHeuristics(projectPath string) []FailurePrediction {
	var predictions []FailurePrediction
	files, err := ListWorkspaceSourceFiles(context.Background(), projectPath, nil)
	if err != nil {
		return nil
	}

	for _, rel := range files {
		full := filepath.Join(projectPath, rel)
		data, readErr := os.ReadFile(full)
		if readErr != nil {
			continue
		}
		content := string(data)

		// 1. Buffer type mutation: returning immutable bytes assigned back to mutable buffer
		if strings.Contains(content, "buffer.extend(") && strings.Contains(content, "decode_resp_stream") {
			if strings.Contains(content, "remaining = ") && !strings.Contains(content, "bytearray(remaining") {
				predictions = append(predictions, FailurePrediction{
					RiskLevel:       "critical",
					Category:        "buffer_mutation",
					Description:     "Stream buffer reassignment may store immutable bytes instead of bytearray, causing buffer.extend() to crash on subsequent commands",
					AffectedFile:    rel,
					TriggerScenario: "Client sends multiple commands over a single persistent TCP connection or pipelined stream",
					Mitigation:      "Ensure remaining buffer slice is wrapped as bytearray(remaining) before calling buffer.extend()",
				})
			}
		}

		// 2. Single-command connection teardown anti-pattern in server loop
		if (strings.Contains(rel, "server") || strings.Contains(rel, "main")) &&
			strings.Contains(content, "socket.accept(") {
			if strings.Contains(content, "client_socket.close()") && !strings.Contains(content, "while ") && !strings.Contains(content, "for ") {
				predictions = append(predictions, FailurePrediction{
					RiskLevel:       "high",
					Category:        "connection_lifecycle",
					Description:     "Server closes client socket immediately without persistent connection loop",
					AffectedFile:    rel,
					TriggerScenario: "Standard clients expect persistent connections across multiple requests",
					Mitigation:      "Wrap per-client command processing in a continuous loop reading until EOF or close",
				})
			}
		}
	}
	return predictions
}

func (a *SovereignQAAuditor) collectCodebaseSnapshot(ctx context.Context, projectPath string) string {
	files, err := ListWorkspaceSourceFiles(ctx, projectPath, nil)
	if err != nil || len(files) == 0 {
		return "No source files found."
	}
	var snippets []string
	for _, rel := range files {
		lower := strings.ToLower(rel)
		if strings.Contains(lower, "main") || strings.Contains(lower, "server") ||
			strings.Contains(lower, "handler") || strings.Contains(lower, "parser") ||
			strings.Contains(lower, "protocol") || strings.Contains(lower, "storage") ||
			strings.Contains(lower, "e2e") {
			full := filepath.Join(projectPath, rel)
			if data, err := os.ReadFile(full); err == nil {
				snippets = append(snippets, fmt.Sprintf("--- %s ---\n%s\n", rel, capText(string(data), 2500)))
			}
		}
	}
	return strings.Join(snippets, "\n")
}

func (a *SovereignQAAuditor) buildAuditPrompt(spec, probeCmd, probeOut string, probeErr error, immediateErrors []string, heuristics []FailurePrediction, codeSnapshot string) string {
	var sb strings.Builder
	sb.WriteString("You are the Sovereign QA Auditor Agent.\n")
	sb.WriteString("Your mandate is to guarantee that the project genuinely WORKS by analyzing runtime execution logs and inspecting the codebase.\n\n")
	sb.WriteString("CRITICAL DIRECTIVE: PREDICTIVE FAILURE ANALYSIS\n")
	sb.WriteString("Inspect the execution logs and code NOT ONLY for immediate errors, but to PREDICT if future failures can happen.\n")
	sb.WriteString("Even if the current probe or test run passed, search for latent defects that could break in production under extended use or edge cases:\n")
	sb.WriteString("- Buffer type mutations (e.g. methods returning immutable bytes where bytearray is expected on subsequent calls)\n")
	sb.WriteString("- Connection lifecycle violations (e.g. single-request socket closure instead of persistent connections)\n")
	sb.WriteString("- Concurrency & state race hazards\n")
	sb.WriteString("- Resource/descriptor leaks\n")
	sb.WriteString("- Unhandled EOF, client disconnections, or malformed frame panics\n\n")

	sb.WriteString("SPECIFICATION (SPEC.md):\n```markdown\n")
	sb.WriteString(capText(spec, 4000))
	sb.WriteString("\n```\n\n")

	sb.WriteString("INTERFACE PROBE EXECUTION:\n")
	fmt.Fprintf(&sb, "Command: %s\n", probeCmd)
	if probeErr != nil {
		fmt.Fprintf(&sb, "Status: FAILED (%v)\n", probeErr)
	} else {
		sb.WriteString("Status: PASSED\n")
	}
	sb.WriteString("Output Logs:\n```\n")
	sb.WriteString(capText(probeOut, 3000))
	sb.WriteString("\n```\n\n")

	if len(heuristics) > 0 {
		sb.WriteString("STATIC HEURISTIC RISK PREDICTIONS:\n")
		for _, h := range heuristics {
			fmt.Fprintf(&sb, "- [%s] %s (%s): %s\n", h.RiskLevel, h.Category, h.AffectedFile, h.Description)
		}
		sb.WriteString("\n")
	}

	sb.WriteString("KEY CODE IMPLEMENTATIONS:\n```\n")
	sb.WriteString(capText(codeSnapshot, 5000))
	sb.WriteString("\n```\n\n")

	sb.WriteString(`AUDIT RULES:
1. Verify if the interface probe or tests failed. If so, document root causes in "immediate_errors".
2. Perform Predictive Failure Analysis: identify latent defects in code or logs that will cause future failures. List them in "predicted_failures" with risk_level ("critical", "high", "medium"), category, description, affected_file, trigger_scenario, and mitigation.
3. If ANY immediate error exists, OR ANY critical/high predicted failure risk is detected, set "passed": false.
4. Formulate concrete, actionable tasks in "actionable_requirements" and "fixes" so the next internal Noctifab loop can eliminate both current bugs and predicted future failures.
5. If the probe passed AND no high/critical failure risks are predicted, set "passed": true.

Respond ONLY with a JSON object in this exact schema:
{
  "actions": [
    {
      "tool": "submit_sovereign_qa",
      "args": {
        "passed": true,
        "summary": "Detailed QA evaluation including empirical execution and future failure prediction",
        "immediate_errors": [],
        "predicted_failures": [
          {
            "risk_level": "critical",
            "category": "buffer_mutation",
            "description": "Explanation of latent flaw",
            "affected_file": "src/main.py",
            "trigger_scenario": "Client executes second command on persistent connection",
            "mitigation": "Ensure buffer slice is bytearray"
          }
        ],
        "actionable_requirements": [],
        "fixes": [
          {
            "file": "src/main.py",
            "action": "modify",
            "description": "Fix buffer handling"
          }
        ]
      }
    }
  ]
}
`)
	return sb.String()
}

func (a *SovereignQAAuditor) parseAuditResponse(resp *domain.LLMResponse, probeCmd, probeOut string, immediateErrors []string, heuristics []FailurePrediction) *SovereignQAResult {
	result := &SovereignQAResult{
		ProbeCommand:      probeCmd,
		ProbeOutput:       probeOut,
		ImmediateErrors:   immediateErrors,
		PredictedFailures: heuristics,
	}

	if resp == nil {
		result.Passed = len(immediateErrors) == 0 && !hasCriticalPredictions(heuristics)
		result.Summary = "Empty LLM response received"
		return result
	}

	for _, act := range resp.Actions {
		if act.Tool == "submit_sovereign_qa" {
			a.populateFromResultArgs(result, act.Args)
			return result
		}
	}

	// JSON fallback parsing
	if strings.Contains(resp.Reasoning, "submit_sovereign_qa") || strings.Contains(resp.Reasoning, "\"passed\"") {
		start := strings.Index(resp.Reasoning, "{")
		end := strings.LastIndex(resp.Reasoning, "}")
		if start >= 0 && end > start {
			var raw map[string]any
			if err := json.Unmarshal([]byte(resp.Reasoning[start:end+1]), &raw); err == nil {
				if args, ok := raw["args"].(map[string]any); ok {
					a.populateFromResultArgs(result, args)
					return result
				}
				a.populateFromResultArgs(result, raw)
				return result
			}
		}
	}

	result.Passed = len(immediateErrors) == 0 && !hasCriticalPredictions(result.PredictedFailures)
	result.Summary = strings.TrimSpace(resp.Reasoning)
	if result.Summary == "" {
		result.Summary = "Sovereign QA audit completed"
	}
	return result
}

func (a *SovereignQAAuditor) populateFromResultArgs(result *SovereignQAResult, args map[string]any) {
	if p, ok := args["passed"].(bool); ok {
		result.Passed = p
	}
	if s, ok := args["summary"].(string); ok {
		result.Summary = s
	}
	if imm, ok := args["immediate_errors"].([]any); ok {
		for _, e := range imm {
			if es, ok := e.(string); ok && strings.TrimSpace(es) != "" {
				result.ImmediateErrors = append(result.ImmediateErrors, strings.TrimSpace(es))
			}
		}
	}
	if preds, ok := args["predicted_failures"].([]any); ok {
		for _, p := range preds {
			if pm, ok := p.(map[string]any); ok {
				fp := FailurePrediction{
					RiskLevel:       getString(pm, "risk_level"),
					Category:        getString(pm, "category"),
					Description:     getString(pm, "description"),
					AffectedFile:    getString(pm, "affected_file"),
					TriggerScenario: getString(pm, "trigger_scenario"),
					Mitigation:      getString(pm, "mitigation"),
				}
				if fp.Description != "" {
					result.PredictedFailures = append(result.PredictedFailures, fp)
				}
			}
		}
	}
	if reqs, ok := args["actionable_requirements"].([]any); ok {
		for _, r := range reqs {
			if rs, ok := r.(string); ok && strings.TrimSpace(rs) != "" {
				result.ActionableRequirements = append(result.ActionableRequirements, strings.TrimSpace(rs))
			}
		}
	}
	if fixes, ok := args["fixes"].([]any); ok {
		for _, f := range fixes {
			if fm, ok := f.(map[string]any); ok {
				pf := ProposedFix{
					File:        getString(fm, "file"),
					Action:      getString(fm, "action"),
					Description: getString(fm, "description"),
				}
				if pf.File != "" || pf.Description != "" {
					result.Fixes = append(result.Fixes, pf)
				}
			}
		}
	}

	if len(result.ImmediateErrors) > 0 || hasCriticalPredictions(result.PredictedFailures) {
		result.Passed = false
	}
}

func hasCriticalPredictions(preds []FailurePrediction) bool {
	for _, p := range preds {
		lvl := strings.ToLower(p.RiskLevel)
		if lvl == "critical" || lvl == "high" {
			return true
		}
	}
	return false
}

func getString(m map[string]any, key string) string {
	if v, ok := m[key].(string); ok {
		return strings.TrimSpace(v)
	}
	return ""
}
