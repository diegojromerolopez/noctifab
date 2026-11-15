package services

import (
	"fmt"
	"regexp"
	"strings"
)

// DiagnosticError represents a structured error extracted from build/test/linter output.
type DiagnosticError struct {
	FilePath   string
	LineNumber string
	Column     string
	Message    string
	Category   string
}

var (
	// Matches file.ext:line:col: message or file.ext:line: message (Go, C, generic)
	goErrorRegex = regexp.MustCompile(`(?m)^([a-zA-Z0-9_\-/\\]+\.[a-zA-Z0-9]+):(\d+)(?::(\d+))?:\s*(.+)$`)

	// C# Roslyn: Board.cs(20,50): error CS0206: A property...
	csharpErrorRegex = regexp.MustCompile(`(?m)^\s*([a-zA-Z0-9_\-/\\]+\.cs)\((\d+)(?:,(\d+))?\):\s*(?:error|warning)\s+([A-Z0-9]+:\s*.+)$`)

	// Rust rustc / cargo: --> src/main.rs:12:5
	rustLocationRegex = regexp.MustCompile(`(?m)^\s*-->\s*([a-zA-Z0-9_\-/\\]+\.rs):(\d+)(?::(\d+))?`)
	rustErrorMsgRegex = regexp.MustCompile(`(?m)^\s*error(?:\[[A-Z0-9]+\])?:\s*(.+)$`)

	// OCaml Dune / ocamlc: File "lib/engine.ml", line 44, characters 12-25:
	ocamlErrorRegex = regexp.MustCompile(`(?m)File "([^"]+\.ml[i]?)", line (\d+)(?:, characters (\d+)-\d+)?:(?:\s*\n)?\s*(?:Error:\s*(.+)|(.+))?`)

	// Java / Kotlin: App.java:23: error: ... or App.java:[23,15] error: ...
	javaErrorRegex = regexp.MustCompile(`(?m)^\s*([a-zA-Z0-9_\-/\\]+\.(?:java|kt)):\[?(\d+)(?:,(\d+))?\]?:\s*(error:\s*.+)$`)

	// TypeScript / JavaScript: src/index.ts:14:5 - error TS2322: ...
	typeScriptErrorRegex = regexp.MustCompile(`(?m)^\s*([a-zA-Z0-9_\-/\\]+\.(?:ts|tsx|js|jsx)):(\d+):(\d+)\s*-\s*error\s+([A-Z0-9]+:\s*.+)$`)

	// Make missing target: make: *** No rule to make target 'build'. Stop.
	makeMissingTargetRegex = regexp.MustCompile(`(?m)make(?:\s*\[\d+\])?:\s*\*\*\*\s*No rule to make target ['"]?([^'",\s]+)['"]?`)

	// Cargo unresolvable dependency: failed to select a version for the requirement '...'
	cargoDependencyRegex = regexp.MustCompile(`(?m)failed to select a version for the requirement ['"]?([^'"]+)['"]?`)

	// Python traceback and pytest
	pythonErrorRegex = regexp.MustCompile(`(?m)File "([^"]+)", line (\d+), in (.+)`)
	pytestErrorRegex = regexp.MustCompile(`(?m)^FAILED ([^:]+)::(\S+) - (.+)$`)
	linterRegex      = regexp.MustCompile(`(?m)^([a-zA-Z0-9_\-/\\]+\.[a-zA-Z0-9]+):(\d+):\d+:\s*\[([^\]]+)\]\s*(.+)$`)
)

// FormatStructuredErrorFeedback parses raw log output and constructs an instructive,
// structured feedback string highlighting exact error lines and actionable repair steps.
func FormatStructuredErrorFeedback(rawLog string) string {
	if strings.TrimSpace(rawLog) == "" {
		return "No error output captured."
	}

	diagnostics := extractDiagnostics(rawLog)
	var sb strings.Builder

	sb.WriteString("=== STRUCTURED DIAGNOSTIC FEEDBACK ===\n")
	if len(diagnostics) > 0 {
		sb.WriteString("Structured File Error Breakdown:\n")
		for _, d := range diagnostics {
			loc := d.FilePath
			if d.LineNumber != "" {
				loc += fmt.Sprintf(" (Line %s", d.LineNumber)
				if d.Column != "" {
					loc += fmt.Sprintf(", Col %s", d.Column)
				}
				loc += ")"
			}
			fmt.Fprintf(&sb, "- Location: %s\n  Category: %s\n  Message: %s\n", loc, d.Category, d.Message)
		}
		sb.WriteString("\nInstructive Directives:\n")
		sb.WriteString("1. Modify ONLY the target files listed above at the specified line numbers.\n")
		sb.WriteString("2. Ensure all referenced symbols, imports, and variables are explicitly defined.\n")
		sb.WriteString("3. Re-run tests immediately after making line adjustments.\n\n")
	}

	sb.WriteString("Raw Output Snippet (Tail):\n")
	sb.WriteString(extractTailLog(rawLog, 15))
	return sb.String()
}

