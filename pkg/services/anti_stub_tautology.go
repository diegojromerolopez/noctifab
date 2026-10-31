package services

import (
	"fmt"
	"path/filepath"
	"regexp"
	"strings"
)

var (
	// Python tautological assertion patterns
	pyAssertTautologyRE   = regexp.MustCompile(`(?i)^\s*assert\s+(?:True\b|1\b|not\s+False\b|1\s*==\s*1|0\s*==\s*0|["']["']\s*==\s*["']["']|["'][a-z0-9_]+["']\s*==\s*["'][a-z0-9_]+["'])`)
	pyUnittestTautologyRE = regexp.MustCompile(`(?i)^\s*self\.(?:assertTrue\s*\(\s*(?:True|1|not\s+False)\s*\)|assertFalse\s*\(\s*(?:False|0)\s*\)|assertEqual\s*\(\s*(?:1\s*,\s*1|0\s*,\s*0|True\s*,\s*True|False\s*,\s*False|["']["']\s*,\s*["']["'])\s*\)|assertNotEqual\s*\(\s*(?:1\s*,\s*0|0\s*,\s*1|True\s*,\s*False)\s*\)|assertIsNotNone\s*\(\s*(?:1|True|["'][^"']*["'])\s*\))`)
	pyAssertSelfRE        = regexp.MustCompile(`^\s*assert\s+([a-zA-Z_][a-zA-Z0-9_\.]*)\s*==\s*([a-zA-Z_][a-zA-Z0-9_\.]*)`)
	pyUnittestSelfRE      = regexp.MustCompile(`^\s*self\.assertEqual\s*\(\s*([a-zA-Z_][a-zA-Z0-9_\.]*)\s*,\s*([a-zA-Z_][a-zA-Z0-9_\.]*)\s*\)`)

	// Go tautological assertion patterns
	goAssertTautologyRE = regexp.MustCompile(`(?:assert|require)\.(?:True\s*\(\s*[a-zA-Z0-9_]+\s*,\s*true\s*\)|False\s*\(\s*[a-zA-Z0-9_]+\s*,\s*false\s*\)|Equal\s*\(\s*[a-zA-Z0-9_]+\s*,\s*(?:1\s*,\s*1|0\s*,\s*0|true\s*,\s*true|false\s*,\s*false|""\s*,\s*"")\s*\)|NotNil\s*\(\s*[a-zA-Z0-9_]+\s*,\s*(?:&struct\{\}\{\}|new\([a-zA-Z0-9_]+\))\s*\))`)
	goSelfCompareRE     = regexp.MustCompile(`(?:assert|require)\.Equal(?:Values)?\s*\(\s*[a-zA-Z0-9_]+\s*,\s*([a-zA-Z_][a-zA-Z0-9_\.]*)\s*,\s*([a-zA-Z_][a-zA-Z0-9_\.]*)\s*\)`)

	// C / C++ tautological assertion patterns
	cAssertTautologyRE      = regexp.MustCompile(`(?i)^\s*(?:assert|CU_ASSERT|CU_ASSERT_TRUE|cr_assert|TEST_ASSERT|TEST_ASSERT_TRUE|EXPECT_TRUE|ASSERT_TRUE)\s*\(\s*(?:1|true|!0|1\s*==\s*1|0\s*==\s*0)\s*\)\s*;?`)
	cAssertEqualTautologyRE = regexp.MustCompile(`(?i)^\s*(?:CU_ASSERT_EQUAL|EXPECT_EQ|ASSERT_EQ|TEST_ASSERT_EQUAL)\s*\(\s*(?:1\s*,\s*1|0\s*,\s*0|true\s*,\s*true)\s*\)\s*;?`)
	cAssertSelfRE           = regexp.MustCompile(`^\s*(?:assert|cr_assert|TEST_ASSERT)\s*\(\s*([a-zA-Z_][a-zA-Z0-9_\.\->]*)\s*==\s*([a-zA-Z_][a-zA-Z0-9_\.\->]*)\s*\)\s*;?`)
	cAssertEqualSelfRE      = regexp.MustCompile(`^\s*(?:EXPECT_EQ|ASSERT_EQ|TEST_ASSERT_EQUAL|CU_ASSERT_EQUAL)\s*\(\s*([a-zA-Z_][a-zA-Z0-9_\.\->]*)\s*,\s*([a-zA-Z_][a-zA-Z0-9_\.\->]*)\s*\)\s*;?`)

	// Rust tautological assertion patterns
	rustAssertTautologyRE = regexp.MustCompile(`^\s*(?:assert!\s*\(\s*(?:true|1\s*==\s*1|0\s*==\s*0)\s*\)|assert_eq!\s*\(\s*(?:1\s*,\s*1|0\s*,\s*0|true\s*,\s*true)\s*\))\s*;?`)
	rustSelfCompareRE     = regexp.MustCompile(`^\s*assert_eq!\s*\(\s*([a-zA-Z_][a-zA-Z0-9_\.]*)\s*,\s*([a-zA-Z_][a-zA-Z0-9_\.]*)\s*\)\s*;?`)

	// JS / TS tautological assertion patterns
	jsAssertTautologyRE = regexp.MustCompile(`(?i)^\s*(?:expect\s*\(\s*(?:true|1|["'][^"']*["'])\s*\)\s*\.(?:toBe|toEqual|toStrictEqual)\s*\(\s*(?:true|1|["'][^"']*["'])\s*\)|expect\s*\(\s*true\s*\)\s*\.toBeTruthy\s*\(\s*\)|expect\s*\(\s*false\s*\)\s*\.toBeFalsy\s*\(\s*\)|assert(?:\.strictEqual|\.equal)?\s*\(\s*(?:true|1\s*,\s*1|true\s*,\s*true)\s*\))\s*;?`)
	jsSelfCompareRE     = regexp.MustCompile(`^\s*expect\s*\(\s*([a-zA-Z_][a-zA-Z0-9_\.]*)\s*\)\s*\.(?:toBe|toEqual|toStrictEqual)\s*\(\s*([a-zA-Z_][a-zA-Z0-9_\.]*)\s*\)`)

	// Evasive test skipping patterns
	pyUnconditionalSkipRE     = regexp.MustCompile(`^\s*@(?:pytest\.mark\.skip(?:\s*\([^)]*\))?|unittest\.skip(?:\s*\([^)]*\))?)\s*$`)
	rustUnconditionalIgnoreRE = regexp.MustCompile(`^\s*#\[ignore\]\s*$`)
	jsUnconditionalSkipRE     = regexp.MustCompile(`^\s*(?:it|test|describe)\.skip\s*\(`)

	// Function header patterns for tests
	pyTestDefHeaderRE  = regexp.MustCompile(`^\s*(?:async\s+)?def\s+test_([a-zA-Z0-9_]*)\s*\(`)
	goTestFuncHeaderRE = regexp.MustCompile(`^\s*func\s+Test([A-Z][a-zA-Z0-9_]*)\s*\(\s*([a-zA-Z0-9_]+)\s*\*testing\.T\s*\)\s*\{\s*$`)
	goTestFuncInlineRE = regexp.MustCompile(`^\s*func\s+Test([A-Z][a-zA-Z0-9_]*)\s*\(\s*([a-zA-Z0-9_]+)\s*\*testing\.T\s*\)\s*\{\s*(?:return;?\s*)?\}\s*$`)
	cTestFuncHeaderRE  = regexp.MustCompile(`^\s*(?:void|int)\s+test_([a-zA-Z0-9_]*)\s*\([^)]*\)\s*\{\s*$`)
	cTestFuncInlineRE  = regexp.MustCompile(`^\s*(?:void|int)\s+test_([a-zA-Z0-9_]*)\s*\([^)]*\)\s*\{\s*(?:return\s+0\s*;\s*|return\s*;\s*)?\}\s*$`)
	jsTestInlineRE     = regexp.MustCompile(`^\s*(?:it|test)\s*\(\s*['"][^'"]*['"]\s*,\s*(?:async\s*)?\(\s*\)\s*=>\s*\{\s*\}\s*\)\s*;?`)
)

