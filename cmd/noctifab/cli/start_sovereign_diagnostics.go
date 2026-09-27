package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/diegojromerolopez/noctifab/pkg/domain"
	"github.com/diegojromerolopez/noctifab/pkg/infrastructure/config"
	"github.com/diegojromerolopez/noctifab/pkg/infrastructure/telemetry"
	"github.com/diegojromerolopez/noctifab/pkg/services"
)

var (
	pyTraceRegex     = regexp.MustCompile(`File "([^"]+)", line (\d+)`)
	generalLineRegex = regexp.MustCompile(`([a-zA-Z0-9_\-\./]+\.(?:py|go|rs|ts|js|c|cpp|h|toml|json)):(\d+)`)
	pyImportRegex    = regexp.MustCompile(`cannot import name '([^']+)' from '([^']+)'`)
)

// SovereignDiagnosticBundle encapsulates compiled failure logs, traces,
// gaps, and offending code artifacts to send directly to the LLM.
type SovereignDiagnosticBundle struct {
	ValidationOutput string
	TaskFailures     []string
	AcceptanceGaps   []string
	FailedStories    []string
	OffendingFiles   map[string]string
	ActionErrors     []string
	TelemetrySpans   string
}

// CollectSovereignDiagnostics inspects the workspace, state tasks, validation
// traces, and error logs to extract all concrete diagnostic data and offending
// source code snippets into a unified diagnostic report.
func CollectSovereignDiagnostics(targetDir string, state *domain.State, failedStories, acceptanceGaps []string, validationOutput string, telemetryInject ...bool) string {
	return CollectSovereignDiagnosticsWithConfig(targetDir, state, failedStories, acceptanceGaps, validationOutput, config.ContextConfig{Mode: "diff_window", WindowSize: 30}, 0, telemetryInject...)
}

// CollectSovereignDiagnosticsWithWindow applies an optional sliding window character budget to validation output.
// If slidingWindow <= 0, no sliding window or truncation is performed, retaining full raw logs.
func CollectSovereignDiagnosticsWithWindow(targetDir string, state *domain.State, failedStories, acceptanceGaps []string, validationOutput string, slidingWindow int, telemetryInject ...bool) string {
	return CollectSovereignDiagnosticsWithConfig(targetDir, state, failedStories, acceptanceGaps, validationOutput, config.ContextConfig{Mode: "diff_window", WindowSize: 30}, slidingWindow, telemetryInject...)
}

// CollectSovereignDiagnosticsWithConfig applies a context configuration (windowing schema, AST/tree_sitter mode)
// and sliding window character budget to validation output, task failure logs, and offending file extractions.
func CollectSovereignDiagnosticsWithConfig(targetDir string, state *domain.State, failedStories, acceptanceGaps []string, validationOutput string, ctxCfg config.ContextConfig, slidingWindow int, telemetryInject ...bool) string {
	inject := len(telemetryInject) > 0 && telemetryInject[0]
	var bundle SovereignDiagnosticBundle

	trimmedValidation := strings.TrimSpace(validationOutput)
	if slidingWindow > 0 && len(trimmedValidation) > slidingWindow {
		trimmedValidation = "... [log truncated by sovereign rescue sliding window] ...\n" + trimmedValidation[len(trimmedValidation)-slidingWindow:]
	}
	bundle.ValidationOutput = trimmedValidation
	bundle.AcceptanceGaps = acceptanceGaps
	bundle.FailedStories = failedStories
	bundle.OffendingFiles = make(map[string]string)

	var allLogsBuilder strings.Builder
	if bundle.ValidationOutput != "" {
		allLogsBuilder.WriteString(bundle.ValidationOutput)
		allLogsBuilder.WriteString("\n")
	}

	var relevantTargetFiles []string

	// 1. Collect failed task traces from state
	if state != nil {
		for _, t := range state.Tasks {
			if t.Status == domain.TaskFailed || strings.TrimSpace(t.FailureLog) != "" {
				relevantTargetFiles = append(relevantTargetFiles, t.TargetFiles...)
				trimmedLog := strings.TrimSpace(t.FailureLog)
				// Limit individual task failure logs to avoid context blowup
				maxTaskLog := 1200
				if len(trimmedLog) > maxTaskLog {
					trimmedLog = "... [log truncated] ...\n" + trimmedLog[len(trimmedLog)-maxTaskLog:]
				}
				msg := fmt.Sprintf("Task [%s] %q (Status: %s):\n%s", t.ID, t.Title, t.Status, trimmedLog)
				bundle.TaskFailures = append(bundle.TaskFailures, msg)
				allLogsBuilder.WriteString(msg)
				allLogsBuilder.WriteString("\n")
			}
		}

		// Collect recent failed actions and diagnostic probe outputs from state
		for _, act := range state.LastActions {
			if !act.Success {
				bundle.ActionErrors = append(bundle.ActionErrors, fmt.Sprintf("- [%s] Tool %s failed: %s (Output: %s)", act.Timestamp.Format("15:04:05"), act.Tool, act.Reasoning, act.Result))
			} else if act.Tool == "check_socket" || act.Tool == "check_http" || act.Tool == "validate_manifest" {
				bundle.ActionErrors = append(bundle.ActionErrors, fmt.Sprintf("- [%s] Diagnostic Probe %s succeeded: %s", act.Timestamp.Format("15:04:05"), act.Tool, act.Result))
			}
		}
	}

	combinedLogs := allLogsBuilder.String()

	// 3. Discover offending source code files mentioned in error traces or failed task targets
	discoveredFiles := extractOffendingFilePaths(targetDir, combinedLogs, relevantTargetFiles)
	count := 0
	for filePath, targetLine := range discoveredFiles {
		if count >= 6 {
			break
		}
		snippet := extractFileSnippetWithConfig(targetDir, filePath, targetLine, ctxCfg)
		if snippet != "" {
			rel, _ := filepath.Rel(targetDir, filePath)
			if rel == "" {
				rel = filePath
			}
			bundle.OffendingFiles[rel] = snippet
			count++
		}
	}

	// 4. Collect recent OpenTelemetry workflow and subprocess spans only when sandbox.telemetry.inject is true
	if inject && targetDir != "" {
		if spans, err := telemetry.ReadRecentSpans(targetDir, 15); err == nil && len(spans) > 0 {
			bundle.TelemetrySpans = telemetry.FormatSpansForPrompt(spans)
		}
	}

	return formatDiagnosticBundle(&bundle)
}

