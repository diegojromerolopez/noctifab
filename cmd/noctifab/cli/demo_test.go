package cli

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDemoCmd_ExecutionWithPositionalArg(t *testing.T) {
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

func TestDemoCmd_ExecutionDefaultCurrentDir(t *testing.T) {
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
	origWd, err := os.Getwd()
	require.NoError(t, err)
	defer func() { _ = os.Chdir(origWd) }()
	require.NoError(t, os.Chdir(tempDir))

	realTempDir, err := filepath.EvalSymlinks(tempDir)
	require.NoError(t, err)

	err = demoCmd.RunE(demoCmd, []string{})
	require.NoError(t, err)
	assert.True(t, called)
	realCalledDir, err := filepath.EvalSymlinks(calledDir)
	require.NoError(t, err)
	assert.Equal(t, realTempDir, realCalledDir)
}

func TestDemoCmd_Flags(t *testing.T) {
	assert.True(t, strings.HasPrefix(demoCmd.Use, "demo"))
	assert.NotNil(t, demoCmd.Flags().Lookup("project"))
	assert.Nil(t, demoCmd.Flags().Lookup("dir"))
	assert.Nil(t, demoCmd.Flags().Lookup("cleanup"))
	assert.Nil(t, demoCmd.Flags().Lookup("offline"))
	assert.Nil(t, demoCmd.Flags().Lookup("speed"))
}
