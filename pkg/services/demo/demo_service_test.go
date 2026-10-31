package demo

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/diegojromerolopez/noctifab/pkg/infrastructure/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
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

func TestDemoService_RunWithDefaultCurrentDir(t *testing.T) {
	called := false
	var calledDir string
	runner := func(_ context.Context, dir string) error {
		called = true
		calledDir = dir
		return nil
	}
	svc := NewDemoService(runner)

	tempDir := t.TempDir()
	origWd, err := os.Getwd()
	require.NoError(t, err)
	defer func() { _ = os.Chdir(origWd) }()
	require.NoError(t, os.Chdir(tempDir))

	cfg := DemoConfig{
		Archetype: ArchetypeT4,
		TargetDir: "",
	}

	realTempDir, err := filepath.EvalSymlinks(tempDir)
	require.NoError(t, err)

	err = svc.Run(context.Background(), cfg)
	require.NoError(t, err)
	assert.True(t, called)
	realCalledDir, err := filepath.EvalSymlinks(calledDir)
	require.NoError(t, err)
	expectedDir := filepath.Join(realTempDir, "demo")
	assert.Equal(t, expectedDir, realCalledDir)
	assert.FileExists(t, filepath.Join(expectedDir, "SPEC.md"))
	assert.FileExists(t, filepath.Join(expectedDir, ".noctifab", "config.yaml"))
}

func TestDemoService_RunRequiresRunner(t *testing.T) {
	svc := NewDemoService()
	tempDir := filepath.Join(t.TempDir(), "no_runner_demo")
	cfg := DemoConfig{
		Archetype: ArchetypeT4,
		TargetDir: tempDir,
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

	for _, arch := range []DemoArchetype{ArchetypeT4, ArchetypeCLI} {
		archDir := t.TempDir()
		require.NoError(t, svc.unpackArchetype(arch, archDir))
		cfgBytes, err := os.ReadFile(filepath.Join(archDir, ".noctifab", "config.yaml"))
		require.NoError(t, err)

		parsedCfg := config.DefaultConfig()
		require.NoError(t, yaml.Unmarshal(cfgBytes, parsedCfg))

		assert.False(t, parsedCfg.Agents.Spike.IsEnabled(), "spike should be disabled in %s demo config", arch)
		assert.Equal(t, 1, parsedCfg.Agents.ProductManager.Iterations, "product_manager.iterations should be 1 in %s demo config", arch)
		assert.Equal(t, 1, parsedCfg.Agents.ProductManager.GetPasses(), "product_manager passes should be 1 in %s demo config", arch)
		require.NotEmpty(t, parsedCfg.Agents.ProductManager.Providers)
		assert.Equal(t, "claude-haiku-5-5", parsedCfg.Agents.ProductManager.Providers[0].Model)
		require.NotEmpty(t, parsedCfg.Agents.Planner.Providers)
		assert.Equal(t, "claude-haiku-5-5", parsedCfg.Agents.Planner.Providers[0].Model)
		require.NotEmpty(t, parsedCfg.Agents.QA.Providers)
		assert.Equal(t, "claude-haiku-5-5", parsedCfg.Agents.QA.Providers[0].Model)
		require.NotEmpty(t, parsedCfg.Agents.Auditor.Providers)
		assert.Equal(t, "claude-haiku-5-5", parsedCfg.Agents.Auditor.Providers[0].Model)
	}
}
