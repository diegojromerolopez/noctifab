package services_test

import (
	"testing"

	"github.com/diegojromerolopez/noctifab/pkg/services"
	"github.com/stretchr/testify/assert"
)

func TestAdaptiveTurnManager_EnforceMinTurns(t *testing.T) {
	mgr := services.NewAdaptiveTurnManager()
	assert.Equal(t, 2, mgr.EnforceMinTurns(0))
	assert.Equal(t, 2, mgr.EnforceMinTurns(1))
	assert.Equal(t, 2, mgr.EnforceMinTurns(2))
	assert.Equal(t, 5, mgr.EnforceMinTurns(5))
}

func TestAdaptiveTurnManager_ShouldGrantExtension(t *testing.T) {
	mgr := services.NewAdaptiveTurnManager()

	// Failures decreased from 4 to 2: extension granted
	grant, msg := mgr.ShouldGrantExtension(4, 2, 0)
	assert.True(t, grant)
	assert.Contains(t, msg, "Test failures decreasing (4 -> 2)")

	// Failures increased from 2 to 3: extension denied
	grant, _ = mgr.ShouldGrantExtension(2, 3, 0)
	assert.False(t, grant)

	// Failures stagnated (2 -> 2): extension denied
	grant, _ = mgr.ShouldGrantExtension(2, 2, 0)
	assert.False(t, grant)

	// Failures reached 0 (passing): no extension needed
	grant, _ = mgr.ShouldGrantExtension(2, 0, 0)
	assert.False(t, grant)

	// Max extensions exceeded: extension denied
	grant, _ = mgr.ShouldGrantExtension(4, 1, 2)
	assert.False(t, grant)
}
