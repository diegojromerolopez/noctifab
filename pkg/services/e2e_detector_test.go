package services

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/diegojromerolopez/noctifab/pkg/infrastructure/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDetectE2ECommand(t *testing.T) {
	t.Run("custom command override always wins regardless of mode", func(t *testing.T) {
		tmpDir := t.TempDir()
		assert.Equal(t, "custom-e2e-run", DetectE2ECommand(tmpDir, "docker", "custom-e2e-run"))
		assert.Equal(t, "custom-e2e-run", DetectE2ECommand(tmpDir, "native", "custom-e2e-run"))
		assert.Equal(t, "custom-e2e-run", DetectE2ECommand(tmpDir, "", "custom-e2e-run"))
	})

	t.Run("docker mode detection hierarchy", func(t *testing.T) {
		tmpDir := t.TempDir()

		// 1. Empty directory
		assert.Empty(t, DetectE2ECommand(tmpDir, "docker", ""))
		assert.Empty(t, DetectE2ECommand(tmpDir, "", "")) // Default mode is docker

		// 2. Makefile fallback
		require.NoError(t, os.WriteFile(filepath.Join(tmpDir, "Makefile"), []byte("e2e:\n\t@echo testing\n"), 0600))
		assert.Equal(t, "make e2e", DetectE2ECommand(tmpDir, "docker", ""))

		// 3. docker-compose.yml with e2e: target beats Makefile
		require.NoError(t, os.WriteFile(filepath.Join(tmpDir, "docker-compose.yml"), []byte("services:\n  e2e:\n    image: test\n"), 0600))
		assert.Equal(t, "docker compose up --build --exit-code-from e2e", DetectE2ECommand(tmpDir, "docker", ""))

		// 4. docker-compose.e2e.yml beats docker-compose.yml
		require.NoError(t, os.WriteFile(filepath.Join(tmpDir, "docker-compose.e2e.yml"), []byte("services:\n  test-runner:\n    image: test\n"), 0600))
		assert.Equal(t, "docker compose -f docker-compose.e2e.yml up --build --exit-code-from test-runner", DetectE2ECommand(tmpDir, "docker", ""))
	})

	t.Run("native mode detection hierarchy", func(t *testing.T) {
		t.Run("prefers Makefile e2e target if present", func(t *testing.T) {
			tmpDir := t.TempDir()
			require.NoError(t, os.WriteFile(filepath.Join(tmpDir, "Makefile"), []byte("e2e:\n\tpytest tests/e2e\n"), 0600))
			assert.Equal(t, "make e2e", DetectE2ECommand(tmpDir, "native", ""))
		})

		t.Run("detects node package.json test:e2e script", func(t *testing.T) {
			tmpDir := t.TempDir()
			require.NoError(t, os.WriteFile(filepath.Join(tmpDir, "package.json"), []byte(`{"scripts": {"test:e2e": "playwright test"}}`), 0600))
			assert.Equal(t, "npm run test:e2e", DetectE2ECommand(tmpDir, "native", ""))
		})

		t.Run("detects node package.json e2e script", func(t *testing.T) {
			tmpDir := t.TempDir()
			require.NoError(t, os.WriteFile(filepath.Join(tmpDir, "package.json"), []byte(`{"scripts": {"e2e": "cypress run"}}`), 0600))
			assert.Equal(t, "npm run e2e", DetectE2ECommand(tmpDir, "native", ""))
		})

		t.Run("detects Python tests/e2e directory", func(t *testing.T) {
			tmpDir := t.TempDir()
			require.NoError(t, os.MkdirAll(filepath.Join(tmpDir, "tests", "e2e"), 0755))
			require.NoError(t, os.WriteFile(filepath.Join(tmpDir, "requirements.txt"), []byte("pytest\n"), 0600))
			cmd := DetectE2ECommand(tmpDir, "native", "")
			assert.True(t, cmd == "pytest tests/e2e" || cmd == "uv run pytest tests/e2e", "unexpected python e2e command: %s", cmd)
		})

		t.Run("detects Python test/e2e directory", func(t *testing.T) {
			tmpDir := t.TempDir()
			require.NoError(t, os.MkdirAll(filepath.Join(tmpDir, "test", "e2e"), 0755))
			require.NoError(t, os.WriteFile(filepath.Join(tmpDir, "setup.py"), []byte("# setup"), 0600))
			cmd := DetectE2ECommand(tmpDir, "native", "")
			assert.True(t, cmd == "pytest test/e2e" || cmd == "uv run pytest test/e2e", "unexpected python e2e command: %s", cmd)
		})

		t.Run("detects Rust Cargo.toml with tests/e2e directory", func(t *testing.T) {
			tmpDir := t.TempDir()
			require.NoError(t, os.MkdirAll(filepath.Join(tmpDir, "tests", "e2e"), 0755))
			require.NoError(t, os.WriteFile(filepath.Join(tmpDir, "Cargo.toml"), []byte("[package]\nname = \"foo\"\n"), 0600))
			assert.Equal(t, "cargo test --test e2e", DetectE2ECommand(tmpDir, "native", ""))
		})

		t.Run("detects Rust Cargo.toml with tests/e2e.rs file", func(t *testing.T) {
			tmpDir := t.TempDir()
			require.NoError(t, os.MkdirAll(filepath.Join(tmpDir, "tests"), 0755))
			require.NoError(t, os.WriteFile(filepath.Join(tmpDir, "tests", "e2e.rs"), []byte("// e2e test"), 0600))
			require.NoError(t, os.WriteFile(filepath.Join(tmpDir, "Cargo.toml"), []byte("[package]\nname = \"foo\"\n"), 0600))
			assert.Equal(t, "cargo test --test e2e", DetectE2ECommand(tmpDir, "native", ""))
		})

		t.Run("detects Go go.mod with tests/e2e directory", func(t *testing.T) {
			tmpDir := t.TempDir()
			require.NoError(t, os.MkdirAll(filepath.Join(tmpDir, "tests", "e2e"), 0755))
			require.NoError(t, os.WriteFile(filepath.Join(tmpDir, "go.mod"), []byte("module foo\n"), 0600))
			assert.Equal(t, "go test -v ./tests/e2e/...", DetectE2ECommand(tmpDir, "native", ""))
		})

		t.Run("falls back to Makefile test: target in native mode", func(t *testing.T) {
			tmpDir := t.TempDir()
			require.NoError(t, os.WriteFile(filepath.Join(tmpDir, "Makefile"), []byte("test:\n\tgo test ./...\n"), 0600))
			assert.Equal(t, "make test", DetectE2ECommand(tmpDir, "native", ""))
		})

		t.Run("returns empty when no native test markers exist", func(t *testing.T) {
			tmpDir := t.TempDir()
			assert.Empty(t, DetectE2ECommand(tmpDir, "native", ""))
		})
	})
}

