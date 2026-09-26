package services

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// TestCountMonotonicityViolation reports that the number of active test cases decreased.
type TestCountMonotonicityViolation struct {
	BaselineCount int
	NewCount      int
	Context       string
}

func (v TestCountMonotonicityViolation) Error() string {
	return fmt.Sprintf("test count monotonicity violation: test suite shrank from %d to %d test cases (%s); deleting or disabling failing test cases is prohibited",
		v.BaselineCount, v.NewCount, v.Context)
}

// TestCountMonotonicityGuard verifies that the total count of test cases does not decrease.
type TestCountMonotonicityGuard struct {
	pyTestRe   *regexp.Regexp
	goTestRe   *regexp.Regexp
	jsTestRe   *regexp.Regexp
	rustTestRe *regexp.Regexp
}

// NewTestCountMonotonicityGuard creates a new TestCountMonotonicityGuard.
func NewTestCountMonotonicityGuard() *TestCountMonotonicityGuard {
	return &TestCountMonotonicityGuard{
		pyTestRe:   regexp.MustCompile(`^\s*def\s+test_[a-zA-Z0-9_]+\s*\(`),
		goTestRe:   regexp.MustCompile(`^\s*func\s+Test[a-zA-Z0-9_]+\s*\(`),
		jsTestRe:   regexp.MustCompile(`(?:^|[^\w])(?:it|test)\s*\(\s*['"\x60]`),
		rustTestRe: regexp.MustCompile(`^\s*#\[test\]`),
	}
}

// CountTestsInContent parses the given file content and counts test case definitions.
func (g *TestCountMonotonicityGuard) CountTestsInContent(filePath, content string) int {
	ext := strings.ToLower(filepath.Ext(filePath))
	lines := strings.Split(content, "\n")
	count := 0

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		switch ext {
		case ".py":
			if strings.HasPrefix(trimmed, "#") {
				continue
			}
			if g.pyTestRe.MatchString(trimmed) {
				count++
			}
		case ".go":
			if strings.HasPrefix(trimmed, "//") {
				continue
			}
			if g.goTestRe.MatchString(trimmed) {
				count++
			}
		case ".js", ".jsx", ".ts", ".tsx":
			if strings.HasPrefix(trimmed, "//") {
				continue
			}
			if g.jsTestRe.MatchString(trimmed) {
				count++
			}
		case ".rs":
			if strings.HasPrefix(trimmed, "//") {
				continue
			}
			if g.rustTestRe.MatchString(trimmed) {
				count++
			}
		}
	}

	return count
}

// CountTestsInDir scans all test files in rootDir and returns the total test count.
func (g *TestCountMonotonicityGuard) CountTestsInDir(rootDir string) (int, error) {
	total := 0
	err := filepath.WalkDir(rootDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			name := d.Name()
			if name == ".git" || name == "node_modules" || name == ".venv" || name == "target" || name == "__pycache__" {
				return filepath.SkipDir
			}
			return nil
		}

		// Only check files in test directories or ending with test suffixes
		base := filepath.Base(path)
		isTestFile := strings.HasSuffix(base, "_test.go") ||
			strings.HasPrefix(base, "test_") ||
			strings.HasSuffix(base, "_test.py") ||
			strings.HasSuffix(base, ".test.ts") ||
			strings.HasSuffix(base, ".test.js") ||
			strings.HasSuffix(base, ".spec.ts") ||
			strings.HasSuffix(base, ".spec.js") ||
			strings.Contains(path, string(filepath.Separator)+"tests"+string(filepath.Separator))

		if !isTestFile {
			return nil
		}

		data, err := os.ReadFile(path)
		if err != nil {
			return nil
		}

		total += g.CountTestsInContent(path, string(data))
		return nil
	})

	return total, err
}

// ValidateMonotonicity ensures that newCount is greater than or equal to baselineCount.
func (g *TestCountMonotonicityGuard) ValidateMonotonicity(baselineCount, newCount int, context string) error {
	if newCount < baselineCount {
		return &TestCountMonotonicityViolation{
			BaselineCount: baselineCount,
			NewCount:      newCount,
			Context:       context,
		}
	}
	return nil
}
