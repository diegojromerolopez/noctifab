package cli

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDemoCmd_Execution(t *testing.T) {
	demoSpeedFactor = 100.0 // instant for unit test
	demoOffline = true
	tempDir := t.TempDir()
	defer func() {
		demoSpeedFactor = 1.0
	}()

	err := demoCmd.RunE(demoCmd, []string{tempDir})
	require.NoError(t, err)
}

func TestDemoCmd_Flags(t *testing.T) {
	assert.True(t, strings.HasPrefix(demoCmd.Use, "demo"))
	assert.NotNil(t, demoCmd.Flags().Lookup("project"))
	assert.NotNil(t, demoCmd.Flags().Lookup("dir"))
	assert.NotNil(t, demoCmd.Flags().Lookup("offline"))
	assert.NotNil(t, demoCmd.Flags().Lookup("speed"))
	assert.NotNil(t, demoCmd.Flags().Lookup("cleanup"))
}
