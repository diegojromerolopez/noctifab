package services

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/diegojromerolopez/noctifab/pkg/domain"
)

// pythonSyntaxCheckTimeout bounds each py_compile invocation so a hung
// interpreter cannot block file write/edit tools indefinitely.
const pythonSyntaxCheckTimeout = 10 * time.Second

func checkPythonSyntax(ctx context.Context, path string) error {
	if !strings.HasSuffix(path, ".py") {
		return nil
	}
	checkCtx, cancel := context.WithTimeout(ctx, pythonSyntaxCheckTimeout)
	defer cancel()
	// Try running python3 -m py_compile
	cmd := exec.CommandContext(checkCtx, "python3", "-m", "py_compile", path)
	if out, err := cmd.CombinedOutput(); err != nil {
		// Fallback to python
		cmdFallback := exec.CommandContext(checkCtx, "python", "-m", "py_compile", path)
		if outFallback, errFallback := cmdFallback.CombinedOutput(); errFallback != nil {
			errMsg := string(outFallback)
			if len(errMsg) == 0 {
				errMsg = string(out)
			}
			return fmt.Errorf("python syntax compilation failed:\n%s", errMsg)
		}
	}
	return nil
}

// resolveSandboxPath checks prefix path jail and blacklists .noctifab
func resolveSandboxPath(projectPath, targetPath string) (string, error) {
	targetPath = strings.TrimSpace(targetPath)
	targetPath = strings.ReplaceAll(targetPath, "\\", "/")
	targetPath = strings.TrimPrefix(targetPath, "./")

	var absPath string
	if filepath.IsAbs(targetPath) {
		absPath = filepath.Clean(targetPath)
	} else {
		absPath = filepath.Clean(filepath.Join(projectPath, targetPath))
	}
	cleanProj := filepath.Clean(projectPath)

	if !strings.HasPrefix(absPath, cleanProj) {
		return "", fmt.Errorf("Sandbox violation: path '%s' resolves outside the workspace boundary '%s'", targetPath, projectPath)
	}

	rel, err := filepath.Rel(cleanProj, absPath)
	if err != nil {
		return "", err
	}
	parts := strings.Split(filepath.ToSlash(rel), "/")
	for _, part := range parts {
		if part == ".noctifab" || part == ".git" {
			return "", fmt.Errorf("Sandbox violation: path '%s' targets blacklisted configuration directory", targetPath)
		}
	}

	return absPath, nil
}

// ReadFileTool implements read_file.
type ReadFileTool struct{}

func (t *ReadFileTool) Name() string { return "read_file" }
func (t *ReadFileTool) Description() string {
	return "read_file reads the contents of a file in the workspace. Arguments: path (string)."
}
func (t *ReadFileTool) Execute(ctx context.Context, state *domain.State, args map[string]any) (string, error) {
	path, ok := args["path"].(string)
	if !ok || path == "" {
		return "", errors.New("missing or invalid 'path' argument")
	}
	fullPath, err := resolveSandboxPath(state.ProjectPath, path)
	if err != nil {
		return "", err
	}
	content, err := os.ReadFile(fullPath)
	if err != nil {
		return "", err
	}
	return string(content), nil
}

// WriteFileTool implements write_file.
type WriteFileTool struct{}

func (t *WriteFileTool) Name() string { return "write_file" }
func (t *WriteFileTool) Description() string {
	return "write_file creates or replaces a file in the workspace. Arguments: path (string), content (string)."
}
func (t *WriteFileTool) Execute(ctx context.Context, state *domain.State, args map[string]any) (string, error) {
	path, ok := args["path"].(string)
	if !ok || path == "" {
		return "", errors.New("missing or invalid 'path' argument")
	}
	content, ok := args["content"].(string)
	if !ok {
		return "", errors.New("missing or invalid 'content' argument")
	}
	fullPath, err := resolveSandboxPath(state.ProjectPath, path)
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(filepath.Dir(fullPath), 0755); err != nil {
		return "", err
	}
	content = normalizeMakefileTabs(path, content)
	if err := os.WriteFile(fullPath, []byte(content), 0644); err != nil {
		return "", err
	}
	if err := checkPythonSyntax(ctx, fullPath); err != nil {
		return "", err
	}
	return "File written successfully", nil
}