func isTestFilePath(path string) bool {
	lower := strings.ToLower(path)
	base := filepath.Base(lower)
	return strings.Contains(lower, "test") ||
		strings.Contains(lower, "spec") ||
		strings.HasPrefix(base, "test_") ||
		strings.HasSuffix(base, "_test.go") ||
		strings.HasSuffix(base, "_test.py") ||
		strings.HasSuffix(base, ".spec.ts") ||
		strings.HasSuffix(base, ".spec.js") ||
		strings.HasSuffix(base, ".test.ts") ||
		strings.HasSuffix(base, ".test.js")
}

// checkLineTautology inspects a single line of code in a test file for tautological assertions or evasive skips.
func checkLineTautology(path, trimmed string, lineNum int) *AntiStubViolation {
	// 1. Literal Tautologies
	if pyAssertTautologyRE.MatchString(trimmed) || pyUnittestTautologyRE.MatchString(trimmed) ||
		goAssertTautologyRE.MatchString(trimmed) ||
		cAssertTautologyRE.MatchString(trimmed) || cAssertEqualTautologyRE.MatchString(trimmed) ||
		rustAssertTautologyRE.MatchString(trimmed) ||
		jsAssertTautologyRE.MatchString(trimmed) {
		return &AntiStubViolation{
			Path:    path,
			Line:    lineNum,
			Rule:    "tautological_test_assertion",
			Snippet: trimmed,
		}
	}

	// 2. Self-comparison tautologies (comparing identifier to itself)
	if m := pyAssertSelfRE.FindStringSubmatch(trimmed); len(m) == 3 && m[1] != "" && m[1] == m[2] {
		return &AntiStubViolation{
			Path:    path,
			Line:    lineNum,
			Rule:    "tautological_test_assertion",
			Snippet: fmt.Sprintf("%s (self-comparison: comparing %s to itself)", trimmed, m[1]),
		}
	}
	if m := pyUnittestSelfRE.FindStringSubmatch(trimmed); len(m) == 3 && m[1] != "" && m[1] == m[2] {
		return &AntiStubViolation{
			Path:    path,
			Line:    lineNum,
			Rule:    "tautological_test_assertion",
			Snippet: fmt.Sprintf("%s (self-comparison: comparing %s to itself)", trimmed, m[1]),
		}
	}
	if m := goSelfCompareRE.FindStringSubmatch(trimmed); len(m) == 3 && m[1] != "" && m[1] == m[2] {
		return &AntiStubViolation{
			Path:    path,
			Line:    lineNum,
			Rule:    "tautological_test_assertion",
			Snippet: fmt.Sprintf("%s (self-comparison: comparing %s to itself)", trimmed, m[1]),
		}
	}
	if m := cAssertSelfRE.FindStringSubmatch(trimmed); len(m) == 3 && m[1] != "" && m[1] == m[2] {
		return &AntiStubViolation{
			Path:    path,
			Line:    lineNum,
			Rule:    "tautological_test_assertion",
			Snippet: fmt.Sprintf("%s (self-comparison: comparing %s to itself)", trimmed, m[1]),
		}
	}
	if m := cAssertEqualSelfRE.FindStringSubmatch(trimmed); len(m) == 3 && m[1] != "" && m[1] == m[2] {
		return &AntiStubViolation{
			Path:    path,
			Line:    lineNum,
			Rule:    "tautological_test_assertion",
			Snippet: fmt.Sprintf("%s (self-comparison: comparing %s to itself)", trimmed, m[1]),
		}
	}
	if m := rustSelfCompareRE.FindStringSubmatch(trimmed); len(m) == 3 && m[1] != "" && m[1] == m[2] {
		return &AntiStubViolation{
			Path:    path,
			Line:    lineNum,
			Rule:    "tautological_test_assertion",
			Snippet: fmt.Sprintf("%s (self-comparison: comparing %s to itself)", trimmed, m[1]),
		}
	}
	if m := jsSelfCompareRE.FindStringSubmatch(trimmed); len(m) == 3 && m[1] != "" && m[1] == m[2] {
		return &AntiStubViolation{
			Path:    path,
			Line:    lineNum,
			Rule:    "tautological_test_assertion",
			Snippet: fmt.Sprintf("%s (self-comparison: comparing %s to itself)", trimmed, m[1]),
		}
	}

	// 3. Evasive test skips
	if pyUnconditionalSkipRE.MatchString(trimmed) || rustUnconditionalIgnoreRE.MatchString(trimmed) || jsUnconditionalSkipRE.MatchString(trimmed) {
		return &AntiStubViolation{
			Path:    path,
			Line:    lineNum,
			Rule:    "evasive_test_skipping",
			Snippet: fmt.Sprintf("%s (unconditional test skip evades verification)", trimmed),
		}
	}

	return nil
}

