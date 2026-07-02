package services

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
)

// WritePIDFile writes the current process ID to the given path.
func WritePIDFile(path string) error {
	pid := os.Getpid()
	return os.WriteFile(path, []byte(strconv.Itoa(pid)), 0644)
}

// ReadPIDFile reads a PID from the given path and returns it as an integer.
// Returns an error if the file is missing or contains an invalid PID.
func ReadPIDFile(path string) (int, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return 0, fmt.Errorf("noctifab daemon is not running (PID file not found at %s)", path)
		}
		return 0, fmt.Errorf("failed to read PID file: %w", err)
	}

	pidStr := strings.TrimSpace(string(data))
	pid, err := strconv.Atoi(pidStr)
	if err != nil {
		return 0, fmt.Errorf("invalid PID value in file %s: %w", path, err)
	}
	if pid <= 0 {
		return 0, fmt.Errorf("invalid PID %d in file %s", pid, path)
	}
	return pid, nil
}

// RemovePIDFile deletes the PID file. If the file does not exist it is a no-op.
func RemovePIDFile(path string) error {
	err := os.Remove(path)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("failed to remove PID file %s: %w", path, err)
	}
	return nil
}
