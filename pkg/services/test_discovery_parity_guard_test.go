package services

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTestDiscoveryParityGuard(t *testing.T) {
	guard := NewTestDiscoveryParityGuard()

	t.Run("when declared tests are positive and runner executes positive count, validation succeeds", func(t *testing.T) {
		assert.NoError(t, guard.ValidateDiscoveryParity(5, 5))
		assert.NoError(t, guard.ValidateDiscoveryParity(5, 3))
	})

	t.Run("when zero declared tests exist and runner executes zero, validation succeeds", func(t *testing.T) {
		assert.NoError(t, guard.ValidateDiscoveryParity(0, 0))
	})

	t.Run("when declared tests exist but runner executes 0 tests, violation is flagged", func(t *testing.T) {
		err := guard.ValidateDiscoveryParity(4, 0)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "test discovery parity violation")
		assert.Contains(t, err.Error(), "workspace defines 4 test case(s), but runner executed/discovered 0 tests")
	})
}
