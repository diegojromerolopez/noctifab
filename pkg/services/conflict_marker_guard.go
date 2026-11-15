package services

import (
	"bufio"
	"os"
	"path/filepath"
	"strings"
)

// ConflictMarkerViolation represents an unresolved git merge conflict marker.
type ConflictMarkerViolation struct {
	Path    string `json:"path"`
	Line    int    `json:"line"`
	Snippet string `json:"snippet"`
}

// isConflictMarkerLine returns true if the trimmed line is a git merge conflict marker.
func isConflictMarkerLine(trimmed string) bool {
	if strings.HasPrefix(trimmed, "<<<<<<< ") ||
		trimmed == "=======" ||
		strings.HasPrefix(trimmed, ">>>>>>> ") ||
		strings.HasPrefix(trimmed, "||||||| ") {
		return true
	}
	return false
}

// checkConflictMarkerViolations checks the given file lines for unresolved git conflict markers.
func checkConflictMarkerViolations(path string, lines []string) []AntiStubViolation {
	ext := strings.ToLower(filepath.Ext(path))
	if ext == ".md" || ext == ".markdown" || ext == ".txt" {
		return nil
	}

	var violations []AntiStubViolation
	for idx, line := range lines {
		trimmed := strings.TrimSpace(line)
		if isConflictMarkerLine(trimmed) {
			violations = append(violations, AntiStubViolation{
				Path:    path,
				Line:    idx + 1,
				Rule:    "git_merge_conflict_marker",
				Snippet: trimmed,
			})
		}
	}
	return violations
}

// DetectConflictMarkersInWorkspace scans all workspace source files for unresolved conflict markers.
func DetectConflictMarkersInWorkspace(projectPath string) ([]ConflictMarkerViolation, error) {
	var violations []ConflictMarkerViolation
	if projectPath == "" {
		return violations, nil
	}

	err := filepath.Walk(projectPath, func(path string, info os.FileInfo, walkErr error) error {
		if walkErr != nil {
			return nil
		}
		rel, rErr := filepath.Rel(projectPath, path)
		if rErr != nil {
			return nil
		}
		if info.IsDir() {
			if IsPathExcluded(rel, nil) {
				return filepath.SkipDir
			}
			return nil
		}
		if IsPathExcluded(rel, nil) {
			return nil
		}

		ext := strings.ToLower(filepath.Ext(path))
		if ext == ".md" || ext == ".markdown" || ext == ".txt" || ext == ".png" || ext == ".ico" {
			return nil
		}
		if info.Size() > 1024*1024 {
			return nil
		}

		file, oErr := os.Open(path)
		if oErr != nil {
			return nil
		}
		defer func() { _ = file.Close() }()

		scanner := bufio.NewScanner(file)
		lineNum := 1
		for scanner.Scan() {
			trimmed := strings.TrimSpace(scanner.Text())
			if isConflictMarkerLine(trimmed) {
				violations = append(violations, ConflictMarkerViolation{
					Path:    rel,
					Line:    lineNum,
					Snippet: trimmed,
				})
			}
			lineNum++
		}
		return nil
	})

	return violations, err
}
