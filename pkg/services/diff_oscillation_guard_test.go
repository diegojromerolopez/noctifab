package services

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDiffOscillationGuard(t *testing.T) {
	t.Run("when different diffs are recorded, it detects no oscillation", func(t *testing.T) {
		guard := NewDiffOscillationGuard(10)

		isOsc, violation := guard.RecordAndCheck(1, "diff --git a/foo.go b/foo.go\n+func A() {}")
		assert.False(t, isOsc)
		assert.Nil(t, violation)

		isOsc, violation = guard.RecordAndCheck(2, "diff --git a/foo.go b/foo.go\n+func B() {}")
		assert.False(t, isOsc)
		assert.Nil(t, violation)

		isOsc, violation = guard.RecordAndCheck(3, "diff --git a/foo.go b/foo.go\n+func C() {}")
		assert.False(t, isOsc)
		assert.Nil(t, violation)
	})

	t.Run("when a previous diff is repeated in a later turn, it flags an oscillation violation", func(t *testing.T) {
		guard := NewDiffOscillationGuard(10)

		diffStateA := "diff --git a/main.py b/main.py\n-x = 1\n+x = 2"
		diffStateB := "diff --git a/main.py b/main.py\n-x = 2\n+x = 3"

		// Turn 1: State A
		isOsc, violation := guard.RecordAndCheck(1, diffStateA)
		assert.False(t, isOsc)
		assert.Nil(t, violation)

		// Turn 2: State B
		isOsc, violation = guard.RecordAndCheck(2, diffStateB)
		assert.False(t, isOsc)
		assert.Nil(t, violation)

		// Turn 3: Reverts back to State A (ping-pong)
		isOsc, violation = guard.RecordAndCheck(3, diffStateA)
		assert.True(t, isOsc)
		require.NotNil(t, violation)
		assert.Equal(t, 3, violation.CurrentTurn)
		assert.Equal(t, 1, violation.PreviousTurn)
		assert.Contains(t, violation.Error(), "diff oscillation detected")
		assert.Contains(t, violation.Error(), "turn 3 is identical to turn 1")
	})

	t.Run("when bounded window expires, old states can be pruned", func(t *testing.T) {
		guard := NewDiffOscillationGuard(2)

		diffA := "diff A"
		diffB := "diff B"
		diffC := "diff C"

		_, _ = guard.RecordAndCheck(1, diffA)
		_, _ = guard.RecordAndCheck(2, diffB)
		_, _ = guard.RecordAndCheck(3, diffC)

		// Turn 4 with diffA again; since window is 2, diffA should have been pruned
		isOsc, violation := guard.RecordAndCheck(4, diffA)
		assert.False(t, isOsc)
		assert.Nil(t, violation)
	})

	t.Run("reset clears history", func(t *testing.T) {
		guard := NewDiffOscillationGuard(10)

		_, _ = guard.RecordAndCheck(1, "diff A")
		guard.Reset()

		isOsc, violation := guard.RecordAndCheck(2, "diff A")
		assert.False(t, isOsc)
		assert.Nil(t, violation)
	})
}
