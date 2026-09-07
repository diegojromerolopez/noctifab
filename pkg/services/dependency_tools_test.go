package services

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/diegojromerolopez/noctifab/pkg/domain"
	"github.com/stretchr/testify/assert"
)

type mockSandboxRunner struct {
	lastCmd string
	out     string
	err     error
}

func (m *mockSandboxRunner) RunCommand(ctx context.Context, dir, command, pkg string) (string, error) {
	m.lastCmd = command
	return m.out, m.err
}

func (m *mockSandboxRunner) EvictTool(tool string)          {}
func (m *mockSandboxRunner) IsToolEvicted(tool string) bool { return false }
func (m *mockSandboxRunner) GetEvictedTools() []string      { return nil }

func TestInstallPackageTool(t *testing.T) {
	t.Run("when package argument is missing it returns an error", func(t *testing.T) {
		tool := &InstallPackageTool{}
		_, err := tool.Execute(context.Background(), &domain.State{}, map[string]any{})
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "missing or invalid 'package' argument")
	})

	t.Run("when manager is specified it constructs package manager command", func(t *testing.T) {
		runner := &mockSandboxRunner{out: "Successfully installed coverage"}
		tool := &InstallPackageTool{Runner: runner}

		res, err := tool.Execute(context.Background(), &domain.State{ProjectPath: "/test"}, map[string]any{
			"package": "coverage",
			"manager": "pip",
		})
		assert.NoError(t, err)
		assert.Equal(t, "pip install coverage", runner.lastCmd)
		assert.Contains(t, res, "installed successfully")
	})

	t.Run("when npm manager is specified it constructs npm command", func(t *testing.T) {
		runner := &mockSandboxRunner{out: "added vitest"}
		tool := &InstallPackageTool{Runner: runner}

		res, err := tool.Execute(context.Background(), &domain.State{ProjectPath: "/test"}, map[string]any{
			"package": "vitest",
			"manager": "npm",
		})
		assert.NoError(t, err)
		assert.Equal(t, "npm install -g vitest", runner.lastCmd)
		assert.Contains(t, res, "installed successfully")
	})

	t.Run("when manager is omitted it auto-detects from toolPackageMap", func(t *testing.T) {
		runner := &mockSandboxRunner{out: "added jest"}
		tool := &InstallPackageTool{Runner: runner}

		res, err := tool.Execute(context.Background(), &domain.State{ProjectPath: "/test"}, map[string]any{
			"package": "jest",
		})
		assert.NoError(t, err)
		assert.Equal(t, "npm install -g jest", runner.lastCmd)
		assert.Contains(t, res, "installed successfully")
	})

	t.Run("when manager is omitted it auto-detects from project manifest", func(t *testing.T) {
		tempDir := t.TempDir()
		assert.NoError(t, os.WriteFile(filepath.Join(tempDir, "package.json"), []byte("{}"), 0644))

		runner := &mockSandboxRunner{out: "added custom-pkg"}
		tool := &InstallPackageTool{Runner: runner}

		res, err := tool.Execute(context.Background(), &domain.State{ProjectPath: tempDir}, map[string]any{
			"package": "custom-pkg",
		})
		assert.NoError(t, err)
		assert.Equal(t, "npm install -g custom-pkg", runner.lastCmd)
		assert.Contains(t, res, "installed successfully")
	})

	t.Run("when gem manager is specified it constructs gem install command", func(t *testing.T) {
		runner := &mockSandboxRunner{out: "installed rspec"}
		tool := &InstallPackageTool{Runner: runner}

		res, err := tool.Execute(context.Background(), &domain.State{ProjectPath: "/test"}, map[string]any{
			"package": "rspec",
			"manager": "gem",
		})
		assert.NoError(t, err)
		assert.Equal(t, "gem install rspec", runner.lastCmd)
		assert.Contains(t, res, "installed successfully")
	})
}
