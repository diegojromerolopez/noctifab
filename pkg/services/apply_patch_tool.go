package services

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/diegojromerolopez/noctifab/pkg/domain"
)

var hunkHeaderRegexp = regexp.MustCompile(`^@@ -(\d+)(?:,(\d+))? \+(\d+)(?:,(\d+))? @@`)

// ApplyPatchTool implements apply_patch for single- and multi-file unified diff patching.
type ApplyPatchTool struct{}

func (t *ApplyPatchTool) Name() string { return "apply_patch" }
func (t *ApplyPatchTool) Description() string {
	return "apply_patch applies a unified diff patch string (Git / diff -u format) to one or more files in the workspace. Arguments: patch (string, required), path (optional string)."
}

type diffFileHeader struct {
	OldPath string
	NewPath string
}

type diffHunkLine struct {
	Kind byte // ' ', '-', '+'
	Text string
}

type diffHunk struct {
	OldStart int
	OldCount int
	NewStart int
	NewCount int
	Lines    []diffHunkLine
}

type fileDiff struct {
	Header diffFileHeader
	Hunks  []diffHunk
}

func (t *ApplyPatchTool) Execute(ctx context.Context, state *domain.State, args map[string]any) (string, error) {
	patchStr, ok := args["patch"].(string)
	if !ok || strings.TrimSpace(patchStr) == "" {
		return "", errors.New("missing or invalid 'patch' argument")
	}

	fallbackPath, _ := args["path"].(string)
	diffs, err := parseUnifiedDiff(patchStr, fallbackPath)
	if err != nil {
		return "", fmt.Errorf("failed to parse patch: %w", err)
	}

	if len(diffs) == 0 {
		return "", errors.New("patch string contains no valid file diffs or hunks")
	}

	var modifiedFiles []string
	for _, diff := range diffs {
		targetRel := cleanDiffPath(diff.Header.NewPath)
		if targetRel == "" || targetRel == "/dev/null" {
			targetRel = cleanDiffPath(diff.Header.OldPath)
		}

		if targetRel == "" || targetRel == "/dev/null" {
			return "", errors.New("patch target file path cannot be resolved")
		}

		fullPath, err := resolveSandboxPath(state.ProjectPath, targetRel)
		if err != nil {
			return "", err
		}

		// Handle file deletion if new path is /dev/null
		if cleanDiffPath(diff.Header.NewPath) == "/dev/null" {
			if err := os.Remove(fullPath); err != nil && !os.IsNotExist(err) {
				return "", fmt.Errorf("failed to delete file %s: %w", targetRel, err)
			}
			modifiedFiles = append(modifiedFiles, targetRel+" (deleted)")
			continue
		}

		// Read existing content or start fresh if new file
		var existingLines []string
		if contentBytes, err := os.ReadFile(fullPath); err == nil {
			existingLines = strings.Split(string(contentBytes), "\n")
		}

		patchedLines, err := applyFileDiff(existingLines, diff)
		if err != nil {
			return "", fmt.Errorf("failed to apply patch to %s: %w", targetRel, err)
		}

		if err := os.MkdirAll(filepath.Dir(fullPath), 0755); err != nil {
			return "", fmt.Errorf("failed to create directory for %s: %w", targetRel, err)
		}

		newContent := strings.Join(patchedLines, "\n")
		if err := os.WriteFile(fullPath, []byte(newContent), 0644); err != nil {
			return "", fmt.Errorf("failed to write patched file %s: %w", targetRel, err)
		}

		if err := checkPythonSyntax(ctx, fullPath); err != nil {
			return "", err
		}

		modifiedFiles = append(modifiedFiles, targetRel)
	}

	return fmt.Sprintf("Patch applied successfully to %d file(s): %s", len(modifiedFiles), strings.Join(modifiedFiles, ", ")), nil
}

func cleanDiffPath(p string) string {
	p = strings.TrimSpace(p)
	if p == "" {
		return ""
	}
	if p == "/dev/null" || strings.HasPrefix(p, "/dev/null") {
		return "/dev/null"
	}
	p = strings.TrimPrefix(p, "a/")
	p = strings.TrimPrefix(p, "b/")
	return filepath.Clean(p)
}

