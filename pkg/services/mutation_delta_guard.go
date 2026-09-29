package services

import (
	"fmt"
	"regexp"
	"strings"
)

var (
	// Matches scalar integer assertion comparisons: e.g. assertEqual(8, ...), assert res == 8, assertEqual(b":8\r\n", ...)
	scalarAssertionRE = regexp.MustCompile(`(?i)(?:assertEqual\s*\(\s*(?:b?["']:\d+\\r\\n["']|\d+)\s*,|assert\s+[a-zA-Z0-9_\(\)]+\s*==\s*\d+)`)
	// Common mutation verbs indicating container/collection state modification
	mutationVerbRE = regexp.MustCompile(`(?i)\b(?:append|push|add|insert|set|incr|decr|remove|delete|pop|write|put)\b`)
)

// MutationDeltaViolation captures an assertion on a state-mutating operation that lacks delta verification.
type MutationDeltaViolation struct {
	FilePath   string
	LineNumber int
	Line       string
	Reason     string
}

func (v MutationDeltaViolation) Error() string {
	return fmt.Sprintf("mutation delta invariant violation at %s:%d: %s (%s)", v.FilePath, v.LineNumber, v.Line, v.Reason)
}

// MutationDeltaGuard enforces that authored tests for mutating operations verify
// state delta invariants (pre-state vs post-state delta or resulting length) rather
// than asserting uncorroborated magic scalar returns.
type MutationDeltaGuard struct{}

// NewMutationDeltaGuard creates a new MutationDeltaGuard.
func NewMutationDeltaGuard() *MutationDeltaGuard {
	return &MutationDeltaGuard{}
}

// ValidateMutationDeltaAssertions inspects test file content and detects raw scalar
// assertions on mutating operations that lack state-delta corroboration.
func (g *MutationDeltaGuard) ValidateMutationDeltaAssertions(filePath, content string) []MutationDeltaViolation {
	if strings.TrimSpace(content) == "" {
		return nil
	}

	var violations []MutationDeltaViolation
	lines := strings.Split(content, "\n")

	for idx, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "#") || strings.HasPrefix(trimmed, "//") {
			continue
		}

		if scalarAssertionRE.MatchString(trimmed) && mutationVerbRE.MatchString(trimmed) {
			// If assertion does not include descriptive msg= or state verification context
			if !strings.Contains(trimmed, "msg=") && !strings.Contains(trimmed, "delta") && !strings.Contains(trimmed, "len(") && !strings.Contains(trimmed, "size") {
				violations = append(violations, MutationDeltaViolation{
					FilePath:   filePath,
					LineNumber: idx + 1,
					Line:       trimmed,
					Reason:     "scalar return on mutating operation lacks state-delta invariant corroboration (delta or length assertion)",
				})
			}
		}
	}

	return violations
}

// FormatDeltaInvariantTemplate produces an algebraic pre/post state assertion template
// for inclusion in authored unit or integration tests.
func (g *MutationDeltaGuard) FormatDeltaInvariantTemplate(opName, targetKey, argVal string) string {
	return fmt.Sprintf(
		"# Invariant: return value must equal the post-mutation length/size\n"+
			"pre_len = len(store.get(%q) or b\"\")\n"+
			"res = execute(%q, %q, %s)\n"+
			"post_len = len(store.get(%q))\n"+
			"assert post_len - pre_len == expected_delta, f\"Delta mismatch: expected {expected_delta}, got {post_len - pre_len}\"",
		targetKey, opName, targetKey, argVal, targetKey,
	)
}
