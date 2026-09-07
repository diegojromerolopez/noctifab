package services

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
)

type testDeterministicSandboxRunner struct {
	executedCommands []string
}

func (m *testDeterministicSandboxRunner) RunCommand(_ context.Context, _ string, command string, _ string) (string, error) {
	m.executedCommands = append(m.executedCommands, command)
	return "", nil
}

func TestRunDeterministicAutoFormat_GoProject(t *testing.T) {
	tempDir := t.TempDir()
	_ = os.WriteFile(filepath.Join(tempDir, "go.mod"), []byte("module example.com/test\n"), 0644)
	_ = os.WriteFile(filepath.Join(tempDir, "main.go"), []byte("package main\n"), 0644)

	runner := &testDeterministicSandboxRunner{}
	RunDeterministicAutoFormat(context.Background(), runner, tempDir)

	assert.Contains(t, runner.executedCommands, "gofmt -w .")
}

func TestRunDeterministicAutoFormat_RustProject(t *testing.T) {
	tempDir := t.TempDir()
	_ = os.WriteFile(filepath.Join(tempDir, "Cargo.toml"), []byte("[package]\nname = \"test\"\n"), 0644)

	runner := &testDeterministicSandboxRunner{}
	RunDeterministicAutoFormat(context.Background(), runner, tempDir)

	assert.Contains(t, runner.executedCommands, "cargo fmt --all || true")
}

func TestRunDeterministicAutoFormat_PythonProject(t *testing.T) {
	tempDir := t.TempDir()
	_ = os.WriteFile(filepath.Join(tempDir, "pyproject.toml"), []byte("[tool.poetry]\nname = \"test\"\n"), 0644)

	runner := &testDeterministicSandboxRunner{}
	RunDeterministicAutoFormat(context.Background(), runner, tempDir)

	assert.Contains(t, runner.executedCommands, "ruff format . 2>/dev/null || black . 2>/dev/null || true")
}

func TestRunDeterministicAutoFormat_RubyProject(t *testing.T) {
	tempDir := t.TempDir()
	_ = os.WriteFile(filepath.Join(tempDir, "Gemfile"), []byte("source 'https://rubygems.org'\n"), 0644)

	runner := &testDeterministicSandboxRunner{}
	RunDeterministicAutoFormat(context.Background(), runner, tempDir)

	assert.Contains(t, runner.executedCommands, "bundle exec rubocop -A 2>/dev/null || rubocop -A 2>/dev/null || true")
}

func TestRunDeterministicAutoFormat_NilRunnerOrEmptyPath(t *testing.T) {
	// Must not panic
	RunDeterministicAutoFormat(context.Background(), nil, "/tmp")
	runner := &testDeterministicSandboxRunner{}
	RunDeterministicAutoFormat(context.Background(), runner, "")
	assert.Empty(t, runner.executedCommands)
}
