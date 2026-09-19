package services

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

// PrepareTestEnvironment performs language-agnostic pre-flight structural
// preparation on workspace test directories. For example, ensuring nested test
// packages contain necessary structural markers (like __init__.py in Python test
// trees) so that ecosystem test runners do not silently skip nested suites.
func PrepareTestEnvironment(projectPath string) error {
	if projectPath == "" {
		return nil
	}

	testRoots := []string{"tests", "test", "spec", "specs"}
	for _, root := range testRoots {
		rootPath := filepath.Join(projectPath, root)
		info, err := os.Stat(rootPath)
		if err != nil || !info.IsDir() {
			continue
		}

		_ = filepath.Walk(rootPath, func(path string, fi os.FileInfo, walkErr error) error {
			if walkErr != nil || !fi.IsDir() {
				return nil
			}

			// Skip version control and build cache folders
			base := fi.Name()
			if strings.HasPrefix(base, ".") || base == "node_modules" || base == "__pycache__" || base == "target" || base == "vendor" {
				return filepath.SkipDir
			}

			// Check if directory contains Python test files
			entries, readErr := os.ReadDir(path)
			if readErr != nil {
				return nil
			}

			hasPythonFiles := false
			for _, entry := range entries {
				if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".py") {
					hasPythonFiles = true
					break
				}
			}

			if hasPythonFiles {
				initPath := filepath.Join(path, "__init__.py")
				if _, statErr := os.Stat(initPath); os.IsNotExist(statErr) {
					// Auto-scaffold package marker for Python discovery
					_ = os.WriteFile(initPath, []byte(""), 0644)
				}
			}

			return nil
		})
	}

	return nil
}

// DiscoverTestFiles scans the workspace and returns relative paths to all
// recognized test files across all supported languages (Go, Python, Rust,
// JS/TS, Java, C/C++).
func DiscoverTestFiles(projectPath string) []string {
	if projectPath == "" {
		return nil
	}

	var testFiles []string
	_ = filepath.Walk(projectPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}

		if info.IsDir() {
			base := info.Name()
			if strings.HasPrefix(base, ".") || base == "node_modules" || base == "vendor" ||
				base == "target" || base == "build" || base == "dist" || base == "__pycache__" ||
				base == ".venv" || base == "venv" {
				return filepath.SkipDir
			}
			return nil
		}

		rel, relErr := filepath.Rel(projectPath, path)
		if relErr != nil {
			return nil
		}

		if IsTestFile(rel) {
			testFiles = append(testFiles, rel)
		}

		return nil
	})

	return testFiles
}

// IsTestFile returns true if the relative file path represents a recognized
// test file in any supported ecosystem.
func IsTestFile(relPath string) bool {
	lower := strings.ToLower(filepath.ToSlash(relPath))
	base := filepath.Base(lower)

	// Exclude non-source metadata/init files
	if base == "__init__.py" || base == "conftest.py" || base == "setup.py" {
		return false
	}
	ext := filepath.Ext(base)

	// Go: *_test.go
	if strings.HasSuffix(base, "_test.go") {
		return true
	}

	// Python: test_*.py, *_test.py
	if ext == ".py" {
		if strings.HasPrefix(base, "test_") || strings.HasSuffix(base, "_test.py") {
			return true
		}
		if strings.HasPrefix(lower, "tests/") || strings.HasPrefix(lower, "test/") {
			return true
		}
	}

	// JavaScript / TypeScript: *.test.js, *.spec.js, *.test.ts, *.spec.ts, etc.
	if ext == ".js" || ext == ".ts" || ext == ".jsx" || ext == ".tsx" || ext == ".mjs" {
		if strings.Contains(base, ".test.") || strings.Contains(base, ".spec.") ||
			strings.HasPrefix(base, "test-") || strings.HasPrefix(base, "test_") {
			return true
		}
		if strings.HasPrefix(lower, "tests/") || strings.HasPrefix(lower, "test/") {
			return true
		}
	}

	// Rust: *_test.rs, test_*.rs or under tests/
	if ext == ".rs" {
		if strings.HasSuffix(base, "_test.rs") || strings.HasPrefix(base, "test_") {
			return true
		}
		if strings.HasPrefix(lower, "tests/") {
			return true
		}
	}

	// Java: *Test.java, *Tests.java, *TestCase.java
	if ext == ".java" {
		if strings.HasSuffix(base, "test.java") || strings.HasSuffix(base, "tests.java") || strings.HasSuffix(base, "testcase.java") {
			return true
		}
	}

	// C / C++: *_test.c, *_test.cpp, *_test.cc
	if strings.HasSuffix(base, "_test.c") || strings.HasSuffix(base, "_test.cpp") || strings.HasSuffix(base, "_test.cc") {
		return true
	}

	return false
}

