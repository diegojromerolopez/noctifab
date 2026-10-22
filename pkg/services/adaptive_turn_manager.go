package services

import (
	"fmt"
)

// AdaptiveTurnManager governs minimum turn guarantees and active convergence extensions.
type AdaptiveTurnManager struct {
	MinTurns      int
	MaxExtensions int
}

// NewAdaptiveTurnManager creates a manager with a minimum of 2 turns and up to MaxExtensions adaptive extensions (default 3).
func NewAdaptiveTurnManager(maxExtensions ...int) *AdaptiveTurnManager {
	limit := 3
	if len(maxExtensions) > 0 && maxExtensions[0] > 0 {
		limit = maxExtensions[0]
	}
	return &AdaptiveTurnManager{
		MinTurns:      2,
		MaxExtensions: limit,
	}
}

// EnforceMinTurns guarantees that the turn budget is at least MinTurns (2).
func (m *AdaptiveTurnManager) EnforceMinTurns(configuredTurns int) int {
	if configuredTurns < m.MinTurns {
		return m.MinTurns
	}
	return configuredTurns
}

// ShouldGrantExtension evaluates whether an active repair is converging.
// If the number of test failures strictly decreased and is still > 0, an extra turn is granted.
func (m *AdaptiveTurnManager) ShouldGrantExtension(prevFailures, currFailures, extensionsUsed int) (bool, string) {
	if extensionsUsed >= m.MaxExtensions {
		return false, ""
	}
	if prevFailures > 0 && currFailures >= 0 && currFailures < prevFailures {
		msg := fmt.Sprintf("⚡ [Adaptive Turn Extension] Test failures decreasing (%d -> %d). Granting +1 turn to complete remediation (extension %d/%d).",
			prevFailures, currFailures, extensionsUsed+1, m.MaxExtensions)
		return true, msg
	}
	return false, ""
}

// ShouldGrantProgressExtension evaluates whether progress was made across runs (failures decreased or passed tests increased).
func (m *AdaptiveTurnManager) ShouldGrantProgressExtension(prevFailures, currFailures, prevPassed, currPassed, extensionsUsed int) (bool, string) {
	if extensionsUsed >= m.MaxExtensions {
		return false, ""
	}
	// Case 1: Failures decreased
	if prevFailures > 0 && currFailures >= 0 && currFailures < prevFailures {
		msg := fmt.Sprintf("⚡ [Adaptive Turn Extension] Test failures decreasing (%d -> %d). Granting +1 turn to complete remediation (extension %d/%d).",
			prevFailures, currFailures, extensionsUsed+1, m.MaxExtensions)
		return true, msg
	}
	// Case 2: Passing tests increased
	if currPassed > prevPassed && currFailures > 0 {
		msg := fmt.Sprintf("⚡ [Adaptive Turn Extension] Passing tests increasing (%d -> %d). Granting +1 turn to complete remediation (extension %d/%d).",
			prevPassed, currPassed, extensionsUsed+1, m.MaxExtensions)
		return true, msg
	}
	return false, ""
}