func extractDiagnostics(rawLog string) []DiagnosticError {
	var results []DiagnosticError
	seen := make(map[string]bool)

	addResult := func(d DiagnosticError) {
		key := fmt.Sprintf("%s:%s:%s", d.FilePath, d.LineNumber, d.Message)
		if !seen[key] {
			seen[key] = true
			results = append(results, d)
		}
	}

	// 1. C# Roslyn errors
	for _, m := range csharpErrorRegex.FindAllStringSubmatch(rawLog, -1) {
		if len(m) >= 5 {
			col := ""
			if len(m) > 3 {
				col = m[3]
			}
			addResult(DiagnosticError{
				FilePath:   m[1],
				LineNumber: m[2],
				Column:     col,
				Message:    m[4],
				Category:   "C# Compiler Error",
			})
		}
	}

	// 2. Rust Cargo / rustc errors
	rustLocs := rustLocationRegex.FindAllStringSubmatch(rawLog, -1)
	rustMsgs := rustErrorMsgRegex.FindAllStringSubmatch(rawLog, -1)
	for i, m := range rustLocs {
		if len(m) >= 3 {
			col := ""
			if len(m) > 3 {
				col = m[3]
			}
			msg := "rustc compiler error"
			if i < len(rustMsgs) && len(rustMsgs[i]) > 1 {
				msg = rustMsgs[i][1]
			} else if len(rustMsgs) > 0 && len(rustMsgs[0]) > 1 {
				msg = rustMsgs[0][1]
			}
			addResult(DiagnosticError{
				FilePath:   m[1],
				LineNumber: m[2],
				Column:     col,
				Message:    msg,
				Category:   "Rust Compiler Error",
			})
		}
	}

	// 3. OCaml Dune / ocamlc errors
	for _, m := range ocamlErrorRegex.FindAllStringSubmatch(rawLog, -1) {
		if len(m) >= 3 {
			col := ""
			if len(m) > 3 {
				col = m[3]
			}
			msg := "Syntax/Type error"
			if len(m) > 4 && m[4] != "" {
				msg = m[4]
			} else if len(m) > 5 && m[5] != "" {
				msg = m[5]
			}
			addResult(DiagnosticError{
				FilePath:   m[1],
				LineNumber: m[2],
				Column:     col,
				Message:    strings.TrimSpace(msg),
				Category:   "OCaml Compiler Error",
			})
		}
	}

	// 4. Java / Kotlin errors
	for _, m := range javaErrorRegex.FindAllStringSubmatch(rawLog, -1) {
		if len(m) >= 4 {
			col := ""
			if len(m) > 3 {
				col = m[3]
			}
			msg := m[len(m)-1]
			addResult(DiagnosticError{
				FilePath:   m[1],
				LineNumber: m[2],
				Column:     col,
				Message:    msg,
				Category:   "Java Compiler Error",
			})
		}
	}

	// 5. TypeScript / JavaScript errors
	for _, m := range typeScriptErrorRegex.FindAllStringSubmatch(rawLog, -1) {
		if len(m) >= 5 {
			addResult(DiagnosticError{
				FilePath:   m[1],
				LineNumber: m[2],
				Column:     m[3],
				Message:    m[4],
				Category:   "TypeScript Compiler Error",
			})
		}
	}

	// 6. Make missing targets
	for _, m := range makeMissingTargetRegex.FindAllStringSubmatch(rawLog, -1) {
		if len(m) >= 2 {
			addResult(DiagnosticError{
				FilePath:   "Makefile",
				LineNumber: "1",
				Message:    fmt.Sprintf("Missing Makefile target '%s'", m[1]),
				Category:   "Build Manifest Error",
			})
		}
	}

	// 7. Cargo unresolvable dependency
	for _, m := range cargoDependencyRegex.FindAllStringSubmatch(rawLog, -1) {
		if len(m) >= 2 {
			addResult(DiagnosticError{
				FilePath:   "Cargo.toml",
				LineNumber: "1",
				Message:    fmt.Sprintf("Unresolvable dependency '%s'", m[1]),
				Category:   "Manifest Dependency Error",
			})
		}
	}

	// 8. Linter matches
	for _, m := range linterRegex.FindAllStringSubmatch(rawLog, -1) {
		if len(m) >= 5 {
			addResult(DiagnosticError{
				FilePath:   m[1],
				LineNumber: m[2],
				Message:    fmt.Sprintf("[%s] %s", m[3], m[4]),
				Category:   "Linter Violation",
			})
		}
	}

	// 9. Go / C / Compiler matches
	for _, m := range goErrorRegex.FindAllStringSubmatch(rawLog, -1) {
		if len(m) >= 5 {
			addResult(DiagnosticError{
				FilePath:   m[1],
				LineNumber: m[2],
				Column:     m[3],
				Message:    m[4],
				Category:   "Compiler / Syntax Error",
			})
		}
	}

	// 10. Python Traceback matches
	for _, m := range pythonErrorRegex.FindAllStringSubmatch(rawLog, -1) {
		if len(m) >= 4 {
			addResult(DiagnosticError{
				FilePath:   m[1],
				LineNumber: m[2],
				Message:    fmt.Sprintf("In %s", m[3]),
				Category:   "Python Traceback",
			})
		}
	}

	// 11. Pytest matches
	for _, m := range pytestErrorRegex.FindAllStringSubmatch(rawLog, -1) {
		if len(m) >= 4 {
			addResult(DiagnosticError{
				FilePath: m[1],
				Message:  fmt.Sprintf("Failed test '%s': %s", m[2], m[3]),
				Category: "Test Failure",
			})
		}
	}

	return results
}

func extractTailLog(log string, maxLines int) string {
	lines := strings.Split(log, "\n")
	if len(lines) <= maxLines {
		return log
	}
	return strings.Join(lines[len(lines)-maxLines:], "\n")
}
