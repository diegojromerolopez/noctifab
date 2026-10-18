package services

import (
	"regexp"
	"strings"
)

var fileContextPrefixRE = regexp.MustCompile(`^File\s+([^\s:(]+)`)

// extractFilePathFromContext extracts the relative file path from a sliced file context block.
func extractFilePathFromContext(block string) string {
	trimmed := strings.TrimSpace(block)
	m := fileContextPrefixRE.FindStringSubmatch(trimmed)
	if len(m) > 1 {
		return strings.TrimSpace(m[1])
	}
	return ""
}

// DeduplicateFileAndReaderContexts ensures that source files appearing in fileContexts
// are not duplicated in readerContexts (or vice versa), while preserving workspace
// structures and project manifests.
func DeduplicateFileAndReaderContexts(fileContexts, readerContexts []string) ([]string, []string) {
	seenPaths := make(map[string]bool)

	var cleanFileContexts []string
	for _, fc := range fileContexts {
		p := extractFilePathFromContext(fc)
		if p != "" {
			if seenPaths[p] {
				continue
			}
			seenPaths[p] = true
		}
		cleanFileContexts = append(cleanFileContexts, fc)
	}

	var cleanReaderContexts []string
	for _, rc := range readerContexts {
		p := extractFilePathFromContext(rc)
		if p != "" {
			if seenPaths[p] {
				// Already present in fileContexts, skip duplicate full-file injection
				continue
			}
			seenPaths[p] = true
		}
		cleanReaderContexts = append(cleanReaderContexts, rc)
	}

	return cleanFileContexts, cleanReaderContexts
}

// PruneAndWindowToolOutputs eliminates superseded diagnostic tool logs (e.g. old test/linter failures)
// and windows older turns to compact summaries, preserving token bandwidth across long multi-turn sessions.
func PruneAndWindowToolOutputs(history []string, maxRecentTurns int) []string {
	if len(history) == 0 {
		return history
	}
	if maxRecentTurns <= 0 {
		maxRecentTurns = 2
	}

	result := make([]string, len(history))
	copy(result, history)

	// If there are multiple turns, check if the latest turn contains run_tests or run_linter.
	// If so, replace verbose outputs of those tools in earlier turns with compact references.
	hasRecentTests := false
	hasRecentLinter := false

	recentStart := len(result) - maxRecentTurns
	if recentStart < 0 {
		recentStart = 0
	}

	for i := recentStart; i < len(result); i++ {
		if strings.Contains(result[i], "Tool run_tests") || strings.Contains(result[i], "Speculative Fast Validation") {
			hasRecentTests = true
		}
		if strings.Contains(result[i], "Tool run_linter") {
			hasRecentLinter = true
		}
	}

	// Prune superseded tools from older turns (< len(result)-1)
	for i := 0; i < len(result)-1; i++ {
		lines := strings.Split(result[i], "\n")
		var prunedLines []string
		skipUntilNextTool := false

		for j := 0; j < len(lines); j++ {
			line := lines[j]
			isToolHeader := strings.HasPrefix(line, "Tool ") || strings.HasPrefix(line, "[Speculative Fast Validation]")

			if isToolHeader {
				skipUntilNextTool = false

				if hasRecentTests && (strings.HasPrefix(line, "Tool run_tests") || strings.HasPrefix(line, "[Speculative Fast Validation]")) {
					prunedLines = append(prunedLines, "Tool run_tests: (output superseded by subsequent test run)")
					skipUntilNextTool = true
					continue
				}

				if hasRecentLinter && strings.HasPrefix(line, "Tool run_linter") {
					prunedLines = append(prunedLines, "Tool run_linter: (output superseded by subsequent linter run)")
					skipUntilNextTool = true
					continue
				}
			}

			if skipUntilNextTool {
				// Skip multi-line stack trace / failure logs of superseded tool
				if strings.HasPrefix(line, "Tool ") || strings.HasPrefix(line, "--- Turn ") {
					skipUntilNextTool = false
					prunedLines = append(prunedLines, line)
				}
				continue
			}

			// For turns older than maxRecentTurns, truncate long output bodies (> 200 chars)
			if i < recentStart && strings.HasPrefix(line, "Output:") && j+1 < len(lines) {
				prunedLines = append(prunedLines, "Output: [Older turn output condensed to save tokens]")
				skipUntilNextTool = true
				continue
			}

			prunedLines = append(prunedLines, line)
		}
		result[i] = strings.Join(prunedLines, "\n")
	}

	return result
}