// ReplacementChunk defines a single replace instruction for edit_file.
type ReplacementChunk struct {
	StartLine          int
	EndLine            int
	TargetContent      string
	ReplacementContent string
}

// EditFileTool implements edit_file.
type EditFileTool struct{}

func (t *EditFileTool) Name() string { return "edit_file" }
func (t *EditFileTool) Description() string {
	return "edit_file replaces target_content with replacement_content in a specific line range. Arguments: path (string), edits (array of ReplacementChunk: start_line (int), end_line (int), target_content (string), replacement_content (string))."
}
func (t *EditFileTool) Execute(ctx context.Context, state *domain.State, args map[string]any) (string, error) {
	path, ok := args["path"].(string)
	if !ok || path == "" {
		return "", errors.New("missing or invalid 'path' argument")
	}
	var edits []ReplacementChunk
	editsRaw, ok := args["edits"].([]any)
	if ok {
		for _, eVal := range editsRaw {
			m, ok := eVal.(map[string]any)
			if !ok {
				return "", errors.New("invalid ReplacementChunk structure")
			}
			var chunk ReplacementChunk
			if sl, ok := m["start_line"].(float64); ok {
				chunk.StartLine = int(sl)
			} else if slInt, ok := m["start_line"].(int); ok {
				chunk.StartLine = slInt
			}
			if el, ok := m["end_line"].(float64); ok {
				chunk.EndLine = int(el)
			} else if elInt, ok := m["end_line"].(int); ok {
				chunk.EndLine = elInt
			}
			chunk.TargetContent, _ = m["target_content"].(string)
			chunk.ReplacementContent, _ = m["replacement_content"].(string)
			edits = append(edits, chunk)
		}
	} else {
		target, ok1 := args["target_content"].(string)
		replacement, ok2 := args["replacement_content"].(string)
		if !ok1 || !ok2 {
			return "", errors.New("missing or invalid 'edits' or direct 'target_content'/'replacement_content' arguments")
		}
		edits = append(edits, ReplacementChunk{
			StartLine:          1,
			EndLine:            999999,
			TargetContent:      target,
			ReplacementContent: replacement,
		})
	}

	fullPath, err := resolveSandboxPath(state.ProjectPath, path)
	if err != nil {
		return "", err
	}

	contentBytes, err := os.ReadFile(fullPath)
	if err != nil {
		return "", err
	}

	content := string(contentBytes)
	lines := strings.Split(content, "\n")

	for _, edit := range edits {
		start := edit.StartLine
		end := edit.EndLine
		if start < 1 {
			start = 1
		}
		if end > len(lines) {
			end = len(lines)
		}
		if start > end {
			return "", fmt.Errorf("invalid line range %d-%d", start, end)
		}

		// slice lines is 0-indexed, start/end are 1-indexed
		targetSlice := lines[start-1 : end]
		targetJoined := strings.Join(targetSlice, "\n")

		if !strings.Contains(targetJoined, edit.TargetContent) {
			return "", fmt.Errorf(
				"edit_file failed: target_content not found in file (range %d-%d). "+
					"The file content may have changed since you last read it. "+
					"Call read_file first to get the current content, then retry edit_file with the exact matching target_content, "+
					"or use write_file to overwrite the entire file with the corrected content",
				edit.StartLine, edit.EndLine,
			)
		}

		replacedJoined := strings.Replace(targetJoined, edit.TargetContent, edit.ReplacementContent, 1)
		replacedLines := strings.Split(replacedJoined, "\n")

		// Reassemble lines
		newLines := append([]string{}, lines[:start-1]...)
		newLines = append(newLines, replacedLines...)
		newLines = append(newLines, lines[end:]...)
		lines = newLines
	}

	newContent := normalizeMakefileTabs(path, strings.Join(lines, "\n"))
	if err := os.WriteFile(fullPath, []byte(newContent), 0644); err != nil {
		return "", err
	}
	if err := checkPythonSyntax(ctx, fullPath); err != nil {
		return "", err
	}
	return "Edits applied successfully", nil
}

