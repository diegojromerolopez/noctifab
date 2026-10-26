package cli

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDemoCmd_ExecutionWithRunner(t *testing.T) {
	called := false
	var calledDir string
	demoStartRunnerOverride = func(_ context.Context, dir string) error {
		called = true
		calledDir = dir
		return nil
	}
	defer func() {
		demoStartRunnerOverride = nil
	}()

	tempDir := t.TempDir()
	err := demoCmd.RunE(demoCmd, []string{tempDir})
	require.NoError(t, err)
	assert.True(t, called)
	assert.Equal(t, tempDir, calledDir)
}

func TestDemoCmd_Flags(t *testing.T) {
	assert.True(t, strings.HasPrefix(demoCmd.Use, "demo"))
	assert.NotNil(t, demoCmd.Flags().Lookup("project"))
	assert.NotNil(t, demoCmd.Flags().Lookup("dir"))
	assert.NotNil(t, demoCmd.Flags().Lookup("cleanup"))
	assert.Nil(t, demoCmd.Flags().Lookup("offline"))
	assert.Nil(t, demoCmd.Flags().Lookup("speed"))
}
