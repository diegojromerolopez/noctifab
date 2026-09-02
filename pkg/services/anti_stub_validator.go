package services

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// AntiStubViolation records a specific instance of forbidden stub code,
// error-masking shell script, or vacuous test.
type AntiStubViolation struct {
	Path    string `json:"path"`
	Line    int    `json:"line"`
	Rule    string `json:"rule"`
	Snippet string `json:"snippet"`
}

// AntiStubValidator inspects generated source and test files for placeholder
// stubs, empty functions, shell error masking, and tautological assertions.
type AntiStubValidator struct{}

var (
	// Python stub patterns
	pyRaiseNotImplRE = regexp.MustCompile(`(?i)raise\s+NotImplementedError(?:\(.*?\))?|raise\s+NotImplemented\b`)
	pyDefHeaderRE    = regexp.MustCompile(`^\s*(?:async\s+)?def\s+([a-zA-Z0-9_]+)\s*\(`)
	pyStubBodyRE     = regexp.MustCompile(`^\s*(?:pass|\.\.\.)\s*(?:#.*)?$`)

	// Go stub patterns
	goPanicStubRE = regexp.MustCompile(`(?i)panic\s*\(\s*["'](?:not\s+implemented|todo|unimplemented|stub)["']\s*\)`)

	// Rust stub patterns
	rustTodoStubRE = regexp.MustCompile(`\b(?:todo!|unimplemented!)\s*\(`)

	// JS/TS stub patterns
	jsThrowNotImplRE = regexp.MustCompile(`(?i)throw\s+new\s+Error\s*\(\s*["'](?:not\s+implemented|todo|unimplemented)["']\s*\)`)

	// Universal forbidden markers in production code
	universalTodoRE = regexp.MustCompile(`(?i)(?:#|//|/\*)\s*(?:TODO|FIXME)\s*:\s*(?:implement|stub|fill\s+in|not\s+implemented)`)

	// Shell script masking patterns
	shellMaskTrueRE  = regexp.MustCompile(`\|\|\s*true\b`)
	shellMaskExit0RE = regexp.MustCompile(`\|\|\s*exit\s+0\b`)
	shellSetPlusERE  = regexp.MustCompile(`^\s*set\s+\+e\b`)

	// Vacuous / Tautological test patterns
	tautologyPyRE = regexp.MustCompile(`(?i)^\s*(?:assert\s+(?:True|1\s*==\s*1|0\s*==\s*0)\b|self\.assertTrue\s*\(\s*True\s*\)|self\.assertEqual\s*\(\s*(?:1\s*,\s*1|0\s*,\s*0|True\s*,\s*True)\s*\))`)
	tautologyGoRE = regexp.MustCompile(`assert\.(?:True|Equal)\s*\(\s*t\s*,\s*(?:true|1\s*,\s*1)\s*\)`)

	// Makefile stub patterns
	makeStubEchoRE = regexp.MustCompile(`(?i)^\s*@?echo\s+["'].*["']\s*$|^\s*@?echo\s+[^"'].*$|^\s*@?true\s*$|^\s*:\s*$`)
)

// NewAntiStubValidator initializes an AntiStubValidator.
func NewAntiStubValidator() *AntiStubValidator {
	return &AntiStubValidator{}
}

// ValidateWorkspace checks all specified targetFiles (or all files under rootPath if targetFiles is empty).
func (v *AntiStubValidator) ValidateWorkspace(rootPath string, targetFiles []string) ([]AntiStubViolation, error) {
	var violations []AntiStubViolation

	if len(targetFiles) > 0 {
		for _, rel := range targetFiles {
			fullPath := filepath.Join(rootPath, rel)
			if stat, err := os.Stat(fullPath); err == nil && !stat.IsDir() && stat.Mode().IsRegular() && stat.Size() <= 1024*1024 {
				content, rErr := os.ReadFile(fullPath)
				if rErr == nil {
					violations = append(violations, v.ValidateContent(rel, string(content))...)
				}
			}
		}
		return violations, nil
	}

	files, err := ListWorkspaceSourceFiles(context.Background(), rootPath, nil)
	if err != nil {
		return nil, err
	}
	for _, rel := range files {
		fullPath := filepath.Join(rootPath, rel)
		content, rErr := os.ReadFile(fullPath)
		if rErr == nil {
			violations = append(violations, v.ValidateContent(rel, string(content))...)
		}
	}
	return violations, nil
}

