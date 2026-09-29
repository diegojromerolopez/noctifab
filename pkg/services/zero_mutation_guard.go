package services

import (
	"fmt"
	"strings"
)

// ZeroMutationGuard validates final task and story deliverables, rejecting false-positive
// completions where zero files were changed, zero tests were discovered, or contracts failed.
type ZeroMutationGuard struct{}

// NewZeroMutationGuard creates a ZeroMutationGuard instance.
func NewZeroMutationGuard() *ZeroMutationGuard {
	return &ZeroMutationGuard{}
}

// ValidateGitMutation verifies that verifiable mutations exist in either uncommitted
// working tree changes or committed commits relative to the base branch.
func (g *ZeroMutationGuard) ValidateGitMutation(workingTreeDiff, committedDiff string) error {
	hasWorkingTreeChanges := strings.TrimSpace(workingTreeDiff) != ""
	hasCommittedChanges := strings.TrimSpace(committedDiff) != ""

	if !hasWorkingTreeChanges && !hasCommittedChanges {
		return fmt.Errorf("zero mutation rejection: task claimed completion but produced 0 file modifications or diffs relative to base branch")
	}

	return nil
}

// ValidateTestMetrics verifies that test runners discovered and executed non-zero tests,
// and that zero failures occurred.
func (g *ZeroMutationGuard) ValidateTestMetrics(totalTests, failedTests int) error {
	if totalTests <= 0 {
		return fmt.Errorf("anti-spoof test violation: 0 tests were discovered/executed (cannot verify correctness with an empty test suite)")
	}
	if failedTests > 0 {
		return fmt.Errorf("quality gate failure: %d test(s) failed", failedTests)
	}
	return nil
}

// ValidateAcceptanceContracts ensures all executed public contracts passed without failures.
func (g *ZeroMutationGuard) ValidateAcceptanceContracts(executedContracts int, failures []string) error {
	if len(failures) > 0 {
		return fmt.Errorf("public contract DoD failure: %d of %d contract(s) failed:\n - %s", len(failures), executedContracts, strings.Join(failures, "\n - "))
	}
	return nil
}
