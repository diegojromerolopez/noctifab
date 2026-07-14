package services

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/diegojromerolopez/noctifab/pkg/domain"
)

func checkPythonSyntax(path string) error {
	if !strings.HasSuffix(path, ".py") {
		return nil
	}
	// Try running python3 -m py_compile
	cmd := exec.Command("python3", "-m", "py_compile", path)
	if out, err := cmd.CombinedOutput(); err != nil {
		// Fallback to python
		cmdFallback := exec.Command("python", "-m", "py_compile", path)
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
	parts := strings.Split(rel, string(filepath.Separator))
	for _, part := range parts {
		if part == ".noctifab" {
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
	if err := os.WriteFile(fullPath, []byte(content), 0644); err != nil {
		return "", err
	}
	if err := checkPythonSyntax(fullPath); err != nil {
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

	newContent := strings.Join(lines, "\n")
	if err := os.WriteFile(fullPath, []byte(newContent), 0644); err != nil {
		return "", err
	}
	if err := checkPythonSyntax(fullPath); err != nil {
		return "", err
	}
	return "Edits applied successfully", nil
}

// ListDirectoryTool implements list_directory.
type ListDirectoryTool struct{}

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
		info, err := entry.Info()
		if err != nil {
			continue
		}
		typeStr := "F"
		if entry.IsDir() {
			typeStr = "D"
		}
		fmt.Fprintf(&sb, "%s\t%d\t%s\n", typeStr, info.Size(), entry.Name())
	}
	return sb.String(), nil
}

// FindFilesTool implements find_files.
type FindFilesTool struct{}

func (t *FindFilesTool) Name() string { return "find_files" }
func (t *FindFilesTool) Description() string {
	return "find_files finds files matching a pattern. Arguments: pattern (string)."
}
func (t *FindFilesTool) Execute(ctx context.Context, state *domain.State, args map[string]any) (string, error) {
	pattern, ok := args["pattern"].(string)
	if !ok || pattern == "" {
		return "", errors.New("missing or invalid 'pattern' argument")
	}

	var matched []string
	err := filepath.WalkDir(state.ProjectPath, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		rel, err := filepath.Rel(state.ProjectPath, path)
		if err != nil {
			return nil
		}
		if rel == "." || rel == ".." {
			return nil
		}
		// Ignore hidden/excluded paths
		parts := strings.Split(rel, string(filepath.Separator))
		for _, part := range parts {
			if part == ".noctifab" || part == ".git" || part == "node_modules" || part == "vendor" {
				if d.IsDir() {
					return filepath.SkipDir
				}
				return nil
			}
		}

		matchedName, _ := filepath.Match(pattern, d.Name())
		matchedRel, _ := filepath.Match(pattern, rel)
		if matchedName || matchedRel {
			matched = append(matched, rel)
		}
		return nil
	})

	if err != nil {
		return "", err
	}
	return strings.Join(matched, "\n"), nil
}

// GrepSearchTool implements grep_search.
type GrepSearchTool struct{}

func (t *GrepSearchTool) Name() string { return "grep_search" }
func (t *GrepSearchTool) Description() string {
	return "grep_search performs regex query searches over file contents. Arguments: query (string), path (optional, string)."
}
func (t *GrepSearchTool) Execute(ctx context.Context, state *domain.State, args map[string]any) (string, error) {
	query, ok := args["query"].(string)
	if !ok || query == "" {
		return "", errors.New("missing or invalid 'query' argument")
	}
	path, _ := args["path"].(string)
	if path == "" {
		path = "."
	}

	fullPath, err := resolveSandboxPath(state.ProjectPath, path)
	if err != nil {
		return "", err
	}

	re, err := regexp.Compile(query)
	if err != nil {
		return "", fmt.Errorf("invalid regex query: %w", err)
	}

	var matchedLines []string
	err = filepath.WalkDir(fullPath, func(fPath string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			rel, _ := filepath.Rel(state.ProjectPath, fPath)
			parts := strings.Split(rel, string(filepath.Separator))
			for _, part := range parts {
				if part == ".noctifab" || part == ".git" || part == "node_modules" || part == "vendor" {
					return filepath.SkipDir
				}
			}
			return nil
		}

		contentBytes, err := os.ReadFile(fPath)
		if err != nil {
			return nil
		}
		relPath, _ := filepath.Rel(state.ProjectPath, fPath)

		lines := strings.Split(string(contentBytes), "\n")
		for idx, line := range lines {
			if re.MatchString(line) {
				matchedLines = append(matchedLines, fmt.Sprintf("%s:%d: %s", relPath, idx+1, line))
			}
		}
		return nil
	})

	if err != nil {
		return "", err
	}
	return strings.Join(matchedLines, "\n"), nil
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

// RunLinterTool implements run_linter.
type RunLinterTool struct {
	Runner        Sandbox
	LinterCommand string
	Timeout       time.Duration
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
	return t.Runner.RunCommand(runCtx, state.ProjectPath, t.LinterCommand, "")
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