// ValidateContent scans a single file's string content for forbidden stubs, masks, and vacuum tests.
func (v *AntiStubValidator) ValidateContent(path string, content string) []AntiStubViolation {
	var violations []AntiStubViolation
	ext := strings.ToLower(filepath.Ext(path))
	isTestFile := strings.Contains(path, "test") || strings.HasPrefix(filepath.Base(path), "test_") || strings.HasSuffix(filepath.Base(path), "_test.go")

	lines := strings.Split(content, "\n")
	numLines := len(lines)

	for idx, line := range lines {
		lineNum := idx + 1
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}

		// Check universal forbidden TODO/FIXME markers
		if universalTodoRE.MatchString(trimmed) {
			violations = append(violations, AntiStubViolation{
				Path:    path,
				Line:    lineNum,
				Rule:    "forbidden_todo_stub",
				Snippet: trimmed,
			})
		}

		// Shell script checks
		if ext == ".sh" || strings.HasPrefix(trimmed, "#!/bin/") || strings.HasPrefix(trimmed, "#!/usr/bin/env bash") || strings.HasPrefix(trimmed, "#!/usr/bin/env sh") {
			if shellMaskTrueRE.MatchString(trimmed) {
				violations = append(violations, AntiStubViolation{
					Path:    path,
					Line:    lineNum,
					Rule:    "shell_error_suppression_mask",
					Snippet: trimmed,
				})
			}
			if shellMaskExit0RE.MatchString(trimmed) {
				violations = append(violations, AntiStubViolation{
					Path:    path,
					Line:    lineNum,
					Rule:    "shell_error_exit_mask",
					Snippet: trimmed,
				})
			}
			if shellSetPlusERE.MatchString(trimmed) {
				violations = append(violations, AntiStubViolation{
					Path:    path,
					Line:    lineNum,
					Rule:    "shell_disabled_errexit",
					Snippet: trimmed,
				})
			}
		}

		// Python checks
		if ext == ".py" {
			if pyRaiseNotImplRE.MatchString(trimmed) {
				violations = append(violations, AntiStubViolation{
					Path:    path,
					Line:    lineNum,
					Rule:    "python_not_implemented_stub",
					Snippet: trimmed,
				})
			}
			// Check for def function followed immediately by pass or ... (allowing Protocol/abstract definitions)
			if pyDefHeaderRE.MatchString(line) && !isProtocolOrAbstractClass(lines, idx) {
				// Look ahead to the next non-empty line
				for nextIdx := idx + 1; nextIdx < numLines; nextIdx++ {
					nextTrimmed := strings.TrimSpace(lines[nextIdx])
					if nextTrimmed == "" {
						continue
					}
					if pyStubBodyRE.MatchString(nextTrimmed) {
						// Check if there is another statement following or if it is purely a stub
						if isOnlyStatementInDef(lines, nextIdx) {
							violations = append(violations, AntiStubViolation{
								Path:    path,
								Line:    nextIdx + 1,
								Rule:    "python_empty_stub_function",
								Snippet: fmt.Sprintf("%s -> %s", trimmed, nextTrimmed),
							})
						}
					}
					break
				}
			}
		}

		// Go checks
		if ext == ".go" {
			if goPanicStubRE.MatchString(trimmed) {
				violations = append(violations, AntiStubViolation{
					Path:    path,
					Line:    lineNum,
					Rule:    "go_panic_stub",
					Snippet: trimmed,
				})
			}
		}

		// Rust checks
		if ext == ".rs" {
			if rustTodoStubRE.MatchString(trimmed) {
				violations = append(violations, AntiStubViolation{
					Path:    path,
					Line:    lineNum,
					Rule:    "rust_todo_stub",
					Snippet: trimmed,
				})
			}
		}

		// JS/TS checks
		if ext == ".js" || ext == ".ts" || ext == ".jsx" || ext == ".tsx" {
			if jsThrowNotImplRE.MatchString(trimmed) {
				violations = append(violations, AntiStubViolation{
					Path:    path,
					Line:    lineNum,
					Rule:    "javascript_not_implemented_stub",
					Snippet: trimmed,
				})
			}
		}

		// Makefile stub checks
		baseName := strings.ToLower(filepath.Base(path))
		if baseName == "makefile" || baseName == "gnumakefile" || ext == ".mk" {
			if strings.HasPrefix(trimmed, "test:") || strings.HasPrefix(trimmed, "build:") || strings.HasPrefix(trimmed, "e2e:") || strings.HasPrefix(trimmed, "check:") {
				hasRealRecipe := false
				hasEchoStub := false
				for nextIdx := idx + 1; nextIdx < numLines; nextIdx++ {
					nextRaw := lines[nextIdx]
					nextTrimmed := strings.TrimSpace(nextRaw)
					if nextTrimmed == "" || strings.HasPrefix(nextTrimmed, "#") {
						continue
					}
					if !strings.HasPrefix(nextRaw, "\t") && !strings.HasPrefix(nextRaw, "  ") {
						break
					}
					if makeStubEchoRE.MatchString(nextTrimmed) {
						hasEchoStub = true
					} else {
						hasRealRecipe = true
					}
				}
				if hasEchoStub && !hasRealRecipe {
					violations = append(violations, AntiStubViolation{
						Path:    path,
						Line:    lineNum,
						Rule:    "stub_makefile_target",
						Snippet: fmt.Sprintf("%s (target only echoes message or exits 0 without invoking real test/build tools)", trimmed),
					})
				}
			}
		}

		// Test file tautology checks
		if isTestFile {
			if tautologyPyRE.MatchString(trimmed) {
				violations = append(violations, AntiStubViolation{
					Path:    path,
					Line:    lineNum,
					Rule:    "tautological_test_assertion",
					Snippet: trimmed,
				})
			}
			if tautologyGoRE.MatchString(trimmed) {
				violations = append(violations, AntiStubViolation{
					Path:    path,
					Line:    lineNum,
					Rule:    "tautological_test_assertion",
					Snippet: trimmed,
				})
			}
		}
	}

	return violations
}

