package services

import (
	"fmt"
)

// AdaptiveTurnManager governs minimum turn guarantees and active convergence extensions.
type AdaptiveTurnManager struct {
	MinTurns      int
	MaxExtensions int
}

// NewAdaptiveTurnManager creates a manager with a minimum of 2 turns and up to 2 adaptive extensions.
func NewAdaptiveTurnManager() *AdaptiveTurnManager {
	return &AdaptiveTurnManager{
		MinTurns:      2,
		MaxExtensions: 2,
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
	if prevFailures > 0 && currFailures > 0 && currFailures < prevFailures {
		msg := fmt.Sprintf("⚡ [Adaptive Turn Extension] Test failures decreasing (%d -> %d). Granting +1 turn to complete remediation (extension %d/%d).",
			prevFailures, currFailures, extensionsUsed+1, m.MaxExtensions)
		return true, msg
	}
	return false, ""
}
