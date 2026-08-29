package services

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/diegojromerolopez/noctifab/pkg/domain"
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
	llmClient domain.LLMClient
	runner    Sandbox
	e2eCmd    string
}

// NewStoryQAAuditor creates a StoryQAAuditor instance.
func NewStoryQAAuditor(client domain.LLMClient, runner ...Sandbox) *StoryQAAuditor {
	var r Sandbox
	if len(runner) > 0 {
		r = runner[0]
	}
	return &StoryQAAuditor{
		llmClient: client,
		runner:    r,
	}
}

// SetRunner attaches a sandbox runner for E2E validation.
func (a *StoryQAAuditor) SetRunner(runner Sandbox) {
	a.runner = runner
}

// SetE2ECommand sets the custom E2E command.
func (a *StoryQAAuditor) SetE2ECommand(cmd string) {
	a.e2eCmd = cmd
}

// AuditStoryCompleteness verifies whether the codebase fulfills all requirements of the active story.
func (a *StoryQAAuditor) AuditStoryCompleteness(ctx context.Context, state *domain.State, storyPath string) (*StoryQAResult, error) {
	if state == nil || strings.TrimSpace(state.ProjectPath) == "" {
		return &StoryQAResult{Passed: true, Summary: "No project path; story QA audit skipped"}, nil
	}

	// 1. E2E Test Execution Gate:
	// If an E2E test command is configured or detected, run it first to verify real integration.
	e2eCmd := a.detectE2ECommand(state.ProjectPath)
	if e2eCmd != "" && a.runner != nil {
		fmt.Printf("🔍 [Story QA] Running E2E test verification command: %q...\n", e2eCmd)
		e2eOut, e2eErr := a.runner.RunCommand(ctx, state.ProjectPath, e2eCmd, "")
		if e2eErr != nil {
			fmt.Printf("❌ [Story QA] E2E verification failed: %v\n", e2eErr)
			return &StoryQAResult{
				Passed:  false,
				Summary: fmt.Sprintf("E2E test suite failed (%s): %v\n%s", e2eCmd, e2eErr, capText(e2eOut, 1500)),
				MissingFeatures: []string{
					fmt.Sprintf("E2E test execution failure (%s): %s", e2eCmd, capText(e2eOut, 500)),
				},
			}, nil
		}
		fmt.Printf("✅ [Story QA] E2E verification passed successfully.\n")
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

	if a.llmClient == nil {
		return &StoryQAResult{Passed: true, Summary: "No LLM client configured; story QA audit skipped"}, nil
	}

	// Collect accumulated git diff and workspace files
	git := NewGitClient(state.ProjectPath)
	baseBranch := ResolveBaseBranch(ctx, git, state.Metadata.BaseBranch)
	diffContext, _ := git.Run(ctx, false, "diff", baseBranch)
	if strings.TrimSpace(diffContext) == "" {
		diffContext, _ = git.Run(ctx, false, "diff", "HEAD~1")
	}

	workspaceSnapshot := a.collectSourceFiles(state.ProjectPath)
	prompt := a.buildPrompt(storyContent, workspaceSnapshot, diffContext, state)

	auditCtx := context.WithValue(ctx, AgentRoleKey, "qa")
	resp, err := a.llmClient.Complete(auditCtx, prompt)
	if err != nil {
		return nil, fmt.Errorf("story QA audit LLM call failed: %w", err)
	}

	return a.parseAuditResponse(resp), nil
}

func (a *StoryQAAuditor) buildPrompt(storyContent, workspaceSnapshot, diffContext string, state *domain.State) string {
	var sb strings.Builder
	sb.WriteString("You are the QA Acceptance Agent. Your task is to verify whether the accumulated generated code and tests completely fulfill all features, acceptance criteria, and Definitions of Done (DoD) defined in the target User Story.\n\n")
	sb.WriteString("TARGET USER STORY:\n```markdown\n")
	sb.WriteString(capText(storyContent, 20000))
	sb.WriteString("\n```\n\n")

	sb.WriteString("WORKSPACE SOURCE FILES:\n```\n")
	sb.WriteString(capText(workspaceSnapshot, 15000))
	sb.WriteString("\n```\n\n")

	if strings.TrimSpace(diffContext) != "" {
		sb.WriteString("ACCUMULATED GIT DIFF:\n```diff\n")
		sb.WriteString(capText(diffContext, 15000))
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
3. If ANY required feature from the user story is missing, incomplete, or omitted, set "passed": false and enumerate each missing item in "missing_features".
4. If all features and acceptance criteria are fully met, set "passed": true and "missing_features": [].

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

func (a *StoryQAAuditor) collectSourceFiles(projectPath string) string {
	var sb strings.Builder
	ignoredDirs := map[string]bool{
		".git": true, ".noctifab": true, "node_modules": true,
		".venv": true, "venv": true, "__pycache__": true,
		"target": true, "dist": true, "build": true,
	}

	_ = filepath.Walk(projectPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if info.IsDir() {
			if ignoredDirs[info.Name()] {
				return filepath.SkipDir
			}
			return nil
		}

		ext := filepath.Ext(path)
		switch ext {
		case ".go", ".py", ".rs", ".js", ".ts", ".c", ".h", ".cpp", ".java", ".rb":
			rel, rErr := filepath.Rel(projectPath, path)
			if rErr == nil {
				fmt.Fprintf(&sb, "--- File: %s ---\n", rel)
				content, readErr := os.ReadFile(path)
				if readErr == nil {
					sb.WriteString(capText(string(content), 3000))
					sb.WriteString("\n\n")
				}
			}
		}
		return nil
	})

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

func (a *StoryQAAuditor) detectE2ECommand(projectPath string) string {
	if a.e2eCmd != "" {
		return a.e2eCmd
	}
	if _, err := os.Stat(filepath.Join(projectPath, "docker-compose.e2e.yml")); err == nil {
		return "docker compose -f docker-compose.e2e.yml up --build --exit-code-from test-runner"
	}
	if _, err := os.Stat(filepath.Join(projectPath, "Makefile")); err == nil {
		content, rErr := os.ReadFile(filepath.Join(projectPath, "Makefile"))
		if rErr == nil && strings.Contains(string(content), "e2e:") {
			return "make e2e"
		}
	}
	return ""
}
