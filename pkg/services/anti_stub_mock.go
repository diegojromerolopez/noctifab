package services

import (
	"fmt"
	"path/filepath"
	"regexp"
	"strings"
)

var (
	// Python mock creation and call assertion patterns
	pyMockCreationRE = regexp.MustCompile(`\b(?:Mock|MagicMock|create_autospec|PropertyMock)\s*\(|@patch(?:\.object)?\s*\(|\bmocker\.(?:patch|spy)\s*\(`)
	pyMockAssertRE   = regexp.MustCompile(`\.(?:assert_called|assert_called_once|assert_called_with|assert_called_once_with|assert_any_call|assert_not_called|assert_has_calls)\s*\(|\bassert\s+[a-zA-Z0-9_\.]*\b(?:called|call_count|call_args)\b`)

	// Go mock expectation and assertion patterns
	goMockSetupRE  = regexp.MustCompile(`\.On\s*\(\s*["']|\bnew\s*\(\s*(?:mocks?\.)?Mock|\b&Mock[A-Z]|\bmock\.Mock\b`)
	goMockAssertRE = regexp.MustCompile(`\.(?:AssertExpectations|AssertCalled|AssertNotCalled|AssertNumberOfCalls)\s*\(|(?:assert|require)\.(?:True|False)\s*\(\s*[a-zA-Z0-9_]+\s*,\s*[a-zA-Z0-9_\.]*\b[cC]alled\b`)

	// JS / TS mock creation and assertion patterns
	jsMockCreationRE = regexp.MustCompile(`\b(?:jest|vi)\.(?:fn|spyOn)\s*\(|\bsinon\.(?:spy|stub)\s*\(`)
	jsMockAssertRE   = regexp.MustCompile(`\b(?:toHaveBeenCalled|toHaveBeenCalledWith|toHaveBeenCalledTimes|toHaveBeenLastCalledWith|toHaveBeenCalledExactlyOnceWith)\s*\(|\bsinon\.assert\.called|\bexpect\s*\([^)]*\)\.toBeCalled|\bcalled(?:Once|With|Twice|Thrice)\b`)

	// Test block headers
	jsBlockHeaderRE = regexp.MustCompile(`^\s*(?:it|test)(?:\.only|\.concurrent)?\s*\(\s*['"]`)
)

// checkMockCallViolations verifies that whenever tests instantiate or inject mock objects,
// they also assert that the mock methods were actually called (or explicitly not called).
func checkMockCallViolations(path string, lines []string) []AntiStubViolation {
	var violations []AntiStubViolation
	ext := strings.ToLower(filepath.Ext(path))

	switch {
	case ext == ".py":
		violations = append(violations, checkPythonMockViolations(path, lines)...)
	case ext == ".go":
		violations = append(violations, checkGoMockViolations(path, lines)...)
	case ext == ".js" || ext == ".ts" || ext == ".jsx" || ext == ".tsx":
		violations = append(violations, checkJSMockViolations(path, lines)...)
	}

	return violations
}

func checkPythonMockViolations(path string, lines []string) []AntiStubViolation {
	var violations []AntiStubViolation
	numLines := len(lines)

	for i := 0; i < numLines; i++ {
		line := lines[i]
		trimmed := strings.TrimSpace(line)

		// Check for def test_... or @patch preceding def test_...
		if pyTestDefHeaderRE.MatchString(line) {
			funcStartLine := i + 1
			baseIndent := getIndentation(line)
			funcLines := []string{line}

			// Include preceding decorator lines (e.g. @patch)
			for d := i - 1; d >= 0; d-- {
				dTrimmed := strings.TrimSpace(lines[d])
				if strings.HasPrefix(dTrimmed, "@") {
					funcLines = append([]string{lines[d]}, funcLines...)
				} else {
					break
				}
			}

			// Collect function body
			for j := i + 1; j < numLines; j++ {
				nextRaw := lines[j]
				nextTrimmed := strings.TrimSpace(nextRaw)
				if nextTrimmed == "" || strings.HasPrefix(nextTrimmed, "#") {
					continue
				}
				if getIndentation(nextRaw) <= baseIndent {
					break
				}
				funcLines = append(funcLines, nextRaw)
			}

			funcBody := strings.Join(funcLines, "\n")
			if pyMockCreationRE.MatchString(funcBody) && !pyMockAssertRE.MatchString(funcBody) {
				violations = append(violations, AntiStubViolation{
					Path:    path,
					Line:    funcStartLine,
					Rule:    "unverified_mock_assertion",
					Snippet: fmt.Sprintf("%s (test instantiates/injects mock dependency but never asserts that mock methods were called)", trimmed),
				})
			}
		}
	}

	return violations
}

func checkGoMockViolations(path string, lines []string) []AntiStubViolation {
	var violations []AntiStubViolation
	numLines := len(lines)

	for i := 0; i < numLines; i++ {
		line := lines[i]
		trimmed := strings.TrimSpace(line)

		if goTestFuncHeaderRE.MatchString(trimmed) {
			funcStartLine := i + 1
			funcLines := []string{line}
			braceCount := strings.Count(line, "{") - strings.Count(line, "}")

			for j := i + 1; j < numLines; j++ {
				nextRaw := lines[j]
				funcLines = append(funcLines, nextRaw)
				braceCount += strings.Count(nextRaw, "{") - strings.Count(nextRaw, "}")
				if braceCount <= 0 {
					break
				}
			}

			funcBody := strings.Join(funcLines, "\n")
			if goMockSetupRE.MatchString(funcBody) && !goMockAssertRE.MatchString(funcBody) {
				violations = append(violations, AntiStubViolation{
					Path:    path,
					Line:    funcStartLine,
					Rule:    "unverified_mock_assertion",
					Snippet: fmt.Sprintf("%s (test sets up mock expectations but never asserts that mock methods were called)", trimmed),
				})
			}
		}
	}

	return violations
}

func checkJSMockViolations(path string, lines []string) []AntiStubViolation {
	var violations []AntiStubViolation
	numLines := len(lines)

	for i := 0; i < numLines; i++ {
		line := lines[i]
		trimmed := strings.TrimSpace(line)

		if jsBlockHeaderRE.MatchString(trimmed) {
			funcStartLine := i + 1
			funcLines := []string{line}
			braceCount := strings.Count(line, "{") - strings.Count(line, "}")

			for j := i + 1; j < numLines; j++ {
				nextRaw := lines[j]
				funcLines = append(funcLines, nextRaw)
				braceCount += strings.Count(nextRaw, "{") - strings.Count(nextRaw, "}")
				if braceCount <= 0 {
					break
				}
			}

			funcBody := strings.Join(funcLines, "\n")
			if jsMockCreationRE.MatchString(funcBody) && !jsMockAssertRE.MatchString(funcBody) {
				violations = append(violations, AntiStubViolation{
					Path:    path,
					Line:    funcStartLine,
					Rule:    "unverified_mock_assertion",
					Snippet: fmt.Sprintf("%s (test creates mock function but never asserts that it was called)", trimmed),
				})
			}
		}
	}

	return violations
}