func extractOffendingFilePaths(targetDir string, logs string, targetFiles []string) map[string]int {
	offending := make(map[string]int)

	// Python tracebacks
	for _, match := range pyTraceRegex.FindAllStringSubmatch(logs, -1) {
		if len(match) > 2 {
			p := match[1]
			line, _ := strconv.Atoi(match[2])
			if isProjectFile(targetDir, p) {
				resolved := resolveProjectPath(targetDir, p)
				if resolved != "" {
					offending[resolved] = line
				}
			}
		}
	}

	// General file:line traces
	for _, match := range generalLineRegex.FindAllStringSubmatch(logs, -1) {
		if len(match) > 2 {
			p := match[1]
			line, _ := strconv.Atoi(match[2])
			if isProjectFile(targetDir, p) {
				resolved := resolveProjectPath(targetDir, p)
				if resolved != "" {
					offending[resolved] = line
				}
			}
		}
	}

	// Python import errors (e.g. from 'src.resp')
	for _, match := range pyImportRegex.FindAllStringSubmatch(logs, -1) {
		if len(match) > 2 {
			modulePath := match[2]
			if filepath.IsAbs(modulePath) && isProjectFile(targetDir, modulePath) {
				offending[modulePath] = 1
			}
		}
	}

	// Check explicit task target files (only if not already found in traces)
	for _, tf := range targetFiles {
		full := filepath.Join(targetDir, tf)
		if _, exists := offending[full]; !exists {
			if info, err := os.Stat(full); err == nil && !info.IsDir() {
				offending[full] = 1
			}
		}
	}

	return offending
}

func isProjectFile(targetDir, p string) bool {
	if strings.Contains(p, "/lib/python") || strings.Contains(p, "/site-packages/") ||
		strings.Contains(p, "/usr/") || strings.Contains(p, "/.asdf/") ||
		strings.Contains(p, "/.rustup/") || strings.Contains(p, "/node_modules/") {
		return false
	}
	if filepath.IsAbs(p) {
		return strings.HasPrefix(p, targetDir)
	}
	return true
}

func resolveProjectPath(targetDir, p string) string {
	if filepath.IsAbs(p) {
		if _, err := os.Stat(p); err == nil {
			return p
		}
		return ""
	}
	full := filepath.Join(targetDir, p)
	if _, err := os.Stat(full); err == nil {
		return full
	}
	return ""
}