// checkMultiLineTestViolations inspects test file structure for vacuous or empty test functions.
func checkMultiLineTestViolations(path string, lines []string) []AntiStubViolation {
	var violations []AntiStubViolation
	numLines := len(lines)
	ext := strings.ToLower(filepath.Ext(path))

	for idx, line := range lines {
		trimmed := strings.TrimSpace(line)
		lineNum := idx + 1

		// JS/TS inline empty test
		if (ext == ".js" || ext == ".ts" || ext == ".jsx" || ext == ".tsx") && jsTestInlineRE.MatchString(trimmed) {
			violations = append(violations, AntiStubViolation{
				Path:    path,
				Line:    lineNum,
				Rule:    "vacuous_empty_test",
				Snippet: fmt.Sprintf("%s (test block has empty body with no assertions)", trimmed),
			})
			continue
		}

		// Go inline empty test
		if ext == ".go" && goTestFuncInlineRE.MatchString(trimmed) {
			violations = append(violations, AntiStubViolation{
				Path:    path,
				Line:    lineNum,
				Rule:    "vacuous_empty_test",
				Snippet: fmt.Sprintf("%s (test function has empty body with no assertions)", trimmed),
			})
			continue
		}

		// C inline empty test
		if (ext == ".c" || ext == ".cpp") && cTestFuncInlineRE.MatchString(trimmed) {
			violations = append(violations, AntiStubViolation{
				Path:    path,
				Line:    lineNum,
				Rule:    "vacuous_empty_test",
				Snippet: fmt.Sprintf("%s (test function has empty body with no assertions)", trimmed),
			})
			continue
		}

		// Go multi-line empty test
		if ext == ".go" && goTestFuncHeaderRE.MatchString(trimmed) {
			if endLine, isStub := scanForEmptyBlock(lines, idx, numLines); isStub {
				violations = append(violations, AntiStubViolation{
					Path:    path,
					Line:    endLine,
					Rule:    "vacuous_empty_test",
					Snippet: fmt.Sprintf("%s -> } (test function contains no assertions or executable verification)", trimmed),
				})
			}
		}

		// C multi-line empty test
		if (ext == ".c" || ext == ".cpp") && cTestFuncHeaderRE.MatchString(trimmed) {
			if endLine, isStub := scanForEmptyBlock(lines, idx, numLines); isStub {
				violations = append(violations, AntiStubViolation{
					Path:    path,
					Line:    endLine,
					Rule:    "vacuous_empty_test",
					Snippet: fmt.Sprintf("%s -> } (test function contains no assertions or executable verification)", trimmed),
				})
			}
		}

		// Python empty test function: def test_... followed only by pass, ..., or return
		if ext == ".py" && pyTestDefHeaderRE.MatchString(line) {
			if endLine, isStub := scanForEmptyPyTest(lines, idx, numLines); isStub {
				violations = append(violations, AntiStubViolation{
					Path:    path,
					Line:    endLine,
					Rule:    "vacuous_empty_test",
					Snippet: fmt.Sprintf("%s (test contains no assertions or verification logic)", trimmed),
				})
			}
		}
	}

	return violations
}