func TestAuditorE2EConfig(t *testing.T) {
	tmpDir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(tmpDir, "Makefile"), []byte("e2e:\n\tpytest tests/e2e\n"), 0600))
	require.NoError(t, os.WriteFile(filepath.Join(tmpDir, "docker-compose.e2e.yml"), []byte("version: '3'"), 0600))

	t.Run("AcceptanceAuditor respects SetE2EMode and SetE2EConfig", func(t *testing.T) {
		auditor := NewAcceptanceAuditor(nil, nil)

		// Default docker mode picks docker-compose.e2e.yml
		assert.Equal(t, "docker compose -f docker-compose.e2e.yml up --build --exit-code-from test-runner", auditor.detectE2ECommand(tmpDir))

		// Switching to native mode picks make e2e
		auditor.SetE2EMode("native")
		assert.Equal(t, "make e2e", auditor.detectE2ECommand(tmpDir))

		// Switching via SetE2EConfig with command override
		auditor.SetE2EConfig(config.E2EConfig{Mode: "native", Command: "make custom-e2e"})
		assert.Equal(t, "make custom-e2e", auditor.detectE2ECommand(tmpDir))
	})

	t.Run("StoryQAAuditor respects SetE2EMode and SetE2EConfig", func(t *testing.T) {
		auditor := NewStoryQAAuditor(nil)

		// Default docker mode picks docker-compose.e2e.yml
		assert.Equal(t, "docker compose -f docker-compose.e2e.yml up --build --exit-code-from test-runner", auditor.detectE2ECommand(tmpDir))

		// Switching to native mode picks make e2e
		auditor.SetE2EMode("native")
		assert.Equal(t, "make e2e", auditor.detectE2ECommand(tmpDir))

		// Switching via SetE2EConfig with command override
		auditor.SetE2EConfig(config.E2EConfig{Mode: "docker", Command: "docker run e2e"})
		assert.Equal(t, "docker run e2e", auditor.detectE2ECommand(tmpDir))
	})
}
