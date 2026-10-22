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

func TestAdaptiveTurnManager_ConfigurableExtensions(t *testing.T) {
	// Default is 3
	mgrDefault := services.NewAdaptiveTurnManager()
	assert.Equal(t, 3, mgrDefault.MaxExtensions)

	// Custom is 5
	mgrCustom := services.NewAdaptiveTurnManager(5)
	assert.Equal(t, 5, mgrCustom.MaxExtensions)
}

func TestAdaptiveTurnManager_ShouldGrantExtension(t *testing.T) {
	mgr := services.NewAdaptiveTurnManager(3)

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

	// Max extensions exceeded (3/3): extension denied
	grant, _ = mgr.ShouldGrantExtension(4, 1, 3)
	assert.False(t, grant)
}

func TestAdaptiveTurnManager_ShouldGrantProgressExtension(t *testing.T) {
	mgr := services.NewAdaptiveTurnManager(3)

	// Case 1: Failures decreased (5 -> 3, passed 2 -> 4)
	grant, msg := mgr.ShouldGrantProgressExtension(5, 3, 2, 4, 0)
	assert.True(t, grant)
	assert.Contains(t, msg, "Test failures decreasing (5 -> 3)")

	// Case 2: Passed tests increased while failures remained constant (passed 1 -> 3, failures 2 -> 2)
	grant, msg = mgr.ShouldGrantProgressExtension(2, 2, 1, 3, 0)
	assert.True(t, grant)
	assert.Contains(t, msg, "Passing tests increasing (1 -> 3)")

	// Case 3: No progress (failures 3 -> 3, passed 2 -> 2): denied
	grant, _ = mgr.ShouldGrantProgressExtension(3, 3, 2, 2, 0)
	assert.False(t, grant)

	// Case 4: Regression (failures 2 -> 4, passed 3 -> 1): denied
	grant, _ = mgr.ShouldGrantProgressExtension(2, 4, 3, 1, 0)
	assert.False(t, grant)

	// Case 5: Limit reached (extensionsUsed = 3): denied
	grant, _ = mgr.ShouldGrantProgressExtension(5, 2, 1, 4, 3)
	assert.False(t, grant)
}
