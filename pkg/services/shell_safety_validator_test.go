package services

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func mockLookPath(file string) (string, error) {
	if file == "go" || file == "git" || file == "python3" || file == "make" || file == "cargo" || file == "rm" {
		return "/usr/bin/" + file, nil
	}
	return "", fmt.Errorf("executable file not found in $PATH")
}

func TestShellSafetyValidator(t *testing.T) {
	validator := NewShellSafetyValidator(mockLookPath)

	t.Run("empty command fails", func(t *testing.T) {
		err := validator.ValidateShellCommand("   ")
		assert.ErrorContains(t, err, "empty shell command")
	})

	t.Run("valid command with installed binary passes", func(t *testing.T) {
		err := validator.ValidateShellCommand("go test -v ./...")
		require.NoError(t, err)
	})

	t.Run("valid command with env vars passes", func(t *testing.T) {
		err := validator.ValidateShellCommand("CGO_ENABLED=0 go build -o bin/app .")
		require.NoError(t, err)
	})

	t.Run("shell builtin command passes without lookPath", func(t *testing.T) {
		err := validator.ValidateShellCommand("echo 'Hello world'")
		require.NoError(t, err)
	})

	t.Run("cd command fails", func(t *testing.T) {
		err := validator.ValidateShellCommand("cd /tmp && go test")
		assert.ErrorContains(t, err, "forbidden (orchestrator manages working directory)")
	})

	t.Run("pushd command fails", func(t *testing.T) {
		err := validator.ValidateShellCommand("pushd src")
		assert.ErrorContains(t, err, "forbidden (orchestrator manages working directory)")
	})

	t.Run("interactive command fails", func(t *testing.T) {
		err := validator.ValidateShellCommand("vim main.go")
		assert.ErrorContains(t, err, "interactive utility \"vim\" would block headless execution")
	})

	t.Run("error masking with || true fails", func(t *testing.T) {
		err := validator.ValidateShellCommand("go test ./... || true")
		assert.ErrorContains(t, err, "error masking ('|| true', '|| exit 0', 'set +e') is forbidden")
	})

	t.Run("error masking with set +e fails", func(t *testing.T) {
		err := validator.ValidateShellCommand("set +e\nmake build")
		assert.ErrorContains(t, err, "error masking ('|| true', '|| exit 0', 'set +e') is forbidden")
	})

	t.Run("destructive deletion targeting root fails", func(t *testing.T) {
		err := validator.ValidateShellCommand("rm -rf /")
		assert.ErrorContains(t, err, "destructive deletion targeting root")
	})

	t.Run("destructive deletion targeting .git fails", func(t *testing.T) {
		err := validator.ValidateShellCommand("rm -rf .git")
		assert.ErrorContains(t, err, "destructive deletion targeting root")
	})

	t.Run("non-existent binary fails", func(t *testing.T) {
		err := validator.ValidateShellCommand("nonexistent_compiler_xyz --version")
		assert.ErrorContains(t, err, "not found on host PATH")
	})
}
