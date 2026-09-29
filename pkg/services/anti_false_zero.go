package services

import (
	"bufio"
	"regexp"
	"strings"
)

// FalseZeroViolation records details of a process that exited with 0 despite fatal crashes or signal aborts.
type FalseZeroViolation struct {
	Detected    bool   `json:"detected"`
	Signal      string `json:"signal"`
	Reason      string `json:"reason"`
	MatchedLine string `json:"matched_line"`
	LineNumber  int    `json:"line_number"`
}

type crashSignalMatcher struct {
	signal  string
	pattern *regexp.Regexp
	reason  string
}

var crashSignalMatchers = []crashSignalMatcher{
	{
		signal:  "FATAL_PYTHON",
		pattern: regexp.MustCompile(`\bFatal\s+Python\s+error:\s+|\bINTERNALERROR>\s+`),
		reason:  "Python interpreter fatal crash or pytest internal error",
	},
	{
		signal:  "PANIC_GO",
		pattern: regexp.MustCompile(`^panic:\s+(?:runtime\s+error:|[^\n]+)|^fatal\s+error:\s+(?:concurrent\s+map|all\s+goroutines\s+are\s+asleep|stack\s+overflow)`),
		reason:  "Go runtime fatal panic or unhandled concurrency crash",
	},
	{
		signal:  "PANIC_RUST",
		pattern: regexp.MustCompile(`\bthread\s+'[^']+'\s+panicked\s+at\b|\bfatal\s+runtime\s+error:\s+`),
		reason:  "Rust runtime panic or fatal abort",
	},
	{
		signal:  "SIGSEGV",
		pattern: regexp.MustCompile(`(?i)\b(?:segmentation\s+fault(?:\s*\(core\s+dumped\))?|SIGSEGV)\b`),
		reason:  "Memory access violation / segmentation fault occurred",
	},
	{
		signal:  "SIGBUS",
		pattern: regexp.MustCompile(`(?i)\b(?:bus\s+error(?:\s*\(core\s+dumped\))?|SIGBUS)\b`),
		reason:  "Hardware bus error occurred",
	},
	{
		signal:  "SIGABRT",
		pattern: regexp.MustCompile(`(?i)\b(?:aborted\s*\(core\s+dumped\)|SIGABRT)\b`),
		reason:  "Process aborted unexpectedly",
	},
	{
		signal:  "SIGILL",
		pattern: regexp.MustCompile(`(?i)\b(?:illegal\s+instruction(?:\s*\(core\s+dumped\))?|SIGILL)\b`),
		reason:  "Illegal CPU instruction executed",
	},
	{
		signal:  "OOM",
		pattern: regexp.MustCompile(`(?i)\b(?:out\s+of\s+memory|OOMKilled|JavaScript\s+heap\s+out\s+of\s+memory)\b`),
		reason:  "Process terminated due to Out Of Memory (OOM)",
	},
	{
		signal:  "SANITIZER",
		pattern: regexp.MustCompile(`\b(?:AddressSanitizer:\s+DEADLYSIGNAL|heap-buffer-overflow|double\s+free\s+or\s+corruption|free\(\):\s+invalid\s+pointer)\b`),
		reason:  "Memory corruption or address sanitizer deadly signal",
	},
}

// DetectFalseZeroExit inspects command output when exit code was reported as 0.
// It returns a violation if fatal crash signatures or unhandled process signals were masked.
func DetectFalseZeroExit(output string, exitCode int) FalseZeroViolation {
	if exitCode != 0 {
		return FalseZeroViolation{Detected: false}
	}

	scanner := bufio.NewScanner(strings.NewReader(output))
	lineNo := 0

	for scanner.Scan() {
		lineNo++
		line := scanner.Text()
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}

		if isLikelyAssertionOrSourceComment(trimmed) {
			continue
		}

		for _, matcher := range crashSignalMatchers {
			if matcher.pattern.MatchString(trimmed) {
				return FalseZeroViolation{
					Detected:    true,
					Signal:      matcher.signal,
					Reason:      matcher.reason,
					MatchedLine: trimmed,
					LineNumber:  lineNo,
				}
			}
		}
	}

	return FalseZeroViolation{Detected: false}
}

// FormatFalseZeroDiagnostic produces a clear diagnostic message for LLM agents.
func FormatFalseZeroDiagnostic(v FalseZeroViolation) string {
	var sb strings.Builder
	sb.WriteString("\n[Anti-False-Zero Validator] REJECTED FALSE-POSITIVE PASS:\n")
	sb.WriteString("Process reported exit code 0, but execution aborted with fatal runtime crash or signal:\n")
	sb.WriteString("  Signal: ")
	sb.WriteString(v.Signal)
	sb.WriteString("\n  Reason: ")
	sb.WriteString(v.Reason)
	sb.WriteString("\n  Line: \"")
	sb.WriteString(v.MatchedLine)
	sb.WriteString("\"\n  Diagnostic: A fatal crash occurred and was masked by shell scripts or wrappers (e.g. '|| true' or subshell trap).\n")
	return sb.String()
}

func isLikelyAssertionOrSourceComment(line string) bool {
	// Skip comments
	if strings.HasPrefix(line, "//") || strings.HasPrefix(line, "#") || strings.HasPrefix(line, "*") {
		return true
	}
	// Skip unit test assertions that check for crash error strings
	lower := strings.ToLower(line)
	if strings.Contains(lower, "assert") ||
		strings.Contains(lower, "expect(") ||
		strings.Contains(lower, "require.") ||
		strings.Contains(lower, "should contain") ||
		strings.Contains(lower, "should.contain") {
		return true
	}
	// Skip string literal definitions
	if (strings.HasPrefix(line, "\"") && strings.HasSuffix(line, "\",")) ||
		(strings.HasPrefix(line, "'") && strings.HasSuffix(line, "',")) {
		return true
	}
	return false
}
