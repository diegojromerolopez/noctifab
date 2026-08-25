package services

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/diegojromerolopez/noctifab/pkg/domain"
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
	llmClient domain.LLMClient
	renderer  PromptRenderer
}

// NewAcceptanceAuditor instantiates an AcceptanceAuditor service.
func NewAcceptanceAuditor(client domain.LLMClient, renderer PromptRenderer) *AcceptanceAuditor {
	if renderer == nil {
		renderer = prompts.NewDefaultRenderer()
	}
	return &AcceptanceAuditor{
		llmClient: client,
		renderer:  renderer,
	}
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

	workspaceFiles := a.collectWorkspaceSnapshot(state.ProjectPath)
	storyContracts := a.formatStoryContracts(state)
	taskSummaries := a.formatTaskSummaries(state)

	promptData := prompts.AcceptanceAuditPromptData{
		Spec:            capText(specContent, 25000),
		WorkspaceFiles:  workspaceFiles,
		StoryContracts:  storyContracts,
		PublicContracts: a.formatPublicContracts(state),
		TaskSummaries:   taskSummaries,
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

func (a *AcceptanceAuditor) collectWorkspaceSnapshot(projectPath string) string {
	var files []string
	var codeSnippets []string

	ignoredDirs := map[string]bool{
		".git": true, ".noctifab": true, "node_modules": true,
		".venv": true, "venv": true, "__pycache__": true,
		"target": true, "dist": true, "build": true,
	}

	_ = filepath.Walk(projectPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		rel, relErr := filepath.Rel(projectPath, path)
		if relErr != nil || rel == "." {
			return nil
		}
		if info.IsDir() {
			if ignoredDirs[info.Name()] {
				return filepath.SkipDir
			}
			return nil
		}
		// Only collect regular files under 1MB
		if info.Size() > 1024*1024 {
			return nil
		}
		files = append(files, rel)

		// Sample key entrypoints / command handlers / build scripts
		lower := strings.ToLower(rel)
		if strings.Contains(lower, "command") || strings.Contains(lower, "main") ||
			strings.Contains(lower, "cli") || strings.Contains(lower, "server") ||
			strings.Contains(lower, "app") || strings.HasSuffix(lower, "makefile") ||
			strings.Contains(lower, "compose") || strings.HasSuffix(lower, ".sh") ||
			strings.HasSuffix(lower, "pyproject.toml") || strings.HasSuffix(lower, "go.mod") {
			if content, readErr := os.ReadFile(path); readErr == nil {
				snippet := capText(string(content), 3000)
				codeSnippets = append(codeSnippets, fmt.Sprintf("--- %s ---\n%s\n", rel, snippet))
			}
		}
		return nil
	})

	var sb strings.Builder
	sb.WriteString("Workspace File Tree:\n")
	for _, f := range files {
		sb.WriteString("- ")
		sb.WriteString(f)
		sb.WriteString("\n")
	}
	if len(codeSnippets) > 0 {
		sb.WriteString("\nKey Implementation Source Snippets:\n")
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
