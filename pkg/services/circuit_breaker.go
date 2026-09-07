package services

import (
	"path/filepath"
	"strings"
)

// TaskCircuitBreaker monitors agent activity on a task to detect and break
// generator-tester oscillation loops (Task 2 churn). When production logic
// is already stable and passing tests, it stops redundant cosmetic test edits
// and triggers automated task completion.
type TaskCircuitBreaker struct {
	ConsecutiveTestPasses           int  `json:"consecutive_test_passes"`
	ConsecutiveTestOnlyTurns        int  `json:"consecutive_test_only_turns"`
	ConsecutiveDuplicateInspections int  `json:"consecutive_duplicate_inspections"`
	IsTripped                       bool `json:"is_tripped"`
}

// NewTaskCircuitBreaker creates an initialized circuit breaker in the closed state.
func NewTaskCircuitBreaker() *TaskCircuitBreaker {
	return &TaskCircuitBreaker{}
}

// isProductionFile checks whether a file path belongs to production source code
// rather than tests, specifications, or documentation.
func isProductionFile(path string) bool {
	clean := filepath.ToSlash(filepath.Clean(strings.TrimSpace(path)))
	lower := strings.ToLower(clean)
	base := filepath.Base(lower)

	// Exclude tests, specs, roadmaps, documentation, dotfiles/configuration, and logs
	if strings.Contains(lower, "test") ||
		strings.Contains(lower, "spec") ||
		strings.HasPrefix(lower, "roadmap/") ||
		strings.HasSuffix(lower, ".md") ||
		strings.HasSuffix(lower, ".txt") ||
		strings.HasPrefix(base, ".") ||
		strings.HasSuffix(lower, ".log") ||
		strings.HasSuffix(lower, ".lock") ||
		base == "readme" || base == "license" {
		return false
	}

	return true
}

// RecordAction updates circuit breaker state when an action is executed.
func (cb *TaskCircuitBreaker) RecordAction(tool string, args map[string]any) {
	if cb == nil {
		return
	}

	// We only track file mutating tools
	if !IsMutatingTool(tool) {
		return
	}
	cb.ConsecutiveDuplicateInspections = 0

	path, _ := args["path"].(string)
	if path == "" {
		// Check for write_files batch tool
		if files, ok := args["files"].([]any); ok && len(files) > 0 {
			hasProd := false
			for _, f := range files {
				if fMap, ok := f.(map[string]any); ok {
					if p, ok := fMap["path"].(string); ok && isProductionFile(p) {
						hasProd = true
						break
					}
				}
			}
			if hasProd {
				cb.Reset()
				return
			}
			cb.ConsecutiveTestOnlyTurns++
			return
		}
		return
	}

	if isProductionFile(path) {
		// Business logic changed; reset the circuit breaker
		cb.Reset()
	} else {
		// Non-production (test/doc) file changed
		cb.ConsecutiveTestOnlyTurns++
	}
}

// RecordTestResult updates the test pass/fail history.
func (cb *TaskCircuitBreaker) RecordTestResult(passed bool) {
	if cb == nil {
		return
	}
	if passed {
		cb.ConsecutiveTestPasses++
	} else {
		// Real regression: reset pass count and untrip
		cb.ConsecutiveTestPasses = 0
		cb.IsTripped = false
	}
}

// Reset clears the breaker state back to fully closed.
func (cb *TaskCircuitBreaker) Reset() {
	if cb == nil {
		return
	}
	cb.ConsecutiveTestPasses = 0
	cb.ConsecutiveTestOnlyTurns = 0
	cb.ConsecutiveDuplicateInspections = 0
	cb.IsTripped = false
}

// RecordDuplicateInspection increments the count of repeated inspection calls without mutations.
func (cb *TaskCircuitBreaker) RecordDuplicateInspection() {
	if cb == nil {
		return
	}
	cb.ConsecutiveDuplicateInspections++
}

// ShouldBreakReadLoop evaluates whether an agent is stuck in repetitive read/find inspection loops.
// Returns (warn, forceTurn, reason).
func (cb *TaskCircuitBreaker) ShouldBreakReadLoop() (bool, bool, string) {
	if cb == nil {
		return false, false, ""
	}
	if cb.ConsecutiveDuplicateInspections >= 3 {
		return true, true, "CIRCUIT_BREAKER_READ_LOOP: Consecutive duplicate read/search inspections exceeded threshold without mutations. Concluding turn and running verification."
	}
	if cb.ConsecutiveDuplicateInspections == 2 {
		return true, false, "[CIRCUIT BREAKER: READ LOOP WARNING] You have called inspection tools repeatedly without making file modifications. Repeated read_file or find_files is blocked. You MUST now call 'run_tests' to verify tests, call 'write_file'/'edit_file' to implement changes, or call 'noop' if verification is complete."
	}
	return false, false, ""
}

// ShouldTrip evaluates whether the oscillation loop has been detected.
// It trips when:
// 1. Tests have passed at least 2 consecutive times (>= 2).
// 2. Edits in recent turns have touched only tests/specs (>= 2).
// 3. Task progress has reached at least 70%.
func (cb *TaskCircuitBreaker) ShouldTrip(taskProgress int) (bool, string) {
	if cb == nil {
		return false, ""
	}

	if cb.IsTripped {
		return true, "CIRCUIT_BREAKER_TRIPPED: Acceptance criteria already satisfied and verified."
	}

	if cb.ConsecutiveTestPasses >= 2 && cb.ConsecutiveTestOnlyTurns >= 2 && taskProgress >= 70 {
		cb.IsTripped = true
		return true, "CIRCUIT_BREAKER_TRIPPED: Core logic is verified and has passed all test suites 2 consecutive times with unchanged production code. Further cosmetic test modifications are halted. Conclude task."
	}

	return false, ""
}
