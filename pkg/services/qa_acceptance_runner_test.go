package services

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/diegojromerolopez/noctifab/pkg/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestQAAcceptanceRunner_HasAcceptanceTests(t *testing.T) {
	runner := NewQAAcceptanceRunner(nil)

	t.Run("returns false when tests/acceptance does not exist", func(t *testing.T) {
		tmpDir := t.TempDir()
		assert.False(t, runner.HasAcceptanceTests(tmpDir))
	})

	t.Run("returns true when acceptance test files exist", func(t *testing.T) {
		tmpDir := t.TempDir()
		accDir := filepath.Join(tmpDir, "tests", "acceptance")
		require.NoError(t, os.MkdirAll(accDir, 0755))
		require.NoError(t, os.WriteFile(filepath.Join(accDir, "test_api.py"), []byte("# test"), 0644))

		assert.True(t, runner.HasAcceptanceTests(tmpDir))
	})
}

func TestQAAcceptanceRunner_DetectAcceptanceCommand(t *testing.T) {
	runner := NewQAAcceptanceRunner(nil)

	t.Run("detects run_acceptance.sh when present", func(t *testing.T) {
		tmpDir := t.TempDir()
		accDir := filepath.Join(tmpDir, "tests", "acceptance")
		require.NoError(t, os.MkdirAll(accDir, 0755))
		require.NoError(t, os.WriteFile(filepath.Join(accDir, "run_acceptance.sh"), []byte("#!/bin/sh\nexit 0"), 0755))

		cmd := runner.DetectAcceptanceCommand(tmpDir)
		assert.Equal(t, "sh tests/acceptance/run_acceptance.sh", cmd)
	})

	t.Run("detects make acceptance when Makefile has target", func(t *testing.T) {
		tmpDir := t.TempDir()
		require.NoError(t, os.WriteFile(filepath.Join(tmpDir, "Makefile"), []byte("acceptance:\n\techo ok\n"), 0644))

		cmd := runner.DetectAcceptanceCommand(tmpDir)
		assert.Equal(t, "make acceptance", cmd)
	})

	t.Run("detects pytest for python workspace with acceptance dir", func(t *testing.T) {
		tmpDir := t.TempDir()
		require.NoError(t, os.MkdirAll(filepath.Join(tmpDir, "tests", "acceptance"), 0755))
		require.NoError(t, os.WriteFile(filepath.Join(tmpDir, "pyproject.toml"), []byte("[tool.pytest]\n"), 0644))

		cmd := runner.DetectAcceptanceCommand(tmpDir)
		assert.Equal(t, "pytest -v tests/acceptance", cmd)
	})
}

