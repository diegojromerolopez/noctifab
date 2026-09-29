package services

import (
	"path/filepath"
	"regexp"
	"strings"

	"github.com/diegojromerolopez/noctifab/pkg/domain"
)

var (
	// pytestFailureRE captures failed test paths like "FAILED tests/integration/test_hashes.py::test_hset"
	pytestFailureRE = regexp.MustCompile(`(?:FAILED|ERROR)\s+([^\s:]+(?:::[\w]+)?)`)
	// pyTracebackRE captures python traceback frames like "tests/integration/test_hashes.py:42: in test_hset"
	pyTracebackRE = regexp.MustCompile(`([\w/.-]+\.py):[0-9]+:`)

	// goFailRE captures Go test failures like "--- FAIL: TestHashes"
	goFailRE = regexp.MustCompile(`---\s+FAIL:\s+([\w/]+)`)
	// goFileRE captures Go test source references like "hashes_test.go:42:"
	goFileRE = regexp.MustCompile(`([\w/.-]+_test\.go):[0-9]+:`)

	// rustFailRE captures Cargo test failures like "test e2e::test_hashes ... FAILED"
	rustFailRE = regexp.MustCompile(`test\s+([\w:]+)\s+\.\.\.\s+FAILED`)

	// genericFailRE captures generic "FAIL: <target>" or "FAILURE in <target>"
	genericFailRE = regexp.MustCompile(`(?i)(?:FAIL|FAILURE|ERROR)(?:\s+in\s+|:\s*)([\w/.-]+)`)
)

// extractFailingTargets parses test runner output and returns the identifiers
// (file paths, test names, or modules) that suffered failures.
func extractFailingTargets(output string) []string {
	var targets []string
	seen := make(map[string]bool)

	add := func(t string) {
		t = strings.TrimSpace(t)
		if t != "" && !seen[t] {
			seen[t] = true
			targets = append(targets, t)
		}
	}

	for _, match := range pytestFailureRE.FindAllStringSubmatch(output, -1) {
		if len(match) > 1 {
			add(match[1])
		}
	}

	for _, match := range pyTracebackRE.FindAllStringSubmatch(output, -1) {
		if len(match) > 1 {
			add(match[1])
		}
	}

	for _, match := range goFailRE.FindAllStringSubmatch(output, -1) {
		if len(match) > 1 {
			add(match[1])
		}
	}

	for _, match := range goFileRE.FindAllStringSubmatch(output, -1) {
		if len(match) > 1 {
			add(match[1])
		}
	}

	for _, match := range rustFailRE.FindAllStringSubmatch(output, -1) {
		if len(match) > 1 {
			add(match[1])
		}
	}

	for _, match := range genericFailRE.FindAllStringSubmatch(output, -1) {
		if len(match) > 1 {
			add(match[1])
		}
	}

	return targets
}

func isGenericStopWord(w string) bool {
	switch w {
	case "test", "tests", "testing", "implement", "implementation", "support",
		"feature", "task", "core", "verify", "verification", "check", "setup",
		"write", "create", "make", "with", "from", "into", "that", "this",
		"service", "runner", "code", "file", "files", "unit", "spec":
		return true
	default:
		return false
	}
}

func collectFeatureScopeTokens(state *domain.State, task domain.Task) map[string]bool {
	tokens := make(map[string]bool)

	// Add Story ID (e.g. "us-001", "001")
	if task.StoryID != "" {
		sID := strings.ToLower(task.StoryID)
		tokens[sID] = true
		tokens[strings.TrimPrefix(sID, "us-")] = true
		tokens[strings.TrimPrefix(sID, "us")] = true
	}

	// Add TargetFiles basenames and stems
	for _, tf := range task.TargetFiles {
		clean := filepath.Clean(tf)
		tokens[strings.ToLower(clean)] = true
		base := filepath.Base(clean)
		tokens[strings.ToLower(base)] = true
		ext := filepath.Ext(base)
		stem := strings.TrimSuffix(base, ext)
		tokens[strings.ToLower(stem)] = true
		// Strip common test prefixes/suffixes
		tokens[strings.ToLower(strings.TrimPrefix(stem, "test_"))] = true
		tokens[strings.ToLower(strings.TrimSuffix(stem, "_test"))] = true
	}

	// Add tokens from task title (words >= 4 letters or "e2e" that are not stop words)
	for _, word := range strings.Fields(task.Title) {
		cleanWord := strings.ToLower(strings.Trim(word, ":;,.-_()[]{}'\""))
		if (len(cleanWord) >= 4 || cleanWord == "e2e") && !isGenericStopWord(cleanWord) {
			tokens[cleanWord] = true
		}
	}

	// Add FeatureName from state metadata if available
	if state != nil && state.Metadata.FeatureName != "" {
		fn := strings.ToLower(state.Metadata.FeatureName)
		tokens[fn] = true
		tokens[strings.TrimPrefix(fn, "us-")] = true
		tokens[strings.TrimPrefix(fn, "us")] = true
	}

	return tokens
}

// isE2EFailureInScope determines whether any of the failures detected in an E2E test run
// belong to the active feature / task being developed. If all failures belong to
// out-of-scope features (e.g. downstream unimplemented endpoints), it returns false.
func isE2EFailureInScope(state *domain.State, task domain.Task, output string) bool {
	// Remediation and sovereign rescue tasks are explicitly dedicated to fixing
	// broken integration, acceptance, and E2E failures; all failures are in-scope.
	if strings.HasPrefix(task.ID, "qa-remediation-") ||
		strings.HasPrefix(task.ID, "spec-remediation-") ||
		strings.HasPrefix(task.ID, "sovereign-rescue-") {
		return true
	}

	scopeTokens := collectFeatureScopeTokens(state, task)
	failingTargets := extractFailingTargets(output)

	// If no specific failing targets could be parsed (e.g. fatal syntax crash, missing binary),
	// check if any scope token appears near an error line in the output.
	if len(failingTargets) == 0 {
		lowerOut := strings.ToLower(output)
		for token := range scopeTokens {
			if len(token) >= 3 && strings.Contains(lowerOut, token) {
				return true
			}
		}
		// If target files were modified and compilation/server boot failed completely, consider in-scope
		return true
	}

	// Check if ANY parsed failing target matches our feature scope
	for _, target := range failingTargets {
		lowerTarget := strings.ToLower(target)
		for token := range scopeTokens {
			if token == "" {
				continue
			}
			if strings.Contains(lowerTarget, token) {
				return true
			}
		}
	}

	// All identified failures are out of scope for this feature
	return false
}
