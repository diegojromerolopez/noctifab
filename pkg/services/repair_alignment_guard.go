package services

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

var (
	// Python: File "/path/to/file.py", line 42
	pythonTraceRE = regexp.MustCompile(`File\s+["']([^"']+\.[a-zA-Z0-9_-]+)["'],\s*line\s+\d+`)

	// Go / C / Rust: path/to/file.go:42: or --> src/file.rs:42:5
	standardTraceRE = regexp.MustCompile(`(?:-->\s+)?([a-zA-Z0-9_\-\./\\]+\.[a-zA-Z0-9_]+):(\d+)(?::\d+)?`)

	// Node / Java: at Class.method (/path/to/file.js:42:10)
	nodeTraceRE = regexp.MustCompile(`\(([^()]+?\.[a-zA-Z0-9_]+):\d+:\d+\)`)
)

// RepairAlignmentGuard ensures that task repair attempts target the actual offending code
// identified in error tracebacks, preventing sycophantic or no-op repair loops.
type RepairAlignmentGuard struct{}

// NewRepairAlignmentGuard creates a new RepairAlignmentGuard.
func NewRepairAlignmentGuard() *RepairAlignmentGuard {
	return &RepairAlignmentGuard{}
}

// ExtractTracebackFiles parses failure logs across supported languages and extracts
// relative and normalized file paths appearing in stack traces and compiler errors.
func (g *RepairAlignmentGuard) ExtractTracebackFiles(projectPath, logContent string) []string {
	if strings.TrimSpace(logContent) == "" {
		return nil
	}

	foundMap := make(map[string]bool)

	addFile := func(raw string) {
		clean := filepath.Clean(strings.TrimSpace(raw))
		if clean == "" || clean == "." {
			return
		}

		// Normalize if absolute path inside projectPath
		if projectPath != "" && filepath.IsAbs(clean) {
			if rel, err := filepath.Rel(projectPath, clean); err == nil && !strings.HasPrefix(rel, "..") {
				clean = rel
			}
		}

		// Discard standard library or external virtualenv / node_modules / site-packages files
		lower := strings.ToLower(clean)
		if strings.Contains(lower, "site-packages") ||
			strings.Contains(lower, "node_modules") ||
			strings.Contains(lower, "/usr/") ||
			strings.Contains(lower, ".cargo/") ||
			strings.Contains(lower, ".rustup/") {
			return
		}

		foundMap[clean] = true
	}

	for _, m := range pythonTraceRE.FindAllStringSubmatch(logContent, -1) {
		if len(m) > 1 {
			addFile(m[1])
		}
	}

	for _, m := range standardTraceRE.FindAllStringSubmatch(logContent, -1) {
		if len(m) > 1 {
			addFile(m[1])
		}
	}

	for _, m := range nodeTraceRE.FindAllStringSubmatch(logContent, -1) {
		if len(m) > 1 {
			addFile(m[1])
		}
	}

	var results []string
	for f := range foundMap {
		results = append(results, f)
	}
	sort.Strings(results)
	return results
}

// ComputeDiffHash returns a deterministic SHA-256 hash of a git diff to detect repeat attempts.
func (g *RepairAlignmentGuard) ComputeDiffHash(diffContent string) string {
	trimmed := strings.TrimSpace(diffContent)
	if trimmed == "" {
		return ""
	}
	sum := sha256.Sum256([]byte(trimmed))
	return hex.EncodeToString(sum[:])
}

// AlignmentResult contains the verdict of a repair diff evaluation.
type AlignmentResult struct {
	Allowed bool
	Reason  string
}

// ValidateRepairAlignment enforces:
// 1. Zero identical repeat diffs (rejecting recurring hallucinated edits).
// 2. Diff alignment: modifications must intersect with either traceback files or task target files.
func (g *RepairAlignmentGuard) ValidateRepairAlignment(
	modifiedFiles []string,
	tracebackFiles []string,
	targetFiles []string,
	currentDiffHash string,
	previousDiffHashes []string,
) AlignmentResult {
	if len(modifiedFiles) == 0 {
		return AlignmentResult{
			Allowed: false,
			Reason:  "repair attempt produced zero file modifications",
		}
	}

	// 1. Check for identical diff repetition
	if currentDiffHash != "" {
		for _, prev := range previousDiffHashes {
			if prev == currentDiffHash {
				return AlignmentResult{
					Allowed: false,
					Reason:  "repair attempt produced an identical diff to a previously failed attempt (anti-loop gate triggered)",
				}
			}
		}
	}

	// If no traceback files were extracted, allow modifications to task target files
	if len(tracebackFiles) == 0 {
		return AlignmentResult{Allowed: true}
	}

	// Build set of acceptable target and traceback file patterns
	allowedSet := make(map[string]bool)
	for _, tf := range targetFiles {
		clean := filepath.Clean(strings.TrimSpace(tf))
		if clean != "" {
			allowedSet[clean] = true
			allowedSet[filepath.Base(clean)] = true
		}
	}
	for _, tbf := range tracebackFiles {
		clean := filepath.Clean(strings.TrimSpace(tbf))
		if clean != "" {
			allowedSet[clean] = true
			allowedSet[filepath.Base(clean)] = true
		}
	}

	// Verify at least one modified file aligns with the allowed set
	aligned := false
	for _, mf := range modifiedFiles {
		clean := filepath.Clean(strings.TrimSpace(mf))
		if allowedSet[clean] || allowedSet[filepath.Base(clean)] {
			aligned = true
			break
		}
	}

	if !aligned {
		return AlignmentResult{
			Allowed: false,
			Reason: fmt.Sprintf(
				"sycophantic repair rejected: modified files %v do not intersect with error traceback %v or target files %v",
				modifiedFiles, tracebackFiles, targetFiles,
			),
		}
	}

	return AlignmentResult{Allowed: true}
}
