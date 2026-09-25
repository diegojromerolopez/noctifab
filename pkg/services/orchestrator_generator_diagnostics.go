package services

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/diegojromerolopez/noctifab/pkg/domain"
)

// buildFailureDiagnosticsContext parses the failure log and extracts:
// 1. The full failure trace / test runner output (capped to prevent context explosion).
// 2. The specific failing test or error breakdown.
// 3. The actual source code of the failing test files and offending target files.
func buildFailureDiagnosticsContext(task domain.Task, workspaceDir string) string {
	if task.FailureLog == "" {
		return ""
	}

	var sb strings.Builder
	sb.WriteString("### ❌ FAILING TEST & ERROR TRACE CONTEXT\n")
	sb.WriteString("The previous test run failed. Inspect the following error trace, failing test details, and the failing source/test code:\n\n")

	// 1. Structured failure details
	summary := summarizeFailureLog(task.FailureLog)
	if strings.TrimSpace(summary) != "" {
		sb.WriteString("#### Key Error Summary:\n```\n")
		sb.WriteString(summary)
		if !strings.HasSuffix(summary, "\n") {
			sb.WriteString("\n")
		}
		sb.WriteString("```\n\n")
	}

	// 2. Full unsummarized failure trace (capped at 4000 chars)
	sb.WriteString("#### Test Runner Trace / Error Output:\n```\n")
	sb.WriteString(capText(task.FailureLog, 4000))
	if !strings.HasSuffix(task.FailureLog, "\n") {
		sb.WriteString("\n")
	}
	sb.WriteString("```\n\n")

	// 3. Extract diagnostics to identify failing test files and source files
	diagnostics := extractDiagnostics(task.FailureLog)
	seenFiles := make(map[string]bool)
	var failingFilesContent []string

	for _, d := range diagnostics {
		if d.FilePath != "" && !seenFiles[d.FilePath] {
			seenFiles[d.FilePath] = true
			fullPath := filepath.Join(workspaceDir, d.FilePath)
			if content, err := os.ReadFile(fullPath); err == nil && len(content) > 0 {
				failingFilesContent = append(failingFilesContent, fmt.Sprintf("```\n// File: %s\n%s\n```", d.FilePath, capText(string(content), 3000)))
			}
		}
	}

	// Also attach target files that may have failed if not already included
	for _, target := range task.TargetFiles {
		if target != "" && !seenFiles[target] {
			seenFiles[target] = true
			fullPath := filepath.Join(workspaceDir, target)
			if content, err := os.ReadFile(fullPath); err == nil && len(content) > 0 {
				failingFilesContent = append(failingFilesContent, fmt.Sprintf("```\n// File: %s\n%s\n```", target, capText(string(content), 3000)))
			}
		}
	}

	if len(failingFilesContent) > 0 {
		sb.WriteString("#### Failing Source & Test Code:\n")
		sb.WriteString(strings.Join(failingFilesContent, "\n\n"))
		sb.WriteString("\n\n")
	}

	sb.WriteString("Fix the code and tests to address these specific errors. Run tests to verify the fix.")
	return sb.String()
}

// formatTurnDiagnosticFeedback formats test/linter failures within generator turns
// providing structured diagnostics and highlighting failing source lines.
func formatTurnDiagnosticFeedback(toolName string, execErr error, out string) string {
	var sb strings.Builder
	fmt.Fprintf(&sb, "Tool %s failed: %v\n", toolName, execErr)
	summary := summarizeFailureLog(out)
	if strings.TrimSpace(summary) != "" {
		sb.WriteString("Key Failure Details:\n")
		sb.WriteString(summary)
		if !strings.HasSuffix(summary, "\n") {
			sb.WriteString("\n")
		}
	}
	sb.WriteString("\nFull Diagnostic Trace (Capped):\n")
	sb.WriteString(capText(out, 3000))
	return sb.String()
}