func isPathExcluded(rel string, excludePaths []string) bool {
	parts := strings.Split(rel, string(filepath.Separator))
	for _, part := range parts {
		if part == ".noctifab" || part == ".git" {
			return true
		}
		for _, exp := range excludePaths {
			cleanExp := strings.Trim(exp, "/")
			if cleanExp != "" && part == cleanExp {
				return true
			}
		}
	}
	return false
}

// ListDirectoryTool implements list_directory.
type ListDirectoryTool struct {
	ExcludePaths []string
}

func (t *ListDirectoryTool) Name() string { return "list_directory" }
func (t *ListDirectoryTool) Description() string {
	return "list_directory lists contents of a workspace folder. Arguments: path (string)."
}
func (t *ListDirectoryTool) Execute(ctx context.Context, state *domain.State, args map[string]any) (string, error) {
	path, ok := args["path"].(string)
	if !ok {
		path = "."
	}
	fullPath, err := resolveSandboxPath(state.ProjectPath, path)
	if err != nil {
		return "", err
	}

	entries, err := os.ReadDir(fullPath)
	if err != nil {
		return "", err
	}

	var sb strings.Builder
	for _, entry := range entries {
		name := entry.Name()
		if name == ".noctifab" || name == ".git" {
			continue
		}
		ignored := false
		for _, exp := range t.ExcludePaths {
			cleanExp := strings.Trim(exp, "/")
			if cleanExp != "" && name == cleanExp {
				ignored = true
				break
			}
		}
		if ignored {
			continue
		}

		info, err := entry.Info()
		if err != nil {
			continue
		}
		typeStr := "F"
		if entry.IsDir() {
			typeStr = "D"
		}
		fmt.Fprintf(&sb, "%s\t%d\t%s\n", typeStr, info.Size(), name)
	}
	return sb.String(), nil
}

// RunTestsTool implements run_tests by delegating execution to the active Sandbox engine.
type RunTestsTool struct {
	Runner  Sandbox
	Timeout time.Duration
}

func (t *RunTestsTool) Name() string { return "run_tests" }
func (t *RunTestsTool) Description() string {
	return "run_tests runs a test suite in the sandbox workspace. Arguments: package (string), command (optional, string)."
}
func (t *RunTestsTool) Execute(ctx context.Context, state *domain.State, args map[string]any) (string, error) {
	pkg, _ := args["package"].(string)
	command, _ := args["command"].(string)

	if t.Runner == nil {
		return "", errors.New("no sandbox execution engine registered")
	}

	timeout := t.Timeout
	if timeout <= 0 {
		timeout = 5 * time.Minute
	}
	runCtx, runCancel := context.WithTimeout(ctx, timeout)
	defer runCancel()
	return t.Runner.RunCommand(runCtx, state.ProjectPath, command, pkg)
}

// countLinterIssues counts the number of distinct linter issue lines in the
// linter output. It counts lines that look like error/warning diagnostics
// (start with a file path or contain an issue code pattern like E001, W123).
func countLinterIssues(output string) int {
	count := 0
	for _, line := range strings.Split(output, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		// Match common linter output patterns:
		// - Lines with file:line:col: message (ruff, golangci, flake8, eslint)
		// - Lines starting with a letter+digits error code (e.g. E501, W0123, I001)
		if strings.Contains(trimmed, ":") && !strings.HasPrefix(trimmed, "#") && !strings.HasPrefix(trimmed, "make") && !strings.HasPrefix(trimmed, "Found") && !strings.HasPrefix(trimmed, "[*]") && !strings.HasPrefix(trimmed, "help:") && !strings.HasPrefix(trimmed, "|") {
			// Heuristic: line contains a colon and is not a header/summary line
			count++
		}
	}
	return count
}