// isProtocolOrAbstractClass checks if the function definition is enclosed in a typing.Protocol or abc.ABC class.
func isProtocolOrAbstractClass(lines []string, defIdx int) bool {
	for i := defIdx - 1; i >= 0 && i >= defIdx-30; i-- {
		line := strings.TrimSpace(lines[i])
		if strings.HasPrefix(line, "class ") && (strings.Contains(line, "Protocol") || strings.Contains(line, "ABC") || strings.Contains(line, "abstractmethod")) {
			return true
		}
		if strings.HasPrefix(line, "@abstractmethod") {
			return true
		}
	}
	return false
}

// isOnlyStatementInDef checks if the pass/... is the only statement in the function body.
func isOnlyStatementInDef(lines []string, bodyIdx int) bool {
	if bodyIdx >= len(lines) {
		return true
	}
	baseIndent := getIndentation(lines[bodyIdx])
	for i := bodyIdx + 1; i < len(lines); i++ {
		trimmed := strings.TrimSpace(lines[i])
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		currIndent := getIndentation(lines[i])
		if currIndent >= baseIndent {
			// Found additional code inside same function body
			return false
		}
		// Dedented: function ended
		break
	}
	return true
}

func getIndentation(line string) int {
	indent := 0
	for _, ch := range line {
		switch ch {
		case ' ':
			indent++
		case '\t':
			indent += 4
		default:
			return indent
		}
	}
	return indent
}
