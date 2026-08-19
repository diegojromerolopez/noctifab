package services

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/diegojromerolopez/noctifab/pkg/infrastructure/config"
	"github.com/diegojromerolopez/noctifab/pkg/infrastructure/storage"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSpecOrchestrator_SaveSpecFile(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "spec-orch-test-*")
	require.NoError(t, err)
	defer func() { _ = os.RemoveAll(tempDir) }()

	orchestrator := NewSpecOrchestrator(&config.Config{}, nil, nil, nil)
	targetPath := filepath.Join(tempDir, "sub", "SPEC.md")

	err = orchestrator.saveSpecFile(targetPath, "# Test Spec Content")
	require.NoError(t, err)

	data, err := os.ReadFile(targetPath)
	require.NoError(t, err)
	assert.Equal(t, "# Test Spec Content", string(data))
}

func TestSpecOrchestrator_RunSession_NonInteractive(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "spec-orch-test-*")
	require.NoError(t, err)
	defer func() { _ = os.RemoveAll(tempDir) }()

	// Pre-seed SPEC.md
	specPath := filepath.Join(tempDir, "SPEC.md")
	require.NoError(t, os.WriteFile(specPath, []byte("# Existing Pre-Seeded Spec"), 0644))

	var out bytes.Buffer
	renderer := NewCustomSpecRenderer(strings.NewReader(""), &out)

	orchestrator := NewSpecOrchestrator(&config.Config{}, nil, nil, renderer)
	ctx := context.Background()

	session, err := orchestrator.RunSession(ctx, RunSessionOptions{
		ProjectPath:    tempDir,
		TargetFile:     specPath,
		NonInteractive: true,
		EnableAudit:    false,
	})

	require.NoError(t, err)
	assert.True(t, session.IsComplete)
	assert.Equal(t, "# Existing Pre-Seeded Spec", session.CurrentSpec)
	assert.Equal(t, 1, len(session.Revisions))

	// Verify snapshot created
	snapMgr := storage.NewSpecSnapshotManager(tempDir)
	versions, sErr := snapMgr.ListSnapshots()
	require.NoError(t, sErr)
	assert.Equal(t, []int{1}, versions)
}

func TestSpecOrchestrator_RunSession_InteractiveImmediateApproval(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "spec-orch-test-*")
	require.NoError(t, err)
	defer func() { _ = os.RemoveAll(tempDir) }()

	specPath := filepath.Join(tempDir, "SPEC.md")
	require.NoError(t, os.WriteFile(specPath, []byte("# Existing Spec"), 0644))

	// User inputs "looks good to me\n" followed by "n\n" for roadmap prompt
	input := strings.NewReader("looks good to me\nn\n")
	var out bytes.Buffer
	renderer := NewCustomSpecRenderer(input, &out)

	falseVal := false
	cfg := &config.Config{
		Spec: config.SpecConfig{
			AutoGenerateRoadmap: &falseVal,
		},
	}

	orchestrator := NewSpecOrchestrator(cfg, nil, nil, renderer)
	ctx := context.Background()

	session, err := orchestrator.RunSession(ctx, RunSessionOptions{
		ProjectPath:    tempDir,
		TargetFile:     specPath,
		NonInteractive: false,
		EnableAudit:    false,
	})

	require.NoError(t, err)
	assert.True(t, session.IsComplete)
	assert.Equal(t, "# Existing Spec", session.CurrentSpec)
}

func TestSpecOrchestrator_RunSession_TimeTravelCommands(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "spec-orch-test-*")
	require.NoError(t, err)
	defer func() { _ = os.RemoveAll(tempDir) }()

	specPath := filepath.Join(tempDir, "SPEC.md")
	require.NoError(t, os.WriteFile(specPath, []byte("# Existing Spec"), 0644))

	// Input sequence: history -> undo -> redo -> checkout 1 -> approved
	input := strings.NewReader("history\nundo\nredo\ncheckout 1\nlooks good to me\nn\n")
	var out bytes.Buffer
	renderer := NewCustomSpecRenderer(input, &out)

	falseVal := false
	cfg := &config.Config{
		Spec: config.SpecConfig{
			AutoGenerateRoadmap: &falseVal,
		},
	}

	orchestrator := NewSpecOrchestrator(cfg, nil, nil, renderer)
	ctx := context.Background()

	session, err := orchestrator.RunSession(ctx, RunSessionOptions{
		ProjectPath:    tempDir,
		TargetFile:     specPath,
		NonInteractive: false,
		EnableAudit:    false,
	})

	require.NoError(t, err)
	assert.True(t, session.IsComplete)
	assert.Contains(t, out.String(), "Specification Revision Timeline")
}
