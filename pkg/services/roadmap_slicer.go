package services

import (
	"strings"
)

// SliceSpecForRoadmap slices and compacts large technical specifications for
// the Product Manager agent's high-level roadmap decomposition pass.
//
// Exhaustive test matrices, granular test case catalogs, and raw harness configurations
// (which are needed downstream by Generator and Tester tasks) are replaced with compact
// architectural references. This ensures the PM agent receives the core architecture,
// domain models, and quality gates without blowing past LLM context envelopes or token limits.
func SliceSpecForRoadmap(spec string) string {
	if len(spec) < 10000 {
		return spec
	}

	lines := strings.Split(spec, "\n")
	var result strings.Builder
	result.Grow(len(spec) / 2)

	inExhaustiveTestSection := false
	testSectionLevel := 0
	inLargeCodeBlock := false
	codeBlockLineCount := 0

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)

		// Detect markdown header
		if strings.HasPrefix(trimmed, "#") {
			lower := strings.ToLower(trimmed)
			headerLevel := 0
			for _, r := range trimmed {
				if r == '#' {
					headerLevel++
				} else {
					break
				}
			}

			if inExhaustiveTestSection && headerLevel <= testSectionLevel {
				inExhaustiveTestSection = false
			}

			// Check if this header introduces an exhaustive test matrix
			if !inExhaustiveTestSection && (strings.Contains(lower, "exhaustive") ||
				strings.Contains(lower, "conformance testset matrix") ||
				strings.Contains(lower, "test matrix") ||
				strings.Contains(lower, "testcase matrix")) {
				inExhaustiveTestSection = true
				testSectionLevel = headerLevel
				result.WriteString(line)
				result.WriteString("\n\n*(Exhaustive per-test matrices and assertions are preserved in SPEC.md for downstream task implementation)*\n\n")
				continue
			}
		}

		if inExhaustiveTestSection {
			// Skip granular subsection content inside the exhaustive test matrix
			continue
		}

		// Check code block fences
		if strings.HasPrefix(trimmed, "```") {
			if !inLargeCodeBlock {
				inLargeCodeBlock = true
				codeBlockLineCount = 0
			} else {
				inLargeCodeBlock = false
				codeBlockLineCount = 0
			}
		} else if inLargeCodeBlock {
			codeBlockLineCount++
			if codeBlockLineCount > 40 {
				if codeBlockLineCount == 41 {
					result.WriteString("    # ... [listing truncated for roadmap decomposition] ...\n")
				}
				continue
			}
		}

		result.WriteString(line)
		result.WriteString("\n")
	}

	return strings.TrimSpace(result.String())
}
