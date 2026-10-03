package services

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestOscillationCircuitBreaker_Fingerprint(t *testing.T) {
	cb := NewOscillationCircuitBreaker()

	t.Run("empty log returns empty_pass", func(t *testing.T) {
		hash, summary := cb.Fingerprint("")
		assert.Equal(t, "empty_pass", hash)
		assert.Equal(t, "No errors reported", summary)
	})

	t.Run("normalizes memory addresses, line numbers, and durations", func(t *testing.T) {
		log1 := `
FAIL: test_store (tests.test_store.StoreTests)
AttributeError: 'Store' object has no attribute 'data' at 0x7f8b1c line 42 (0.12s)
`
		log2 := `
FAIL: test_store (tests.test_store.StoreTests)
AttributeError: 'Store' object has no attribute 'data' at 0x4a9d2e line 89 (0.45s)
`
		hash1, summary1 := cb.Fingerprint(log1)
		hash2, summary2 := cb.Fingerprint(log2)

		assert.Equal(t, hash1, hash2, "fingerprints must match despite memory addresses, line numbers, and runtimes")
		assert.Equal(t, summary1, summary2)
		assert.Contains(t, summary1, "py_fail:test_store")
	})
}

func TestOscillationCircuitBreaker_CycleDetection(t *testing.T) {
	t.Run("linear failures without cycle return continue", func(t *testing.T) {
		cb := NewOscillationCircuitBreaker()

		d1 := cb.RecordFailure(1, "FAIL: test_one (tests.test_app.AppTests)\nAssertionError: expected 1 got 2")
		assert.Equal(t, OscillationActionContinue, d1.Action)

		d2 := cb.RecordFailure(2, "FAIL: test_two (tests.test_app.AppTests)\nAssertionError: expected 3 got 4")
		assert.Equal(t, OscillationActionContinue, d2.Action)

		d3 := cb.RecordFailure(3, "FAIL: test_three (tests.test_app.AppTests)\nAssertionError: expected 5 got 6")
		assert.Equal(t, OscillationActionContinue, d3.Action)
	})

	t.Run("ping-pong period-2 cycle triggers reconciliation then trips", func(t *testing.T) {
		cb := NewOscillationCircuitBreaker()

		failureA := "FAIL: test_collections (test_store.StoreTests)\nAttributeError: 'Store' object has no attribute 'data'"
		failureB := "FAIL: test_types (unit.test_store.StoreTests)\nAttributeError: 'Store' object has no attribute 'type'"

		// Turn 1: State A
		d1 := cb.RecordFailure(1, failureA)
		assert.Equal(t, OscillationActionContinue, d1.Action)

		// Turn 2: State B
		d2 := cb.RecordFailure(2, failureB)
		assert.Equal(t, OscillationActionContinue, d2.Action)

		// Turn 3: State A again -> Period 2 oscillation detected!
		d3 := cb.RecordFailure(3, failureA)
		assert.Equal(t, OscillationActionReconcile, d3.Action)
		assert.Equal(t, 2, d3.Period)
		assert.Contains(t, d3.Reason, "period-2 failure oscillation detected")
		assert.Contains(t, d3.Directive, "OSCILLATION CIRCUIT BREAKER: CONFLICT RECONCILIATION DIRECTIVE")
		assert.Contains(t, d3.Directive, "delete_file")

		// Turn 4: State B again -> Persistent oscillation after reconciliation -> TRIP circuit!
		d4 := cb.RecordFailure(4, failureB)
		assert.Equal(t, OscillationActionTrip, d4.Action)
		assert.Equal(t, 2, d4.Period)
		assert.Contains(t, d4.Reason, "persistent period-2 failure oscillation detected")
	})

	t.Run("triangular period-3 cycle triggers reconciliation", func(t *testing.T) {
		cb := NewOscillationCircuitBreaker()

		stateA := "FAIL: test_a (tests.test_a.ATests)\nAssertionError: A failed"
		stateB := "FAIL: test_b (tests.test_b.BTests)\nAssertionError: B failed"
		stateC := "FAIL: test_c (tests.test_c.CTests)\nAssertionError: C failed"

		// Turn 1: A
		d1 := cb.RecordFailure(1, stateA)
		assert.Equal(t, OscillationActionContinue, d1.Action)

		// Turn 2: B
		d2 := cb.RecordFailure(2, stateB)
		assert.Equal(t, OscillationActionContinue, d2.Action)

		// Turn 3: C
		d3 := cb.RecordFailure(3, stateC)
		assert.Equal(t, OscillationActionContinue, d3.Action)

		// Turn 4: A -> Triangular period 3 cycle!
		d4 := cb.RecordFailure(4, stateA)
		assert.Equal(t, OscillationActionReconcile, d4.Action)
		assert.Equal(t, 3, d4.Period)
		assert.Contains(t, d4.Reason, "period-3 failure oscillation detected")
	})

	t.Run("stagnant identical failure states do not trigger oscillation", func(t *testing.T) {
		cb := NewOscillationCircuitBreaker()
		sameFailure := "FAIL: test_stuck (tests.test_app.AppTests)\nAssertionError: not implemented"

		d1 := cb.RecordFailure(1, sameFailure)
		assert.Equal(t, OscillationActionContinue, d1.Action)

		d2 := cb.RecordFailure(2, sameFailure)
		assert.Equal(t, OscillationActionContinue, d2.Action)

		d3 := cb.RecordFailure(3, sameFailure)
		assert.Equal(t, OscillationActionContinue, d3.Action, "identical stagnant failures should not trigger alternation oscillation")
	})
}
