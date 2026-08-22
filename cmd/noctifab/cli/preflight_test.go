package cli

import (
	"os"
	"path/filepath"
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

func TestExtractMakefileToolchains(t *testing.T) {
	makefileContent := `
AARCH64 ?= aarch64-linux-gnu
AS := $(AARCH64)-as
LD := $(AARCH64)-ld
CC := gcc

all: bin/sqlarm
`
	tools := ExtractMakefileToolchains(makefileContent)
	assert.Contains(t, tools, "aarch64-linux-gnu-as")
	assert.Contains(t, tools, "aarch64-linux-gnu-ld")
	assert.Contains(t, tools, "gcc")
}

func TestDetectRequiredProjectToolchains(t *testing.T) {
	tmpDir := t.TempDir()
	makefilePath := filepath.Join(tmpDir, "Makefile")
	err := os.WriteFile(makefilePath, []byte("AARCH64 ?= aarch64-linux-gnu\nAS := $(AARCH64)-as\nLD := $(AARCH64)-ld\n"), 0644)
	require.NoError(t, err)

	tools := DetectRequiredProjectToolchains(tmpDir)
	assert.Contains(t, tools, "aarch64-linux-gnu-as")
	assert.Contains(t, tools, "aarch64-linux-gnu-ld")
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

func TestRunPreFlightChecks_MissingToolchainBinary(t *testing.T) {
	tmpDir := t.TempDir()
	makefilePath := filepath.Join(tmpDir, "Makefile")
	err := os.WriteFile(makefilePath, []byte("AS := nonexistent_cross_as_xyz_999\n"), 0644)
	require.NoError(t, err)

	cfg := &config.Config{
		Sandbox: config.SandboxConfig{
			Mode:        "host",
			TestCommand: "true",
		},
		LLM: config.LLMConfig{
			Provider:    "openai",
			APIKeyValue: "mock-key",
		},
	}

	err = runPreFlightChecks(cfg, tmpDir)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "required toolchain binary not found on host $PATH")
	assert.Contains(t, err.Error(), "nonexistent_cross_as_xyz_999")
}
