package services

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestZeroMutationGuard(t *testing.T) {
	guard := NewZeroMutationGuard()

	t.Run("zero git mutation fails", func(t *testing.T) {
		err := guard.ValidateGitMutation("", "   \n")
		assert.ErrorContains(t, err, "zero mutation rejection")
	})

	t.Run("working tree mutation passes", func(t *testing.T) {
		diff := "diff --git a/main.go b/main.go\n+func New() {}"
		err := guard.ValidateGitMutation(diff, "")
		require.NoError(t, err)
	})

	t.Run("committed diff mutation passes", func(t *testing.T) {
		diff := "commit 123456\n+added feature"
		err := guard.ValidateGitMutation("", diff)
		require.NoError(t, err)
	})

	t.Run("zero tests executed fails test metrics", func(t *testing.T) {
		err := guard.ValidateTestMetrics(0, 0)
		assert.ErrorContains(t, err, "anti-spoof test violation: 0 tests were discovered")
	})

	t.Run("failing tests fail test metrics", func(t *testing.T) {
		err := guard.ValidateTestMetrics(10, 2)
		assert.ErrorContains(t, err, "quality gate failure: 2 test(s) failed")
	})

	t.Run("passing non-zero tests passes test metrics", func(t *testing.T) {
		err := guard.ValidateTestMetrics(15, 0)
		require.NoError(t, err)
	})

	t.Run("contract failures fail acceptance", func(t *testing.T) {
		failures := []string{"Contract CLI-01: unexpected exit code 1"}
		err := guard.ValidateAcceptanceContracts(2, failures)
		assert.ErrorContains(t, err, "public contract DoD failure: 1 of 2 contract(s) failed")
	})

	t.Run("empty contract failures pass acceptance", func(t *testing.T) {
		err := guard.ValidateAcceptanceContracts(3, nil)
		require.NoError(t, err)
	})
}
