package cli

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDemoCmd_Execution(t *testing.T) {
	demoSpeedFactor = 100.0 // instant for unit test
	demoOffline = true
	defer func() {
		demoSpeedFactor = 1.0
	}()

	err := demoCmd.RunE(demoCmd, []string{})
	require.NoError(t, err)
}

func TestDemoCmd_Flags(t *testing.T) {
	assert.Equal(t, "demo", demoCmd.Use)
	assert.NotNil(t, demoCmd.Flags().Lookup("project"))
	assert.NotNil(t, demoCmd.Flags().Lookup("offline"))
	assert.NotNil(t, demoCmd.Flags().Lookup("speed"))
	assert.NotNil(t, demoCmd.Flags().Lookup("no-cleanup"))
}
