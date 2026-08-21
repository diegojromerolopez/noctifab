package cli

import (
	"testing"

	"github.com/diegojromerolopez/noctifab/pkg/infrastructure/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetRequiredSandboxBinaries(t *testing.T) {
	cfg := &config.Config{
		Sandbox: config.SandboxConfig{
			TestCommand:      "pytest -v",
			LinterCommand:    "flake8 .",
			FormatterCommand: "black .",
		},
	}

	binaries := getRequiredSandboxBinaries(cfg)
	assert.Contains(t, binaries, "pytest")
	assert.Contains(t, binaries, "flake8")
	assert.Contains(t, binaries, "black")
}

func TestRunPreFlightChecks_MissingHostBinary(t *testing.T) {
	cfg := &config.Config{
		Sandbox: config.SandboxConfig{
			Mode:        "host",
			TestCommand: "nonexistent_binary_xyz_123 -v",
		},
		LLM: config.LLMConfig{
			Provider:    "openai",
			APIKeyValue: "mock-key",
		},
	}

	err := runPreFlightChecks(cfg)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "required sandbox binary not found on host $PATH")
	assert.Contains(t, err.Error(), "nonexistent_binary_xyz_123")
}

