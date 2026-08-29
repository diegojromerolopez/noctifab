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
	linterCmd := "flake8 ."
	cfg := &config.Config{
		Sandbox: config.SandboxConfig{
			TestCommand:      "pytest -v",
			LinterCommand:    &linterCmd,
			FormatterCommand: "black .",
		},
	}

	binaries := getRequiredSandboxBinaries(cfg)
	assert.Contains(t, binaries, "pytest")
	assert.Contains(t, binaries, "flake8")
	assert.Contains(t, binaries, "black")

	t.Run("ignore relative executable paths", func(t *testing.T) {
		cfgRel := &config.Config{
			Sandbox: config.SandboxConfig{
				TestCommand: "./bin/test_runner -v",
			},
		}
		binariesRel := getRequiredSandboxBinaries(cfgRel)
		assert.NotContains(t, binariesRel, "test_runner")
	})
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

func TestExtractMakefileToolchains_EdgeCases(t *testing.T) {
	t.Run("multiline line continuations and quotes", func(t *testing.T) {
		content := `
CC := \
    "clang" \
    -O2 -Wall
AS := '/usr/bin/aarch64-linux-gnu-as'
LD := /opt/bin/ld
`
		tools := ExtractMakefileToolchains(content)
		assert.Contains(t, tools, "clang")
		assert.Contains(t, tools, "aarch64-linux-gnu-as")
		assert.Contains(t, tools, "ld")
	})

	t.Run("cross compile prefix with trailing dash defaults", func(t *testing.T) {
		content := `
CROSS_COMPILE ?= aarch64-linux-gnu-
`
		tools := ExtractMakefileToolchains(content)
		assert.Contains(t, tools, "aarch64-linux-gnu-as")
		assert.Contains(t, tools, "aarch64-linux-gnu-ld")
		assert.Contains(t, tools, "aarch64-linux-gnu-gcc")
	})

	t.Run("case insensitivity and export syntax", func(t *testing.T) {
		content := `
export cc := nasm
export CXX ::= g++
c_compiler != which clang
`
		tools := ExtractMakefileToolchains(content)
		assert.Contains(t, tools, "nasm")
		assert.Contains(t, tools, "g++")
	})

	t.Run("circular references and shell commands ignored", func(t *testing.T) {
		content := `
A = $(B)
B = $(A)
CC = $(A)
CLEAN = rm -rf build
ECHO = echo "building"
`
		tools := ExtractMakefileToolchains(content)
		assert.NotContains(t, tools, "rm")
		assert.NotContains(t, tools, "echo")
	})

	t.Run("single character variable references", func(t *testing.T) {
		content := `
T = arm-none-eabi
AS = $T-as
LD = $T-ld
`
		tools := ExtractMakefileToolchains(content)
		assert.Contains(t, tools, "arm-none-eabi-as")
		assert.Contains(t, tools, "arm-none-eabi-ld")
	})
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

func TestDetectRequiredProjectToolchains_EdgeCases(t *testing.T) {
	t.Run("included makefiles and story contracts", func(t *testing.T) {
		tmpDir := t.TempDir()

		// Main Makefile with include
		mainMakefile := filepath.Join(tmpDir, "Makefile")
		err := os.WriteFile(mainMakefile, []byte("include config.mk\nCC := gcc\n"), 0644)
		require.NoError(t, err)

		// Included config.mk
		configMk := filepath.Join(tmpDir, "config.mk")
		err = os.WriteFile(configMk, []byte("AS := aarch64-linux-gnu-as\n"), 0644)
		require.NoError(t, err)

		// Story contract in roadmap/user-stories
		storiesDir := filepath.Join(tmpDir, "roadmap", "user-stories")
		err = os.MkdirAll(storiesDir, 0755)
		require.NoError(t, err)

		storyContent := `# Story US-101
` + "```" + `noctifab-contract
{
  "story_id": "US-101",
  "public_contracts": [
    {
      "id": "c1",
      "allowed_executables": ["./bin/app", "valgrind", "docker", "qemu-aarch64", "arm-none-eabi-gcc"],
      "exit_codes": [0]
    }
  ]
}
` + "```\n"
		err = os.WriteFile(filepath.Join(storiesDir, "US-101.md"), []byte(storyContent), 0644)
		require.NoError(t, err)

		tools := DetectRequiredProjectToolchains(tmpDir)
		assert.Contains(t, tools, "gcc")
		assert.Contains(t, tools, "aarch64-linux-gnu-as")
		assert.Contains(t, tools, "valgrind")
		assert.Contains(t, tools, "docker")
		assert.Contains(t, tools, "qemu-aarch64")
		assert.Contains(t, tools, "arm-none-eabi-gcc")
		// Project local binary must not be treated as a required host toolchain
		assert.NotContains(t, tools, "./bin/app")
		assert.NotContains(t, tools, "app")
	})

	t.Run("empty or missing project directory", func(t *testing.T) {
		tools := DetectRequiredProjectToolchains(filepath.Join(t.TempDir(), "nonexistent"))
		assert.Empty(t, tools)
	})
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
