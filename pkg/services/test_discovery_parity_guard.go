package services

import (
	"fmt"
)

// TestDiscoveryParityViolation reports a mismatch where tests are authored in the workspace
// but none (or a fraction) were discovered/executed by the test runner.
type TestDiscoveryParityViolation struct {
	DeclaredCount int
	RunnerCount   int
	Details       string
}

func (v TestDiscoveryParityViolation) Error() string {
	return fmt.Sprintf("test discovery parity violation: workspace defines %d test case(s), but runner executed/discovered %d tests (%s); test file naming, runner configuration, or discovery flags are mismatched",
		v.DeclaredCount, v.RunnerCount, v.Details)
}

// TestDiscoveryParityGuard compares static test declarations with runtime runner metrics
// to prevent silent green runs caused by broken discovery patterns (e.g. running 0 tests and exiting 0).
type TestDiscoveryParityGuard struct{}

// NewTestDiscoveryParityGuard creates a new TestDiscoveryParityGuard.
func NewTestDiscoveryParityGuard() *TestDiscoveryParityGuard {
	return &TestDiscoveryParityGuard{}
}

// ValidateDiscoveryParity checks that if tests are declared in the codebase,
// the test runner actually discovered and executed them.
func (g *TestDiscoveryParityGuard) ValidateDiscoveryParity(declaredCount, runnerExecutedCount int) error {
	if declaredCount > 0 && runnerExecutedCount == 0 {
		return &TestDiscoveryParityViolation{
			DeclaredCount: declaredCount,
			RunnerCount:   runnerExecutedCount,
			Details:       "test runner reported 0 tests discovered/run while static test definitions exist",
		}
	}
	return nil
}
