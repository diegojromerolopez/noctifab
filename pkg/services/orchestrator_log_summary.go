package services

import (
	"fmt"
	"strings"
)

// summarizeFailureLog extracts high-signal error and failure lines from a raw diagnostic log trace,
// truncating unbounded failure cascades to prevent prompt context explosion.
func summarizeFailureLog(log string) string {
	lines := strings.Split(log, "\n")
	var importantLines []string
	capture := false
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "ERROR:") || strings.HasPrefix(trimmed, "FAIL:") || strings.Contains(line, "Anti-stub") {
			capture = true
		}
		if capture {
			importantLines = append(importantLines, line)
		} else if strings.Contains(line, "Error:") || strings.Contains(line, "Exception") || strings.Contains(line, "FAILED") || strings.Contains(line, "error:") || strings.Contains(line, "Anti-stub") {
			importantLines = append(importantLines, line)
		}
	}

	if len(importantLines) == 0 {
		// Fallback to last 15 lines if no specific failures are captured
		start := len(lines) - 15
		if start < 0 {
			start = 0
		}
		return strings.Join(lines[start:], "\n")
	}

	const maxImportantLines = 80
	if len(importantLines) > maxImportantLines {
		truncated := make([]string, 0, maxImportantLines+1)
		truncated = append(truncated, importantLines[:30]...)
		truncated = append(truncated, fmt.Sprintf("...[%d diagnostic error lines truncated]...", len(importantLines)-maxImportantLines))
		truncated = append(truncated, importantLines[len(importantLines)-50:]...)
		importantLines = truncated
	}

	result := strings.Join(importantLines, "\n")
	const maxChars = 4000
	if len(result) > maxChars {
		result = result[:2000] + "\n...[error trace truncated]...\n" + result[len(result)-1900:]
	}
	return result
}
