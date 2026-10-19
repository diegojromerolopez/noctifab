package services

import (
	"regexp"
	"strings"
)

var packageInstallClauseRegex = regexp.MustCompile(`(?i)(?:^|[;&|]\s*)(?:opam\s+install|npm\s+(?:install|i|add)|pip\s+install|pip3\s+install|cargo\s+install|gem\s+install|go\s+install|apt(?:-get)?\s+install|brew\s+install|yarn\s+add|pnpm\s+add)\b[^;&|]*`)

// SanitizeToolCommand inspects a format, lint, or tool command and strips any embedded
// package installation clauses (e.g. "opam install ocamlformat && dune fmt" -> "dune fmt").
// If the command consists entirely of package installation directives, it returns "" to safely bypass execution.
func SanitizeToolCommand(cmd string) string {
	trimmed := strings.TrimSpace(cmd)
	if trimmed == "" {
		return ""
	}

	cleaned := packageInstallClauseRegex.ReplaceAllString(trimmed, "")
	cleaned = strings.TrimSpace(cleaned)

	// Clean up any dangling leading/trailing shell operators (&&, ||, ;, |)
	for {
		prev := cleaned
		cleaned = strings.TrimPrefix(cleaned, "&&")
		cleaned = strings.TrimPrefix(cleaned, "||")
		cleaned = strings.TrimPrefix(cleaned, ";")
		cleaned = strings.TrimPrefix(cleaned, "|")
		cleaned = strings.TrimSuffix(cleaned, "&&")
		cleaned = strings.TrimSuffix(cleaned, "||")
		cleaned = strings.TrimSuffix(cleaned, ";")
		cleaned = strings.TrimSuffix(cleaned, "|")
		cleaned = strings.TrimSpace(cleaned)
		if cleaned == prev {
			break
		}
	}

	return cleaned
}
