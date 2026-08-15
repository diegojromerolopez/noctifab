package reportfs_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/diegojromerolopez/noctifab/pkg/infrastructure/reportfs"
)

func TestPrepareReportDestination(t *testing.T) {
	tmpDir := t.TempDir()
	fsSys := reportfs.OSFileSystem{}

	t.Run("when destination directory exists", func(t *testing.T) {
		t.Run("it succeeds write probe", func(t *testing.T) {
			policy := reportfs.ReportDestinationPolicy{
				ProjectPath: tmpDir,
			}
			target := filepath.Join(tmpDir, ".noctifab", "reports", "report.md")
			err := reportfs.PrepareReportDestination(context.Background(), target, policy, fsSys)
			require.NoError(t, err)

			_, statErr := os.Stat(filepath.Dir(target))
			assert.NoError(t, statErr)
		})
	})

	t.Run("when target path is a directory", func(t *testing.T) {
		t.Run("it fails preparation", func(t *testing.T) {
			policy := reportfs.ReportDestinationPolicy{}
			dirPath := filepath.Join(tmpDir, "somedir")
			require.NoError(t, os.Mkdir(dirPath, 0755))

			err := reportfs.PrepareReportDestination(context.Background(), dirPath, policy, fsSys)
			assert.Error(t, err)
		})
	})

	t.Run("when destination matches protected config location", func(t *testing.T) {
		t.Run("it rejects preparation", func(t *testing.T) {
			cfgPath := filepath.Join(tmpDir, "config.yaml")
			policy := reportfs.ReportDestinationPolicy{
				ConfigPath: cfgPath,
			}
			err := reportfs.PrepareReportDestination(context.Background(), cfgPath, policy, fsSys)
			assert.Error(t, err)
		})
	})
}

func TestAtomicWriter(t *testing.T) {
	tmpDir := t.TempDir()
	fsSys := reportfs.OSFileSystem{}
	writer := reportfs.NewAtomicWriter(fsSys)

	t.Run("when writing atomic file", func(t *testing.T) {
		t.Run("it creates file with 0600 permissions and exact content", func(t *testing.T) {
			target := filepath.Join(tmpDir, "output.md")
			content := []byte("# Execution Report\n")

			err := writer.WriteAtomic(context.Background(), target, content)
			require.NoError(t, err)

			readBytes, readErr := os.ReadFile(target)
			require.NoError(t, readErr)
			assert.Equal(t, content, readBytes)

			info, statErr := os.Stat(target)
			require.NoError(t, statErr)
			assert.Equal(t, os.FileMode(0600), info.Mode().Perm())
		})
	})

	t.Run("when overwriting existing report file", func(t *testing.T) {
		t.Run("it updates file atomically", func(t *testing.T) {
			target := filepath.Join(tmpDir, "output.md")
			initial := []byte("# Initial\n")
			updated := []byte("# Updated\n")

			require.NoError(t, writer.WriteAtomic(context.Background(), target, initial))
			require.NoError(t, writer.WriteAtomic(context.Background(), target, updated))

			readBytes, err := os.ReadFile(target)
			require.NoError(t, err)
			assert.Equal(t, updated, readBytes)
		})
	})
}
