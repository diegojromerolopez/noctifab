package cli

import (
	"testing"

	"github.com/diegojromerolopez/noctifab/pkg/infrastructure/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetRequiredSandboxBinaries(t *testing.T) {
	cfg := &config.Config{
		Profiles: map[string]config.ProfileConfig{
			"tester": {
				AllowedCommands: []string{"pytest", "python3"},
			},
		},
		Sandbox: config.SandboxConfig{
			AllowedCommands:  []string{"gcc", "make", "python3"},
			PackageManagers:  []string{"pip"},
			TestCommand:      "pytest -v",
			LinterCommand:    "flake8 .",
			FormatterCommand: "black .",
		},
	}

	binaries := getRequiredSandboxBinaries(cfg)
	assert.Contains(t, binaries, "gcc")
	assert.Contains(t, binaries, "make")
	assert.Contains(t, binaries, "python3")
	assert.Contains(t, binaries, "pytest")
	assert.Contains(t, binaries, "flake8")
}

func TestRunPreFlightChecks_MissingHostBinary(t *testing.T) {
	cfg := &config.Config{
		Sandbox: config.SandboxConfig{
			Mode:            "host",
			AllowedCommands: []string{"nonexistent_binary_xyz_123"},
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
