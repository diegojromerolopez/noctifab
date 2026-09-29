package services

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"syscall"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPreflightSystem(t *testing.T) {
	tmpDir := t.TempDir()

	t.Run("detects Python runtime and verifies toolchain", func(t *testing.T) {
		pyproject := filepath.Join(tmpDir, "pyproject.toml")
		err := os.WriteFile(pyproject, []byte("[project]\nname = \"sample\"\n"), 0600)
		require.NoError(t, err)

		system := &PreflightSystem{
			lookPath: func(file string) (string, error) {
				if file == "python3" {
					return "/usr/bin/python3", nil
				}
				return "", errors.New("not found")
			},
		}

		report, err := system.PreflightWorkspace(context.Background(), tmpDir, nil)
		require.NoError(t, err)
		assert.Contains(t, report.DetectedRuntimes, "python")
		assert.Empty(t, report.MissingBinaries)
		assert.True(t, report.ReadyForLLMGeneration)
		assert.Equal(t, "local", report.RecommendedStrategy)
	})

	t.Run("missing toolchain switches to docker if docker is available", func(t *testing.T) {
		cargoPath := filepath.Join(tmpDir, "Cargo.toml")
		err := os.WriteFile(cargoPath, []byte("[package]\nname = \"sample\"\n"), 0600)
		require.NoError(t, err)

		system := &PreflightSystem{
			lookPath: func(file string) (string, error) {
				if file == "docker" {
					return "/usr/local/bin/docker", nil
				}
				return "", errors.New("not found")
			},
		}

		report, err := system.PreflightWorkspace(context.Background(), tmpDir, nil)
		require.NoError(t, err)
		assert.Contains(t, report.DetectedRuntimes, "rust")
		assert.Contains(t, report.MissingBinaries, "cargo")
		assert.Equal(t, "docker", report.RecommendedStrategy)
	})

	t.Run("detects invalid Makefile target before running", func(t *testing.T) {
		makefilePath := filepath.Join(tmpDir, "Makefile")
		err := os.WriteFile(makefilePath, []byte("build:\n\t@echo build\n\ntest:\n\t@echo test\n"), 0600)
		require.NoError(t, err)

		system := &PreflightSystem{
			lookPath: func(file string) (string, error) {
				return "/usr/bin/" + file, nil
			},
		}

		commands := []string{"make build", "make format"}
		report, err := system.PreflightWorkspace(context.Background(), tmpDir, commands)
		require.NoError(t, err)
		assert.Len(t, report.InvalidCommands, 1)
		assert.Contains(t, report.InvalidCommands[0], "target \"format\" does not exist in Makefile")
	})

	t.Run("daemon project triggers port preemption and reaper", func(t *testing.T) {
		daemonDir := t.TempDir()
		specPath := filepath.Join(daemonDir, "SPEC.md")
		err := os.WriteFile(specPath, []byte("# Server Daemon\nRuns TCP wire protocol on port 6379\n"), 0600)
		require.NoError(t, err)

		system := &PreflightSystem{
			lookPath: func(file string) (string, error) {
				return "/usr/bin/" + file, nil
			},
			portReaper: &PortReaper{
				dialTimeout: 10 * 1000 * 1000,
				killSignal:  func(pid int, sig syscall.Signal) error { return nil },
				runLsof: func(ctx context.Context, port int) ([]int, error) {
					return []int{12345}, nil
				},
			},
		}

		report, err := system.PreflightWorkspace(context.Background(), daemonDir, nil)
		require.NoError(t, err)
		assert.True(t, report.IsDaemon)
		assert.Contains(t, report.DetectedPorts, 6379)
	})

	t.Run("CLI project skips port preemption", func(t *testing.T) {
		cliDir := t.TempDir()
		specPath := filepath.Join(cliDir, "SPEC.md")
		err := os.WriteFile(specPath, []byte("# Calculator CLI\nA simple CLI calculator.\n"), 0600)
		require.NoError(t, err)

		system := &PreflightSystem{
			lookPath: func(file string) (string, error) {
				return "/usr/bin/" + file, nil
			},
		}

		report, err := system.PreflightWorkspace(context.Background(), cliDir, nil)
		require.NoError(t, err)
		assert.False(t, report.IsDaemon)
		assert.Empty(t, report.DetectedPorts)
	})
}
