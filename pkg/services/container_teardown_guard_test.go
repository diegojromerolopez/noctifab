package services

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestContainerTeardownGuard(t *testing.T) {
	t.Run("DiscoverComposeFiles detects multiple compose files", func(t *testing.T) {
		tmpDir := t.TempDir()
		require.NoError(t, os.WriteFile(filepath.Join(tmpDir, "docker-compose.yml"), []byte("version: '3'"), 0600))
		require.NoError(t, os.WriteFile(filepath.Join(tmpDir, "docker-compose.e2e.yml"), []byte("version: '3'"), 0600))

		guard := NewContainerTeardownGuard(nil, nil)
		files := guard.DiscoverComposeFiles(tmpDir)

		assert.Contains(t, files, "docker-compose.yml")
		assert.Contains(t, files, "docker-compose.e2e.yml")
	})

	t.Run("TeardownProjectEnvironment executes compose down and cleans ports", func(t *testing.T) {
		tmpDir := t.TempDir()
		require.NoError(t, os.WriteFile(filepath.Join(tmpDir, "docker-compose.yml"), []byte("version: '3'\nservices:\n  redis:\n    ports:\n      - \"6379:6379\""), 0600))

		var executedCommands []string
		mockRunner := func(ctx context.Context, dir string, name string, args ...string) ([]byte, error) {
			executedCommands = append(executedCommands, name+" "+args[0]+" "+args[1]+" "+args[2])
			return []byte("stopped"), nil
		}

		// Mock port reaper that reports port is free
		mockReaper := &PortReaper{
			dialTimeout: 10 * time.Millisecond,
		}

		guard := NewContainerTeardownGuard(mockRunner, mockReaper)
		err := guard.TeardownProjectEnvironment(context.Background(), tmpDir)
		assert.NoError(t, err)

		require.Len(t, executedCommands, 1)
		assert.Equal(t, "docker compose -f docker-compose.yml", executedCommands[0])
	})

	t.Run("FormatTeardownSummary formats output correctly", func(t *testing.T) {
		guard := NewContainerTeardownGuard(nil, nil)
		s1 := guard.FormatTeardownSummary([]string{"docker-compose.yml"}, []int{6379, 8080})
		assert.Contains(t, s1, "tore down compose files: docker-compose.yml")
		assert.Contains(t, s1, "unbound port(s): [6379 8080]")

		s2 := guard.FormatTeardownSummary(nil, nil)
		assert.Equal(t, "clean environment (no containers or ports detected)", s2)
	})
}
