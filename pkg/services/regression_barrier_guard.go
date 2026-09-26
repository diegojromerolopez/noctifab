package services

import (
	"fmt"
	"sort"
	"strings"
	"sync"
)

// RegressionBarrierViolation records a test suite regression where previously passing
// tests regressed to failed or were omitted.
type RegressionBarrierViolation struct {
	PreviousTurn   int
	CurrentTurn    int
	RegressedTests []string
	TotalBaseline  int
	TotalCurrent   int
}

func (v RegressionBarrierViolation) Error() string {
	return fmt.Sprintf("regression barrier violation: %d previously passing test(s) failed or were removed in turn %d (previously passing in turn %d): [%s]. Regressions on already-verified functionality are prohibited",
		len(v.RegressedTests), v.CurrentTurn, v.PreviousTurn, strings.Join(v.RegressedTests, ", "))
}

// RegressionBarrierGuard maintains a watermark of passing tests and prevents
// patches from breaking previously verified functionality ("fixing A by breaking B").
type RegressionBarrierGuard struct {
	mu           sync.RWMutex
	lastTurn     int
	passingTests map[string]struct{}
}

// NewRegressionBarrierGuard creates a new RegressionBarrierGuard.
func NewRegressionBarrierGuard() *RegressionBarrierGuard {
	return &RegressionBarrierGuard{
		passingTests: make(map[string]struct{}),
	}
}

// SetBaseline sets or updates the current passing test set baseline.
func (g *RegressionBarrierGuard) SetBaseline(turn int, passing []string) {
	g.mu.Lock()
	defer g.mu.Unlock()

	g.lastTurn = turn
	g.passingTests = make(map[string]struct{}, len(passing))
	for _, t := range passing {
		clean := strings.TrimSpace(t)
		if clean != "" {
			g.passingTests[clean] = struct{}{}
		}
	}
}

// CheckRegression compares the current passing tests with the recorded baseline.
// Returns a violation if any previously passing test is missing from currentPassing.
func (g *RegressionBarrierGuard) CheckRegression(turn int, currentPassing []string) (bool, *RegressionBarrierViolation) {
	g.mu.RLock()
	defer g.mu.RUnlock()

	if len(g.passingTests) == 0 {
		return false, nil
	}

	currentSet := make(map[string]struct{}, len(currentPassing))
	for _, t := range currentPassing {
		clean := strings.TrimSpace(t)
		if clean != "" {
			currentSet[clean] = struct{}{}
		}
	}

	var regressed []string
	for testName := range g.passingTests {
		if _, ok := currentSet[testName]; !ok {
			regressed = append(regressed, testName)
		}
	}

	if len(regressed) > 0 {
		sort.Strings(regressed)
		violation := &RegressionBarrierViolation{
			PreviousTurn:   g.lastTurn,
			CurrentTurn:    turn,
			RegressedTests: regressed,
			TotalBaseline:  len(g.passingTests),
			TotalCurrent:   len(currentSet),
		}
		return true, violation
	}

	return false, nil
}

// UpdateAndCheck checks for regressions against the current baseline. If no regression occurred,
// it advances the baseline to include newly passing tests (monotonic watermark union).
func (g *RegressionBarrierGuard) UpdateAndCheck(turn int, currentPassing []string) (bool, *RegressionBarrierViolation) {
	g.mu.Lock()
	defer g.mu.Unlock()

	currentSet := make(map[string]struct{}, len(currentPassing))
	for _, t := range currentPassing {
		clean := strings.TrimSpace(t)
		if clean != "" {
			currentSet[clean] = struct{}{}
		}
	}

	var regressed []string
	for testName := range g.passingTests {
		if _, ok := currentSet[testName]; !ok {
			regressed = append(regressed, testName)
		}
	}

	if len(regressed) > 0 {
		sort.Strings(regressed)
		violation := &RegressionBarrierViolation{
			PreviousTurn:   g.lastTurn,
			CurrentTurn:    turn,
			RegressedTests: regressed,
			TotalBaseline:  len(g.passingTests),
			TotalCurrent:   len(currentSet),
		}
		return true, violation
	}

	// No regression: advance watermark
	g.lastTurn = turn
	for testName := range currentSet {
		g.passingTests[testName] = struct{}{}
	}

	return false, nil
}

// BaselineSize returns the number of passing tests in the current watermark.
func (g *RegressionBarrierGuard) BaselineSize() int {
	g.mu.RLock()
	defer g.mu.RUnlock()
	return len(g.passingTests)
}

// Reset clears the baseline watermark.
func (g *RegressionBarrierGuard) Reset() {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.lastTurn = 0
	g.passingTests = make(map[string]struct{})
}
