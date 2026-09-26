package services

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sync"
)

// DiffOscillationViolation records a detected state oscillation loop.
type DiffOscillationViolation struct {
	CurrentTurn  int
	PreviousTurn int
	StateDigest  string
}

func (v DiffOscillationViolation) Error() string {
	return fmt.Sprintf("diff oscillation detected: state at turn %d is identical to turn %d (hash: %s); repetitive repair cycles are prohibited",
		v.CurrentTurn, v.PreviousTurn, v.StateDigest)
}

// DiffOscillationGuard detects state or patch oscillations across turns,
// preventing an LLM agent from cycling back and forth between two or more broken states.
type DiffOscillationGuard struct {
	mu          sync.RWMutex
	history     map[string]int // digest -> turn
	turnHistory []string       // index is turn (or sequence) -> digest
	maxWindow   int
}

// NewDiffOscillationGuard creates a guard that tracks up to maxWindow past state digests.
// If maxWindow <= 0, history is unbounded.
func NewDiffOscillationGuard(maxWindow int) *DiffOscillationGuard {
	return &DiffOscillationGuard{
		history:     make(map[string]int),
		turnHistory: make([]string, 0),
		maxWindow:   maxWindow,
	}
}

// ComputeDigest computes a deterministic SHA-256 hex digest for a diff or workspace snapshot.
func (g *DiffOscillationGuard) ComputeDigest(diffContent string) string {
	h := sha256.Sum256([]byte(diffContent))
	return hex.EncodeToString(h[:])
}

// RecordAndCheck records the state for the given turn and checks if it matches any
// previously recorded state in the tracked history window.
func (g *DiffOscillationGuard) RecordAndCheck(turn int, diffContent string) (bool, *DiffOscillationViolation) {
	g.mu.Lock()
	defer g.mu.Unlock()

	digest := g.ComputeDigest(diffContent)

	if prevTurn, exists := g.history[digest]; exists && prevTurn != turn {
		violation := &DiffOscillationViolation{
			CurrentTurn:  turn,
			PreviousTurn: prevTurn,
			StateDigest:  digest,
		}
		return true, violation
	}

	g.history[digest] = turn
	g.turnHistory = append(g.turnHistory, digest)

	// Prune history window if bounded
	if g.maxWindow > 0 && len(g.turnHistory) > g.maxWindow {
		oldestDigest := g.turnHistory[0]
		g.turnHistory = g.turnHistory[1:]
		// Only remove from map if it still points to the pruned instance
		if g.history[oldestDigest] <= turn-g.maxWindow {
			delete(g.history, oldestDigest)
		}
	}

	return false, nil
}

// Reset clears the recorded history.
func (g *DiffOscillationGuard) Reset() {
	g.mu.Lock()
	defer g.mu.Unlock()

	g.history = make(map[string]int)
	g.turnHistory = make([]string, 0)
}
