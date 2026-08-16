package demo

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDemoService_Run(t *testing.T) {
	svc := NewDemoService()
	cfg := DemoConfig{
		Archetype:    ArchetypeCLI,
		ForceOffline: true,
		SpeedFactor:  10.0, // fast execution for unit test
		NoCleanup:    false,
	}

	err := svc.Run(context.Background(), cfg)
	require.NoError(t, err)
}

func TestDemoService_UnpackArchetype(t *testing.T) {
	svc := NewDemoService()
	tempDir := t.TempDir()

	err := svc.unpackArchetype(ArchetypeCLI, tempDir)
	require.NoError(t, err)

	assert.FileExists(t, filepath.Join(tempDir, "SPEC.md"))
	assert.FileExists(t, filepath.Join(tempDir, "main.go"))
	assert.FileExists(t, filepath.Join(tempDir, "calc_test.go"))

	content, err := os.ReadFile(filepath.Join(tempDir, "SPEC.md"))
	require.NoError(t, err)
	assert.Contains(t, string(content), "Calculator")
}

func TestMockDemoLLMClient(t *testing.T) {
	mock := NewMockDemoLLMClient(50.0)

	t.Run("Planner prompt returns tasks", func(t *testing.T) {
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