// RunLinterTool implements run_linter.
type RunLinterTool struct {
	Runner           Sandbox
	LinterCommand    string
	FormatterCommand string
	Timeout          time.Duration
	// MaxLinterIssues is the maximum number of linter issues tolerated before
	// the tool returns an error. 0 means zero tolerance. -1 means disabled
	// (never return an error for linter failures). Default 0 (strict).
	MaxLinterIssues int
}

func (t *RunLinterTool) Name() string { return "run_linter" }
func (t *RunLinterTool) Description() string {
	return "run_linter runs the project's linter check in the sandbox workspace to verify syntax and style. Args: {}"
}
func (t *RunLinterTool) Execute(ctx context.Context, state *domain.State, args map[string]any) (string, error) {
	if t.Runner == nil {
		return "", errors.New("no sandbox execution engine registered")
	}
	if t.LinterCommand == "" {
		return "No linter command configured for this project.", nil
	}
	timeout := t.Timeout
	if timeout <= 0 {
		timeout = 5 * time.Minute
	}
	runCtx, runCancel := context.WithTimeout(ctx, timeout)
	defer runCancel()

	// Auto-fix pre-step: automatically run formatter / auto-fixer command before running linter diagnostics
	if t.FormatterCommand != "" {
		if _, err := t.Runner.RunCommand(runCtx, state.ProjectPath, t.FormatterCommand, ""); err != nil {
			fmt.Fprintf(os.Stderr, "⚠ Formatter auto-fix (%s) failed and was skipped: %v\n", t.FormatterCommand, err)
		}
	}

	out, err := t.Runner.RunCommand(runCtx, state.ProjectPath, t.LinterCommand, "")
	if err == nil {
		return out, nil
	}

	// Linter failed: apply MaxLinterIssues threshold.
	// -1 means disabled (never fail on linter).
	if t.MaxLinterIssues < 0 {
		fmt.Fprintf(os.Stderr, "⚠ Linter reported issues but max_linter_issues=-1 (disabled); suppressing error.\n")
		return fmt.Sprintf("[LINTER ADVISORY — issues suppressed by max_linter_issues=-1]\n%s", out), nil
	}

	if t.MaxLinterIssues > 0 {
		issueCount := countLinterIssues(out)
		if issueCount <= t.MaxLinterIssues {
			fmt.Fprintf(os.Stderr, "⚠ Linter found %d issue(s) (threshold: %d) — within tolerance, treating as advisory warning.\n", issueCount, t.MaxLinterIssues)
			return fmt.Sprintf("[LINTER ADVISORY — %d issue(s) within max_linter_issues=%d threshold]\n%s", issueCount, t.MaxLinterIssues, out), nil
		}
		fmt.Fprintf(os.Stderr, "⚠ Linter found %d issue(s) which exceeds max_linter_issues=%d threshold.\n", issueCount, t.MaxLinterIssues)
	}

	return out, err
}

// RequestTestFixTool implements request_test_fix.
type RequestTestFixTool struct{}

func (t *RequestTestFixTool) Name() string { return "request_test_fix" }
func (t *RequestTestFixTool) Description() string {
	return "request_test_fix requests the Tester Agent to fix a bug in the test code. Arguments: feedback (string)."
}
func (t *RequestTestFixTool) Execute(ctx context.Context, state *domain.State, args map[string]any) (string, error) {
	feedback, _ := args["feedback"].(string)
	if feedback == "" {
		return "", errors.New("missing 'feedback' argument")
	}
	return fmt.Sprintf("Test fix requested: %s", feedback), nil
}