func extractFileSnippet(fullPath string, targetLine int) string {
	return extractFileSnippetWithConfig("", fullPath, targetLine, config.ContextConfig{Mode: "diff_window", WindowSize: 30})
}

func extractFileSnippetWithConfig(targetDir, fullPath string, targetLine int, ctxCfg config.ContextConfig) string {
	content, err := os.ReadFile(fullPath)
	if err != nil {
		return ""
	}

	rel, _ := filepath.Rel(targetDir, fullPath)
	if rel == "" {
		rel = fullPath
	}

	if ctxCfg.GetMode() == config.ContextModeTreeSitter {
		slicer := services.NewContextSlicer(ctxCfg)
		return slicer.SliceFileContext(rel, string(content), "")
	}

	lines := strings.Split(string(content), "\n")
	if len(lines) == 0 {
		return "(empty file)"
	}

	windowLines := ctxCfg.GetWindowLines()
	if windowLines <= 0 {
		windowLines = 30
	}

	// If file is short (<= windowLines*2), provide the entire file
	if len(lines) <= windowLines*2 {
		var sb strings.Builder
		for i, line := range lines {
			marker := "  "
			if targetLine > 0 && i+1 == targetLine {
				marker = ">>"
			}
			fmt.Fprintf(&sb, "%s %3d | %s\n", marker, i+1, line)
		}
		return sb.String()
	}

	// Windowing schema: slice around targetLine
	start := targetLine - windowLines/2
	if targetLine <= 0 {
		start = 0
	}
	if start < 0 {
		start = 0
	}
	end := start + windowLines
	if end > len(lines) {
		end = len(lines)
		start = end - windowLines
		if start < 0 {
			start = 0
		}
	}

	var sb strings.Builder
	if start > 0 {
		fmt.Fprintf(&sb, "... [%d lines omitted before] ...\n", start)
	}
	for i := start; i < end; i++ {
		marker := "  "
		if targetLine > 0 && i+1 == targetLine {
			marker = ">>"
		}
		fmt.Fprintf(&sb, "%s %3d | %s\n", marker, i+1, lines[i])
	}
	if end < len(lines) {
		fmt.Fprintf(&sb, "... [%d lines omitted after] ...\n", len(lines)-end)
	}
	return sb.String()
}

func formatDiagnosticBundle(b *SovereignDiagnosticBundle) string {
	var sb strings.Builder

	if len(b.TaskFailures) > 0 {
		sb.WriteString("### 1. FAILED ROADMAP TASKS & TRACES\n")
		for _, tf := range b.TaskFailures {
			sb.WriteString(tf)
			sb.WriteString("\n\n")
		}
	}

	if b.ValidationOutput != "" {
		sb.WriteString("### 2. VALIDATION & COMPILER FAILURE OUTPUT\n")
		sb.WriteString(b.ValidationOutput)
		sb.WriteString("\n\n")
	}

	if len(b.AcceptanceGaps) > 0 {
		sb.WriteString("### 3. WHOLE-PROJECT ACCEPTANCE AUDIT GAPS\n")
		for _, gap := range b.AcceptanceGaps {
			fmt.Fprintf(&sb, "- %s\n", gap)
		}
		sb.WriteString("\n")
	}

	if len(b.FailedStories) > 0 {
		sb.WriteString("### 4. INCOMPLETE ROADMAP STORIES\n")
		for _, s := range b.FailedStories {
			fmt.Fprintf(&sb, "- %s\n", s)
		}
		sb.WriteString("\n")
	}

	if len(b.OffendingFiles) > 0 {
		sb.WriteString("### 5. OFFENDING SOURCE CODE EXTRACTS (From Failure Traces)\n")
		for rel, snippet := range b.OffendingFiles {
			fmt.Fprintf(&sb, "--- %s ---\n%s\n\n", rel, snippet)
		}
	}

	if len(b.ActionErrors) > 0 {
		sb.WriteString("### 6. RECENT EXECUTION ACTIONS & ERRORS\n")
		for _, err := range b.ActionErrors {
			sb.WriteString(err)
			sb.WriteString("\n")
		}
		sb.WriteString("\n")
	}

	if b.TelemetrySpans != "" {
		sb.WriteString("### 7. RECENT OPENTELEMETRY WORKFLOW & SUBPROCESS SPANS\n")
		sb.WriteString(b.TelemetrySpans)
		sb.WriteString("\n")
	}

	res := strings.TrimSpace(sb.String())
	if res == "" {
		return "No explicit error traces recorded; consult SPEC.md to fulfill missing functionality."
	}
	return res
}
