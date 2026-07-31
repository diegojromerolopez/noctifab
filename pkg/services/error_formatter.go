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
	// Matches file.ext:line:col: message or file.ext:line: message
	goErrorRegex     = regexp.MustCompile(`(?m)^([a-zA-Z0-9_\-/\\]+\.[a-zA-Z0-9]+):(\d+)(?::(\d+))?:\s*(.+)$`)
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

	// Linter matches
	linterMatches := linterRegex.FindAllStringSubmatch(rawLog, -1)
	for _, m := range linterMatches {
		if len(m) >= 5 {
			key := fmt.Sprintf("%s:%s", m[1], m[2])
			if !seen[key] {
				seen[key] = true
				results = append(results, DiagnosticError{
					FilePath:   m[1],
					LineNumber: m[2],
					Message:    fmt.Sprintf("[%s] %s", m[3], m[4]),
					Category:   "Linter Violation",
				})
			}
		}
	}

	// Go / C / Compiler matches
	goMatches := goErrorRegex.FindAllStringSubmatch(rawLog, -1)
	for _, m := range goMatches {
		if len(m) >= 5 {
			key := fmt.Sprintf("%s:%s", m[1], m[2])
			if !seen[key] {
				seen[key] = true
				results = append(results, DiagnosticError{
					FilePath:   m[1],
					LineNumber: m[2],
					Column:     m[3],
					Message:    m[4],
					Category:   "Compiler / Syntax Error",
				})
			}
		}
	}

	// Pytest / Python matches
	pyMatches := pythonErrorRegex.FindAllStringSubmatch(rawLog, -1)
	for _, m := range pyMatches {
		if len(m) >= 4 {
			key := fmt.Sprintf("%s:%s", m[1], m[2])
			if !seen[key] {
				seen[key] = true
				results = append(results, DiagnosticError{
					FilePath:   m[1],
					LineNumber: m[2],
					Message:    fmt.Sprintf("In %s", m[3]),
					Category:   "Python Traceback",
				})
			}
		}
	}

	pytestMatches := pytestErrorRegex.FindAllStringSubmatch(rawLog, -1)
	for _, m := range pytestMatches {
		if len(m) >= 4 {
			key := fmt.Sprintf("%s:%s", m[1], m[2])
			if !seen[key] {
				seen[key] = true
				results = append(results, DiagnosticError{
					FilePath: m[1],
					Message:  fmt.Sprintf("Failed test '%s': %s", m[2], m[3]),
					Category: "Test Failure",
				})
			}
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