func parseUnifiedDiff(patch string, fallbackPath string) ([]fileDiff, error) {
	patch = strings.ReplaceAll(patch, "\r\n", "\n")
	rawLines := strings.Split(patch, "\n")

	var diffs []fileDiff
	var currentFile *fileDiff
	var currentHunk *diffHunk

	for i := 0; i < len(rawLines); i++ {
		line := rawLines[i]

		if strings.HasPrefix(line, "diff --git ") {
			if currentFile != nil {
				if currentHunk != nil {
					currentFile.Hunks = append(currentFile.Hunks, *currentHunk)
					currentHunk = nil
				}
				diffs = append(diffs, *currentFile)
			}
			currentFile = &fileDiff{}
			continue
		}

		if strings.HasPrefix(line, "--- ") {
			if currentFile != nil && currentFile.Header.OldPath != "" {
				if currentHunk != nil {
					currentFile.Hunks = append(currentFile.Hunks, *currentHunk)
					currentHunk = nil
				}
				diffs = append(diffs, *currentFile)
				currentFile = &fileDiff{}
			} else if currentFile == nil {
				currentFile = &fileDiff{}
			}
			currentFile.Header.OldPath = strings.TrimPrefix(line, "--- ")
			continue
		}

		if strings.HasPrefix(line, "+++ ") {
			if currentFile == nil {
				currentFile = &fileDiff{}
			}
			currentFile.Header.NewPath = strings.TrimPrefix(line, "+++ ")
			continue
		}

		if matches := hunkHeaderRegexp.FindStringSubmatch(line); len(matches) > 0 {
			if currentFile == nil {
				currentFile = &fileDiff{
					Header: diffFileHeader{
						OldPath: fallbackPath,
						NewPath: fallbackPath,
					},
				}
			}
			if currentHunk != nil {
				currentFile.Hunks = append(currentFile.Hunks, *currentHunk)
			}

			oldStart, _ := strconv.Atoi(matches[1])
			oldCount := 1
			if matches[2] != "" {
				oldCount, _ = strconv.Atoi(matches[2])
			}
			newStart, _ := strconv.Atoi(matches[3])
			newCount := 1
			if matches[4] != "" {
				newCount, _ = strconv.Atoi(matches[4])
			}

			currentHunk = &diffHunk{
				OldStart: oldStart,
				OldCount: oldCount,
				NewStart: newStart,
				NewCount: newCount,
			}
			continue
		}

		if currentHunk != nil {
			if strings.HasPrefix(line, "\\ ") {
				// Skip "\ No newline at end of file"
				continue
			}
			if len(line) > 0 && (line[0] == ' ' || line[0] == '-' || line[0] == '+') {
				currentHunk.Lines = append(currentHunk.Lines, diffHunkLine{
					Kind: line[0],
					Text: line[1:],
				})
			} else if line == "" && i == len(rawLines)-1 {
				// Ignore trailing empty line
				continue
			}
		}
	}

	if currentHunk != nil && currentFile != nil {
		currentFile.Hunks = append(currentFile.Hunks, *currentHunk)
	}
	if currentFile != nil && len(currentFile.Hunks) > 0 {
		diffs = append(diffs, *currentFile)
	}

	return diffs, nil
}

func applyFileDiff(existingLines []string, diff fileDiff) ([]string, error) {
	lines := append([]string{}, existingLines...)

	for _, hunk := range diff.Hunks {
		// Extract old context lines from hunk
		var expectedOld []string
		for _, hl := range hunk.Lines {
			if hl.Kind == ' ' || hl.Kind == '-' {
				expectedOld = append(expectedOld, hl.Text)
			}
		}

		// Find target index in lines
		matchIdx := findHunkOffset(lines, expectedOld, hunk.OldStart-1)
		if matchIdx == -1 {
			// If file was empty and old count is 0, start at line 0
			if len(lines) == 0 && hunk.OldStart <= 1 {
				matchIdx = 0
			} else {
				return nil, fmt.Errorf("hunk target context at old line %d not found in file", hunk.OldStart)
			}
		}

		// Build replacement lines
		var newLines []string
		for _, hl := range hunk.Lines {
			if hl.Kind == ' ' || hl.Kind == '+' {
				newLines = append(newLines, hl.Text)
			}
		}

		// Calculate old block length to replace
		oldBlockLen := len(expectedOld)
		if matchIdx+oldBlockLen > len(lines) {
			oldBlockLen = len(lines) - matchIdx
			if oldBlockLen < 0 {
				oldBlockLen = 0
			}
		}

		// Reassemble lines
		result := append([]string{}, lines[:matchIdx]...)
		result = append(result, newLines...)
		if matchIdx+oldBlockLen < len(lines) {
			result = append(result, lines[matchIdx+oldBlockLen:]...)
		}
		lines = result
	}

	return lines, nil
}

func findHunkOffset(lines []string, expectedOld []string, targetIdx int) int {
	if len(expectedOld) == 0 {
		if targetIdx < 0 {
			return 0
		}
		if targetIdx > len(lines) {
			return len(lines)
		}
		return targetIdx
	}

	// 1. Try exact target index
	if matchAt(lines, expectedOld, targetIdx) {
		return targetIdx
	}

	// 2. Fuzzy search within window +/- 15 lines
	window := 15
	for offset := 1; offset <= window; offset++ {
		if targetIdx-offset >= 0 && matchAt(lines, expectedOld, targetIdx-offset) {
			return targetIdx - offset
		}
		if targetIdx+offset <= len(lines)-len(expectedOld) && matchAt(lines, expectedOld, targetIdx+offset) {
			return targetIdx + offset
		}
	}

	return -1
}

func matchAt(lines []string, expected []string, idx int) bool {
	if idx < 0 || idx+len(expected) > len(lines) {
		return false
	}
	for i := 0; i < len(expected); i++ {
		if lines[idx+i] != expected[i] {
			return false
		}
	}
	return true
}