var (
	pyRanTestsRegex = regexp.MustCompile(`(?i)ran\s+(\d+)\s+test`)
	cargoTestsRegex = regexp.MustCompile(`(?i)test\s+result:\s+ok\.\s+(\d+)\s+passed`)
	jestTestsRegex  = regexp.MustCompile(`(?i)tests:\s+(\d+)\s+passed`)
	tapTestsRegex   = regexp.MustCompile(`(?i)(?:# tests|# pass)\s+(\d+)`)
)

// EvaluateTestExecution inspects runner output and discovered test files to
// detect zero-test runs, skipped test suites, and discovery discrepancies across
// all languages.
func EvaluateTestExecution(projectPath string, out string) (bool, string) {
	outLower := strings.ToLower(out)

	// 1. Explicit zero-test execution markers across runners
	if strings.Contains(outLower, "no tests ran") ||
		strings.Contains(outLower, "ran 0 tests") ||
		strings.Contains(outLower, "collected 0 items") ||
		strings.Contains(outLower, "collected 0 tests") ||
		strings.Contains(outLower, "0 passed") && strings.Contains(outLower, "0 failed") ||
		strings.Contains(outLower, "[no test files]") ||
		strings.Contains(outLower, "no tests found") ||
		strings.Contains(outLower, "no tests were found") ||
		strings.Contains(outLower, "nothing to be done") ||
		strings.Contains(outLower, "exit status 5") {
		return true, "Test suite execution failed: 0 test assertions executed."
	}

	testFiles := DiscoverTestFiles(projectPath)

	// 2. Empty output when test files exist
	if strings.TrimSpace(out) == "" {
		if len(testFiles) > 0 {
			return true, fmt.Sprintf("Test runner produced empty output while %d test files exist on disk.", len(testFiles))
		}
		testCmd := DetectDefaultTestCommand(projectPath)
		if strings.HasPrefix(testCmd, "make") {
			return true, "Test suite failed: 0 test files discovered in tests/ directory and 0 test assertions executed."
		}
		return false, ""
	}

	// 3. Discrepancy Detection:
	// When multiple test files exist, check whether the test runner actually executed
	// tests across the discovered test suite or silently skipped nested suites.
	if len(testFiles) >= 3 {
		executedCount := extractExecutedTestCount(out)
		var unreferenced []string

		for _, tf := range testFiles {
			base := filepath.Base(tf)
			nameWithoutExt := strings.TrimSuffix(base, filepath.Ext(base))

			// Check if filename or test module name appears in output
			if !strings.Contains(out, base) && !strings.Contains(out, nameWithoutExt) {
				unreferenced = append(unreferenced, tf)
			}
		}

		// If a large majority of test files are completely unreferenced and executed count is trivially small (<= 1)
		if executedCount == 1 && len(unreferenced) >= len(testFiles)-1 {
			return true, fmt.Sprintf(
				"Test discovery discrepancy: Only 1 test executed while %d test files exist on disk (%d unreferenced: %s). Check test runner discovery path or package markers.",
				len(testFiles), len(unreferenced), strings.Join(unreferenced, ", "),
			)
		}
	}

	return false, ""
}

func extractExecutedTestCount(out string) int {
	if m := pyRanTestsRegex.FindStringSubmatch(out); len(m) > 1 {
		if c, err := strconv.Atoi(m[1]); err == nil {
			return c
		}
	}
	if m := cargoTestsRegex.FindStringSubmatch(out); len(m) > 1 {
		if c, err := strconv.Atoi(m[1]); err == nil {
			return c
		}
	}
	if m := jestTestsRegex.FindStringSubmatch(out); len(m) > 1 {
		if c, err := strconv.Atoi(m[1]); err == nil {
			return c
		}
	}
	if m := tapTestsRegex.FindStringSubmatch(out); len(m) > 1 {
		if c, err := strconv.Atoi(m[1]); err == nil {
			return c
		}
	}
	return -1
}