func TestQAAcceptanceRunner_RunAcceptanceTests(t *testing.T) {
	t.Run("returns passed when no acceptance tests present", func(t *testing.T) {
		tmpDir := t.TempDir()
		runner := NewQAAcceptanceRunner(nil)
		report, err := runner.RunAcceptanceTests(context.Background(), tmpDir)
		require.NoError(t, err)
		assert.True(t, report.Passed)
		assert.Contains(t, report.Summary, "No acceptance tests")
	})

	t.Run("executes command and parses success", func(t *testing.T) {
		tmpDir := t.TempDir()
		accDir := filepath.Join(tmpDir, "tests", "acceptance")
		require.NoError(t, os.MkdirAll(accDir, 0755))
		require.NoError(t, os.WriteFile(filepath.Join(accDir, "run_acceptance.sh"), []byte("#!/bin/sh"), 0755))

		sb := &scriptedSandbox{
			results: []error{nil},
			outputs: []string{"3 passed in 0.42s"},
		}
		runner := NewQAAcceptanceRunner(nil, sb)
		report, err := runner.RunAcceptanceTests(context.Background(), tmpDir)
		require.NoError(t, err)
		assert.True(t, report.Passed)
		assert.Equal(t, 3, report.ExecutedTests)
	})

	t.Run("fails when acceptance command fails and extracts errors", func(t *testing.T) {
		tmpDir := t.TempDir()
		accDir := filepath.Join(tmpDir, "tests", "acceptance")
		require.NoError(t, os.MkdirAll(accDir, 0755))
		require.NoError(t, os.WriteFile(filepath.Join(accDir, "run_acceptance.sh"), []byte("#!/bin/sh"), 0755))

		sb := &scriptedSandbox{
			results: []error{errors.New("exit status 1")},
			outputs: []string{"FAILED tests/acceptance/test_api.py::test_put_get - AssertionError: expected 201 got 500"},
		}
		runner := NewQAAcceptanceRunner(nil, sb)
		report, err := runner.RunAcceptanceTests(context.Background(), tmpDir)
		require.NoError(t, err)
		assert.False(t, report.Passed)
		assert.NotEmpty(t, report.Failures)
		assert.Contains(t, report.Failures[0], "AssertionError")
	})

	t.Run("fails when 0 tests were executed", func(t *testing.T) {
		tmpDir := t.TempDir()
		accDir := filepath.Join(tmpDir, "tests", "acceptance")
		require.NoError(t, os.MkdirAll(accDir, 0755))
		require.NoError(t, os.WriteFile(filepath.Join(accDir, "run_acceptance.sh"), []byte("#!/bin/sh"), 0755))

		sb := &scriptedSandbox{
			results: []error{nil},
			outputs: []string{"no tests ran in 0.01s (0 tests collected)"},
		}
		runner := NewQAAcceptanceRunner(nil, sb)
		report, err := runner.RunAcceptanceTests(context.Background(), tmpDir)
		require.NoError(t, err)
		assert.False(t, report.Passed)
		assert.Contains(t, report.Summary, "0 tests")
	})
}

type mockQAAcceptanceLLM struct {
	response *domain.LLMResponse
	err      error
}

func (m *mockQAAcceptanceLLM) Complete(ctx context.Context, prompt string) (*domain.LLMResponse, error) {
	return m.response, m.err
}

func TestQAAcceptanceRunner_SynthesizeAcceptanceTests(t *testing.T) {
	t.Run("synthesizes deterministic acceptance suite when LLM is nil", func(t *testing.T) {
		tmpDir := t.TempDir()
		require.NoError(t, os.WriteFile(filepath.Join(tmpDir, "SPEC.ste.md"), []byte("# T4 Specification\nPUT /obj\n"), 0644))

		state := &domain.State{ProjectPath: tmpDir}
		runner := NewQAAcceptanceRunner(nil)
		n, err := runner.SynthesizeAcceptanceTests(context.Background(), state)
		require.NoError(t, err)
		assert.GreaterOrEqual(t, n, 1)

		scriptPath := filepath.Join(tmpDir, "tests", "acceptance", "run_acceptance.sh")
		assert.FileExists(t, scriptPath)
	})

	t.Run("synthesizes files from LLM actions strictly under tests/acceptance", func(t *testing.T) {
		tmpDir := t.TempDir()
		require.NoError(t, os.WriteFile(filepath.Join(tmpDir, "SPEC.ste.md"), []byte("# T4 Specification\n"), 0644))

		mockLLM := &mockQAAcceptanceLLM{
			response: &domain.LLMResponse{
				Actions: []domain.LLMAction{
					{
						Tool: "write_file",
						Args: map[string]any{
							"path":    "tests/acceptance/test_spec.py",
							"content": "import requests\ndef test_acceptance(): assert True\n",
						},
					},
					{
						// Disallowed destination outside tests/acceptance
						Tool: "write_file",
						Args: map[string]any{
							"path":    "src/main.py",
							"content": "# hack",
						},
					},
				},
			},
		}

		state := &domain.State{ProjectPath: tmpDir}
		runner := NewQAAcceptanceRunner(mockLLM)
		n, err := runner.SynthesizeAcceptanceTests(context.Background(), state)
		require.NoError(t, err)
		assert.Equal(t, 1, n)

		assert.FileExists(t, filepath.Join(tmpDir, "tests", "acceptance", "test_spec.py"))
		assert.NoFileExists(t, filepath.Join(tmpDir, "src", "main.py"))
	})
}
