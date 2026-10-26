package demo

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDemoService_RunWithRunner(t *testing.T) {
	called := false
	var calledDir string
	runner := func(_ context.Context, dir string) error {
		called = true
		calledDir = dir
		return nil
	}
	svc := NewDemoService(runner)
	tempDir := filepath.Join(t.TempDir(), "live_demo")
	cfg := DemoConfig{
		Archetype: ArchetypeT4,
		TargetDir: tempDir,
		Cleanup:   false,
	}

	err := svc.Run(context.Background(), cfg)
	require.NoError(t, err)
	assert.True(t, called)
	assert.Equal(t, tempDir, calledDir)
	assert.FileExists(t, filepath.Join(tempDir, "SPEC.md"))
	assert.FileExists(t, filepath.Join(tempDir, ".noctifab", "config.yaml"))
	assert.DirExists(t, filepath.Join(tempDir, ".git"))
	assert.NoFileExists(t, filepath.Join(tempDir, "src", "http.c"))
	assert.NoFileExists(t, filepath.Join(tempDir, "tests", "unit", "test_http.c"))
}

func TestDemoService_RunRequiresRunner(t *testing.T) {
	svc := NewDemoService()
	tempDir := filepath.Join(t.TempDir(), "no_runner_demo")
	cfg := DemoConfig{
		Archetype: ArchetypeT4,
		TargetDir: tempDir,
		Cleanup:   false,
	}

	err := svc.Run(context.Background(), cfg)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no code generation runner configured")
}

func TestDemoService_UnpackArchetype(t *testing.T) {
	svc := NewDemoService()
	tempDir := t.TempDir()

	err := svc.unpackArchetype(ArchetypeT4, tempDir)
	require.NoError(t, err)

	assert.FileExists(t, filepath.Join(tempDir, "SPEC.md"))
	assert.FileExists(t, filepath.Join(tempDir, "Makefile"))
	assert.FileExists(t, filepath.Join(tempDir, ".noctifab", "config.yaml"))
	assert.NoFileExists(t, filepath.Join(tempDir, "src", "http.c"))
	assert.NoFileExists(t, filepath.Join(tempDir, "tests", "unit", "test_http.c"))

	content, err := os.ReadFile(filepath.Join(tempDir, "SPEC.md"))
	require.NoError(t, err)
	assert.Contains(t, string(content), "t4 Specification")
}