// scanForEmptyPyTest checks if a python test function contains only pass, ..., return, or docstrings.
func scanForEmptyPyTest(lines []string, startIdx int, numLines int) (int, bool) {
	if startIdx >= numLines {
		return 0, false
	}
	baseIndent := getIndentation(lines[startIdx])
	hasExecutableLogic := false
	inDocstring := false
	docstringMarker := ""
	lastLine := startIdx + 1

	for nextIdx := startIdx + 1; nextIdx < numLines; nextIdx++ {
		raw := lines[nextIdx]
		trimmed := strings.TrimSpace(raw)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		currIndent := getIndentation(raw)
		if currIndent <= baseIndent {
			// Dedented to outer scope
			break
		}
		lastLine = nextIdx + 1

		// Check multi-line docstring
		if inDocstring {
			if strings.Contains(trimmed, docstringMarker) {
				inDocstring = false
			}
			continue
		}
		if strings.HasPrefix(trimmed, `"""`) || strings.HasPrefix(trimmed, `'''`) {
			docstringMarker = trimmed[:3]
			rest := trimmed[3:]
			if !strings.Contains(rest, docstringMarker) {
				inDocstring = true
			}
			continue
		}

		// Check if it's only pass, ..., or return (without expression)
		if trimmed == "pass" || trimmed == "..." || trimmed == "return" || trimmed == "return None" {
			continue
		}

		hasExecutableLogic = true
		break
	}

	return lastLine, !hasExecutableLogic
}
