package services

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRegressionBarrierGuard(t *testing.T) {
	t.Run("when baseline is empty, check returns no regression", func(t *testing.T) {
		guard := NewRegressionBarrierGuard()

		hasReg, violation := guard.CheckRegression(1, []string{"test_a", "test_b"})
		assert.False(t, hasReg)
		assert.Nil(t, violation)
	})

	t.Run("when tests continue passing and new tests are added, no regression occurs and watermark advances", func(t *testing.T) {
		guard := NewRegressionBarrierGuard()

		// Turn 1: test_a, test_b pass
		hasReg, violation := guard.UpdateAndCheck(1, []string{"test_a", "test_b"})
		assert.False(t, hasReg)
		assert.Nil(t, violation)
		assert.Equal(t, 2, guard.BaselineSize())

		// Turn 2: test_a, test_b, test_c pass
		hasReg, violation = guard.UpdateAndCheck(2, []string{"test_a", "test_b", "test_c"})
		assert.False(t, hasReg)
		assert.Nil(t, violation)
		assert.Equal(t, 3, guard.BaselineSize())
	})

	t.Run("when a previously passing test fails, violation is flagged", func(t *testing.T) {
		guard := NewRegressionBarrierGuard()
		guard.SetBaseline(1, []string{"test_login", "test_signup", "test_logout"})

		// Turn 2: test_logout fails / missing
		hasReg, violation := guard.CheckRegression(2, []string{"test_login", "test_signup"})
		assert.True(t, hasReg)
		require.NotNil(t, violation)
		assert.Equal(t, 2, violation.CurrentTurn)
		assert.Equal(t, 1, violation.PreviousTurn)
		assert.Equal(t, []string{"test_logout"}, violation.RegressedTests)
		assert.Contains(t, violation.Error(), "regression barrier violation")
		assert.Contains(t, violation.Error(), "test_logout")
	})

	t.Run("when UpdateAndCheck encounters a regression, watermark does not advance", func(t *testing.T) {
		guard := NewRegressionBarrierGuard()
		guard.SetBaseline(1, []string{"test_1", "test_2"})

		hasReg, violation := guard.UpdateAndCheck(2, []string{"test_1"})
		assert.True(t, hasReg)
		require.NotNil(t, violation)

		// Baseline remains at 2 tests
		assert.Equal(t, 2, guard.BaselineSize())
	})

	t.Run("reset clears the watermark", func(t *testing.T) {
		guard := NewRegressionBarrierGuard()
		guard.SetBaseline(1, []string{"test_1", "test_2"})
		assert.Equal(t, 2, guard.BaselineSize())

		guard.Reset()
		assert.Equal(t, 0, guard.BaselineSize())
	})
}
