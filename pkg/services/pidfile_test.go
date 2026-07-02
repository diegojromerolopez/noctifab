package services_test

import (
	"os"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/diegojromerolopez/noctifab/pkg/services"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPIDFile_WriteAndRead(t *testing.T) {
	t.Run("when a PID file is written and read back, it returns the current PID", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, "noctifab.pid")

		err := services.WritePIDFile(path)
		require.NoError(t, err)

		pid, err := services.ReadPIDFile(path)
		require.NoError(t, err)
		assert.Equal(t, os.Getpid(), pid)
	})
}

func TestPIDFile_ReadMissing(t *testing.T) {
	t.Run("when the PID file does not exist, it returns a descriptive error", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, "noctifab.pid")

		pid, err := services.ReadPIDFile(path)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "not running")
		assert.Zero(t, pid)
	})
}

func TestPIDFile_ReadInvalid(t *testing.T) {
	t.Run("when the PID file contains garbage, it returns a parse error", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, "noctifab.pid")
		require.NoError(t, os.WriteFile(path, []byte("not-a-pid"), 0644))

		pid, err := services.ReadPIDFile(path)
		assert.Error(t, err)
		assert.Zero(t, pid)
	})
}

func TestPIDFile_ReadNegativePID(t *testing.T) {
	t.Run("when the PID file contains a negative number, it returns an invalid error", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, "noctifab.pid")
		require.NoError(t, os.WriteFile(path, []byte("-1"), 0644))

		pid, err := services.ReadPIDFile(path)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "invalid PID")
		assert.Zero(t, pid)
	})
}

func TestPIDFile_Remove(t *testing.T) {
	t.Run("when removing an existing PID file, it succeeds", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, "noctifab.pid")
		require.NoError(t, os.WriteFile(path, []byte(strconv.Itoa(os.Getpid())), 0644))

		err := services.RemovePIDFile(path)
		require.NoError(t, err)

		_, statErr := os.Stat(path)
		assert.True(t, os.IsNotExist(statErr))
	})
}

func TestPIDFile_RemoveMissing(t *testing.T) {
	t.Run("when removing a non-existent PID file, it is a no-op (no error)", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, "noctifab.pid")

		err := services.RemovePIDFile(path)
		assert.NoError(t, err)
	})
}
