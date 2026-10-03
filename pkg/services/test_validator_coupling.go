package services

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// TestCouplingViolation records an illegal cross-layer import between distinct test tiers.
type TestCouplingViolation struct {
	FilePath         string
	SourceCategory   string // "unit", "integration", "e2e"
	ImportedCategory string // "unit", "integration", "e2e"
	LineNumber       int
	MatchedLine      string
	Reason           string
}

var (
	testCodeExtensions = map[string]bool{
		".py":   true,
		".go":   true,
		".ts":   true,
		".js":   true,
		".tsx":  true,
		".jsx":  true,
		".rs":   true,
		".rb":   true,
		".java": true,
		".kt":   true,
		".cs":   true,
		".c":    true,
		".cpp":  true,
		".h":    true,
	}

	couplingRegexCache = map[string][]*regexp.Regexp{
		"unit": {
			regexp.MustCompile(`(?i)(?:from|import)\s+tests?\.unit\b`),
			regexp.MustCompile(`(?i)(?:from|import)\s+\.+unit\b`),
			regexp.MustCompile(`(?i)['"\x60](?:\.\./|\./|[a-zA-Z0-9_\-\./]+/)?(?:tests?/)?unit(?:/|['"\x60])`),
			regexp.MustCompile(`(?i)tests?::unit\b`),
		},
		"integration": {
			regexp.MustCompile(`(?i)(?:from|import)\s+tests?\.integration\b`),
			regexp.MustCompile(`(?i)(?:from|import)\s+\.+integration\b`),
			regexp.MustCompile(`(?i)['"\x60](?:\.\./|\./|[a-zA-Z0-9_\-\./]+/)?(?:tests?/)?integration(?:/|['"\x60])`),
			regexp.MustCompile(`(?i)tests?::integration\b`),
		},
		"e2e": {
			regexp.MustCompile(`(?i)(?:from|import)\s+tests?\.e2e\b`),
			regexp.MustCompile(`(?i)(?:from|import)\s+\.+e2e\b`),
			regexp.MustCompile(`(?i)['"\x60](?:\.\./|\./|[a-zA-Z0-9_\-\./]+/)?(?:tests?/)?e2e(?:/|['"\x60])`),
			regexp.MustCompile(`(?i)tests?::e2e\b`),
		},
	}
)

// DetectTestCouplingViolations scans the test directories within projectPath
// to verify hermetic test layer isolation.
func DetectTestCouplingViolations(projectPath string) []TestCouplingViolation {
	if projectPath == "" {
		return nil
	}

	var testDir string
	candidates := []string{
		filepath.Join(projectPath, "tests"),
		filepath.Join(projectPath, "test"),
	}
	for _, cand := range candidates {
		if fi, err := os.Stat(cand); err == nil && fi.IsDir() {
			testDir = cand
			break
		}
	}
	if testDir == "" {
		return nil
	}

	var violations []TestCouplingViolation

	_ = filepath.Walk(testDir, func(path string, info os.FileInfo, err error) error {
		if err != nil || info == nil {
			return nil
		}

		relPath, relErr := filepath.Rel(projectPath, path)
		if relErr != nil {
			relPath = path
		}
		relNorm := filepath.ToSlash(relPath)

		if info.IsDir() {
			base := info.Name()
			if base == ".git" || base == ".noctifab" || base == "__pycache__" || base == "node_modules" || base == "vendor" {
				return filepath.SkipDir
			}
			return nil
		}

		ext := strings.ToLower(filepath.Ext(path))
		if !testCodeExtensions[ext] {
			return nil
		}

		sourceCat := classifyTestCategory(relNorm)
		if sourceCat == "" {
			return nil
		}

		fileViolations := scanFileForCoupling(path, relNorm, sourceCat)
		violations = append(violations, fileViolations...)
		return nil
	})

	return violations
}

func classifyTestCategory(relNorm string) string {
	lower := strings.ToLower(relNorm)
	switch {
	case strings.Contains(lower, "/unit/") || strings.HasPrefix(lower, "tests/unit/") || strings.HasPrefix(lower, "test/unit/"):
		return "unit"
	case strings.Contains(lower, "/integration/") || strings.HasPrefix(lower, "tests/integration/") || strings.HasPrefix(lower, "test/integration/"):
		return "integration"
	case strings.Contains(lower, "/e2e/") || strings.HasPrefix(lower, "tests/e2e/") || strings.HasPrefix(lower, "test/e2e/"):
		return "e2e"
	default:
		return ""
	}
}

func scanFileForCoupling(absPath, relPath, sourceCat string) []TestCouplingViolation {
	f, err := os.Open(absPath)
	if err != nil {
		return nil
	}
	defer func() {
		_ = f.Close()
	}()

	var forbidden []string
	switch sourceCat {
	case "unit":
		forbidden = []string{"integration", "e2e"}
	case "integration":
		forbidden = []string{"unit", "e2e"}
	case "e2e":
		forbidden = []string{"unit", "integration"}
	default:
		return nil
	}

	var violations []TestCouplingViolation
	scanner := bufio.NewScanner(f)
	lineNum := 0

	for scanner.Scan() {
		lineNum++
		line := scanner.Text()
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") || strings.HasPrefix(trimmed, "//") || strings.HasPrefix(trimmed, "*") {
			continue
		}

		for _, target := range forbidden {
			patterns := couplingRegexCache[target]
			matched := false
			for _, pat := range patterns {
				if pat.MatchString(line) {
					matched = true
					break
				}
			}
			if matched {
				violations = append(violations, TestCouplingViolation{
					FilePath:         relPath,
					SourceCategory:   sourceCat,
					ImportedCategory: target,
					LineNumber:       lineNum,
					MatchedLine:      trimmed,
					Reason:           fmt.Sprintf("%s test imports from %s test suite", sourceCat, target),
				})
			}
		}
	}

	return violations
}

// FormatTestCouplingViolations formats coupling diagnostics for preflight test runner error output.
func FormatTestCouplingViolations(violations []TestCouplingViolation) string {
	if len(violations) == 0 {
		return ""
	}

	var sb strings.Builder
	fmt.Fprintf(&sb, "Cross-layer test coupling violation detected (%d violation(s)):\n", len(violations))
	for _, v := range violations {
		fmt.Fprintf(&sb, "- %s:%d: [%s -> %s] %s (%s)\n", v.FilePath, v.LineNumber, v.SourceCategory, v.ImportedCategory, v.MatchedLine, v.Reason)
	}

	sb.WriteString("\nHermetic Test Isolation Mandate:\n")
	sb.WriteString("Tests in one category MUST NOT import from other test categories.\n")
	sb.WriteString("- Unit tests ('tests/unit/') must be hermetic and self-contained; never import from 'tests/e2e/' or 'tests/integration/'.\n")
	sb.WriteString("- Integration tests ('tests/integration/') must not depend on black-box E2E harnesses.\n")
	sb.WriteString("- Move shared helpers, factories, or fixtures to 'tests/fixtures/', 'tests/helpers/', or into the production codebase.\n")
	return sb.String()
}
