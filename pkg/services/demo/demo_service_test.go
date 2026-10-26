package demo

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDemoService_RunOffline(t *testing.T) {
	svc := NewDemoService()
	tempDir := filepath.Join(t.TempDir(), "demo")
	cfg := DemoConfig{
		Archetype:    ArchetypeT4,
		TargetDir:    tempDir,
		ForceOffline: true,
		SpeedFactor:  20.0, // fast execution for unit test
		Cleanup:      false,
	}

	err := svc.Run(context.Background(), cfg)
	require.NoError(t, err)

	assert.FileExists(t, filepath.Join(tempDir, "SPEC.md"))
	assert.FileExists(t, filepath.Join(tempDir, "Makefile"))
	assert.FileExists(t, filepath.Join(tempDir, "src", "http.c"))
	assert.FileExists(t, filepath.Join(tempDir, "tests", "unit", "test_http.c"))
	assert.FileExists(t, filepath.Join(tempDir, ".noctifab", "config.yaml"))
	assert.DirExists(t, filepath.Join(tempDir, ".git"))
}

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
		Archetype:    ArchetypeT4,
		TargetDir:    tempDir,
		ForceOffline: false,
		SpeedFactor:  20.0,
		Cleanup:      false,
	}

	err := svc.Run(context.Background(), cfg)
	require.NoError(t, err)
	assert.True(t, called)
	assert.Equal(t, tempDir, calledDir)
	assert.FileExists(t, filepath.Join(tempDir, ".noctifab", "config.yaml"))
	assert.DirExists(t, filepath.Join(tempDir, ".git"))
}

func TestDemoService_UnpackArchetype(t *testing.T) {
	svc := NewDemoService()
	tempDir := t.TempDir()

	err := svc.unpackArchetype(ArchetypeT4, tempDir)
	require.NoError(t, err)

	assert.FileExists(t, filepath.Join(tempDir, "SPEC.md"))
	assert.FileExists(t, filepath.Join(tempDir, "Makefile"))
	assert.FileExists(t, filepath.Join(tempDir, "src", "http.c"))
	assert.FileExists(t, filepath.Join(tempDir, ".noctifab", "config.yaml"))

	content, err := os.ReadFile(filepath.Join(tempDir, "SPEC.md"))
	require.NoError(t, err)
	assert.Contains(t, string(content), "t4 Specification")
}

func TestMockDemoLLMClient(t *testing.T) {
	mock := NewMockDemoLLMClient(50.0)

	t.Run("Planner prompt returns tasks for t4", func(t *testing.T) {
		resp, err := mock.Complete(context.Background(), "Role: PLANNER decompose t4")
		require.NoError(t, err)
		assert.NotEmpty(t, resp.Actions)
		assert.Equal(t, "add_task", resp.Actions[0].Tool)
	})

	t.Run("Planner prompt returns tasks for cli", func(t *testing.T) {
		resp, err := mock.Complete(context.Background(), "Role: PLANNER decompose calculator")
		require.NoError(t, err)
		assert.NotEmpty(t, resp.Actions)
		assert.Equal(t, "add_task", resp.Actions[0].Tool)
	})

	t.Run("Tester prompt returns test command", func(t *testing.T) {
		resp, err := mock.Complete(context.Background(), "Role: TESTER run tests")
		require.NoError(t, err)
		assert.NotEmpty(t, resp.Actions)
		assert.Equal(t, "run_tests", resp.Actions[0].Tool)
	})

	t.Run("Generator prompt returns write_file", func(t *testing.T) {
		resp, err := mock.Complete(context.Background(), "Role: GENERATOR implement")
		require.NoError(t, err)
		assert.NotEmpty(t, resp.Actions)
		assert.Equal(t, "write_file", resp.Actions[0].Tool)
	})
}
