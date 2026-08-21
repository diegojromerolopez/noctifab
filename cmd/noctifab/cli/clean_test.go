package cli

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestValidateTables(t *testing.T) {
	tests := []struct {
		name    string
		tables  []string
		wantErr bool
	}{
		{
			name:    "all valid tables",
			tables:  []string{"actions", "clarifications", "tasks", "state", "schema_migrations", "validation_criteria", "active_agents", "budget_usage"},
			wantErr: false,
		},
		{
			name:    "empty list",
			tables:  []string{},
			wantErr: false,
		},
		{
			name:    "invalid table name",
			tables:  []string{"actions", "users", "tasks"},
			wantErr: true,
		},
		{
			name:    "malicious table name injection attempt",
			tables:  []string{"actions; DROP TABLE state; --"},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateTables(tt.tables)
			if (err != nil) != tt.wantErr {
				t.Errorf("validateTables() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

// setupCleanTestDir creates a temp directory, changes to it, writes a minimal
// config.yaml and returns teardown func.
func setupCleanTestDir(t *testing.T) (tmpDir string, noctifabDir string, teardown func()) {
	t.Helper()

	_ = os.Setenv("GITHUB_TOKEN", "test-token")
	_ = os.Setenv("OPENAI_API_KEY", "test-api-key")

	oldWd, err := os.Getwd()
	if err != nil {
		t.Fatalf("failed to get wd: %v", err)
	}
	oldDir := WorkspaceDir

	tmpDir = t.TempDir()
	WorkspaceDir = tmpDir

	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("failed to chdir: %v", err)
	}

	noctifabDir = filepath.Join(tmpDir, ".noctifab")
	if err := os.MkdirAll(filepath.Join(noctifabDir, "data"), 0755); err != nil {
		t.Fatalf("failed to create data dir: %v", err)
	}

	configYaml := `
config_version: "2.0"
vcs:
  repository: "owner/repo"
`
	if err := os.WriteFile(filepath.Join(noctifabDir, "config.yaml"), []byte(configYaml), 0644); err != nil {
		t.Fatalf("failed to write config file: %v", err)
	}

	teardown = func() {
		WorkspaceDir = oldDir
		_ = os.Chdir(oldWd)
		_ = os.Unsetenv("GITHUB_TOKEN")
		_ = os.Unsetenv("OPENAI_API_KEY")
	}
	return tmpDir, noctifabDir, teardown
}

func TestCleanCmd_NoFilesExist(t *testing.T) {
	_, noctifabDir, teardown := setupCleanTestDir(t)
	defer teardown()

	stdout, _, err := captureOutput(func() error {
		RootCmd.SetArgs([]string{"clean", "--config", filepath.Join(noctifabDir, "config.yaml"), "--yes"})
		return RootCmd.Execute()
	})
	if err != nil {
		t.Fatalf("RootCmd.Execute() failed: %v", err)
	}

	if strings.Contains(stdout, "Removed database:") {
		t.Errorf("expected no database removal log since db doesn't exist, got: %s", stdout)
	}
	if strings.Contains(stdout, "Removed PID file.") {
		t.Errorf("expected no PID file removal log, got: %s", stdout)
	}
	if strings.Contains(stdout, "Removed logs:") {
		t.Errorf("expected no logs removal log, got: %s", stdout)
	}
	if strings.Contains(stdout, "Removed worktrees:") {
		t.Errorf("expected no worktrees removal log, got: %s", stdout)
	}
	if strings.Contains(stdout, "Removed steer stories:") {
		t.Errorf("expected no steer stories removal log, got: %s", stdout)
	}
}

func TestCleanCmd_WithYes_FilesDeleted(t *testing.T) {
	_, noctifabDir, teardown := setupCleanTestDir(t)
	defer teardown()

	// Create mock files
	dbFile := filepath.Join(noctifabDir, "data", "noctifab.db")
	dbWal := filepath.Join(noctifabDir, "data", "noctifab.db-wal")
	dbShm := filepath.Join(noctifabDir, "data", "noctifab.db-shm")
	metricsFile := filepath.Join(noctifabDir, "data", "metrics.json")
	patchFile := filepath.Join(noctifabDir, "data", "qa-test-12345678.patch")
	_ = os.WriteFile(dbFile, []byte("sqlite data"), 0644)
	_ = os.WriteFile(dbWal, []byte("wal data"), 0644)
	_ = os.WriteFile(dbShm, []byte("shm data"), 0644)
	_ = os.WriteFile(metricsFile, []byte("{}"), 0644)
	_ = os.WriteFile(patchFile, []byte("patch"), 0644)

	pidFile := filepath.Join(noctifabDir, "noctifab.pid")
	_ = os.WriteFile(pidFile, []byte("12345"), 0644)

	logDir := filepath.Join(noctifabDir, "logs")
	_ = os.MkdirAll(filepath.Join(logDir, "roadmap"), 0755)
	_ = os.MkdirAll(filepath.Join(logDir, "tasks"), 0755)
	_ = os.WriteFile(filepath.Join(logDir, "roadmap", "story.log"), []byte("log data"), 0644)
	_ = os.WriteFile(filepath.Join(logDir, "tasks", "task-1.log"), []byte("task data"), 0644)
	_ = os.WriteFile(filepath.Join(logDir, "daemon.log"), []byte("daemon log"), 0644)

	worktreeDir := filepath.Join(noctifabDir, "worktrees")
	_ = os.MkdirAll(filepath.Join(worktreeDir, "task-1"), 0755)

	storiesDir := filepath.Join(noctifabDir, "stories")
	_ = os.MkdirAll(storiesDir, 0755)
	_ = os.WriteFile(filepath.Join(storiesDir, "order.md"), []byte("order"), 0644)

	stdout, _, err := captureOutput(func() error {
		RootCmd.SetArgs([]string{"clean", "--config", filepath.Join(noctifabDir, "config.yaml"), "--yes"})
		return RootCmd.Execute()
	})
	if err != nil {
		t.Fatalf("RootCmd.Execute() failed: %v", err)
	}

	if !strings.Contains(stdout, "Removed database:") {
		t.Errorf("expected database removal log, got: %s", stdout)
	}
	if !strings.Contains(stdout, "Removed PID file.") {
		t.Errorf("expected PID file removal log, got: %s", stdout)
	}
	if !strings.Contains(stdout, "Removed logs:") {
		t.Errorf("expected logs removal log, got: %s", stdout)
	}
	if !strings.Contains(stdout, "Removed worktrees:") {
		t.Errorf("expected worktrees removal log, got: %s", stdout)
	}
	if !strings.Contains(stdout, "Removed steer stories:") {
		t.Errorf("expected steer stories removal log, got: %s", stdout)
	}

	// Assert files are gone
	if _, statErr := os.Stat(dbFile); !os.IsNotExist(statErr) {
		t.Errorf("expected db file to be deleted, stat error: %v", statErr)
	}
	if _, statErr := os.Stat(dbWal); !os.IsNotExist(statErr) {
		t.Errorf("expected db-wal file to be deleted, stat error: %v", statErr)
	}
	if _, statErr := os.Stat(dbShm); !os.IsNotExist(statErr) {
		t.Errorf("expected db-shm file to be deleted, stat error: %v", statErr)
	}
	if _, statErr := os.Stat(metricsFile); !os.IsNotExist(statErr) {
		t.Errorf("expected metrics file to be deleted, stat error: %v", statErr)
	}
	if _, statErr := os.Stat(patchFile); !os.IsNotExist(statErr) {
		t.Errorf("expected patch file to be deleted, stat error: %v", statErr)
	}
	if _, statErr := os.Stat(pidFile); !os.IsNotExist(statErr) {
		t.Errorf("expected pid file to be deleted, stat error: %v", statErr)
	}
	if _, statErr := os.Stat(logDir); !os.IsNotExist(statErr) {
		t.Errorf("expected log dir to be deleted, stat error: %v", statErr)
	}
	if _, statErr := os.Stat(worktreeDir); !os.IsNotExist(statErr) {
		t.Errorf("expected worktree dir to be deleted, stat error: %v", statErr)
	}
	if _, statErr := os.Stat(storiesDir); !os.IsNotExist(statErr) {
		t.Errorf("expected stories dir to be deleted, stat error: %v", statErr)
	}
}

func TestCleanCmd_DryRun_FilesNotDeleted(t *testing.T) {
	_, noctifabDir, teardown := setupCleanTestDir(t)
	defer teardown()

	// Create mock files that should survive dry-run
	dbFile := filepath.Join(noctifabDir, "data", "noctifab.db")
	dbWal := filepath.Join(noctifabDir, "data", "noctifab.db-wal")
	metricsFile := filepath.Join(noctifabDir, "data", "metrics.json")
	patchFile := filepath.Join(noctifabDir, "data", "qa-test-12345678.patch")
	_ = os.WriteFile(dbFile, []byte("sqlite data"), 0644)
	_ = os.WriteFile(dbWal, []byte("wal data"), 0644)
	_ = os.WriteFile(metricsFile, []byte("{}"), 0644)
	_ = os.WriteFile(patchFile, []byte("patch data"), 0644)

	pidFile := filepath.Join(noctifabDir, "noctifab.pid")
	_ = os.WriteFile(pidFile, []byte("12345"), 0644)

	logDir := filepath.Join(noctifabDir, "logs")
	_ = os.MkdirAll(filepath.Join(logDir, "roadmap"), 0755)
	_ = os.WriteFile(filepath.Join(logDir, "roadmap", "story.log"), []byte("log data"), 0644)

	worktreeDir := filepath.Join(noctifabDir, "worktrees")
	_ = os.MkdirAll(worktreeDir, 0755)

	storiesDir := filepath.Join(noctifabDir, "stories")
	_ = os.MkdirAll(storiesDir, 0755)

	stdout, _, err := captureOutput(func() error {
		RootCmd.SetArgs([]string{"clean", "--config", filepath.Join(noctifabDir, "config.yaml"), "--dry-run"})
		return RootCmd.Execute()
	})
	if err != nil {
		t.Fatalf("RootCmd.Execute() failed: %v", err)
	}

	// Output must contain dry-run prefix
	if !strings.Contains(stdout, "[dry-run] Would remove:") {
		t.Errorf("expected dry-run output, got: %s", stdout)
	}
	if !strings.Contains(stdout, "[dry-run] No files were deleted.") {
		t.Errorf("expected dry-run summary line, got: %s", stdout)
	}

	// Files must still exist
	if _, statErr := os.Stat(dbFile); os.IsNotExist(statErr) {
		t.Error("expected db file to still exist after dry-run")
	}
	if _, statErr := os.Stat(dbWal); os.IsNotExist(statErr) {
		t.Error("expected db-wal file to still exist after dry-run")
	}
	if _, statErr := os.Stat(metricsFile); os.IsNotExist(statErr) {
		t.Error("expected metrics file to still exist after dry-run")
	}
	if _, statErr := os.Stat(patchFile); os.IsNotExist(statErr) {
		t.Error("expected patch file to still exist after dry-run")
	}
	if _, statErr := os.Stat(pidFile); os.IsNotExist(statErr) {
		t.Error("expected pid file to still exist after dry-run")
	}
	if _, statErr := os.Stat(logDir); os.IsNotExist(statErr) {
		t.Error("expected log dir to still exist after dry-run")
	}
	if _, statErr := os.Stat(worktreeDir); os.IsNotExist(statErr) {
		t.Error("expected worktree dir to still exist after dry-run")
	}
	if _, statErr := os.Stat(storiesDir); os.IsNotExist(statErr) {
		t.Error("expected stories dir to still exist after dry-run")
	}
}

// captureOutput captures stdout and stderr of a function execution
func captureOutput(f func() error) (string, string, error) {
	oldStdout := os.Stdout
	oldStderr := os.Stderr

	rOut, wOut, _ := os.Pipe()
	rErr, wErr, _ := os.Pipe()

	os.Stdout = wOut
	os.Stderr = wErr

	outChan := make(chan string)
	errChan := make(chan string)

	go func() {
		var buf bytes.Buffer
		_, _ = io.Copy(&buf, rOut)
		outChan <- buf.String()
	}()

	go func() {
		var buf bytes.Buffer
		_, _ = io.Copy(&buf, rErr)
		errChan <- buf.String()
	}()

	err := f()

	_ = wOut.Close()
	_ = wErr.Close()

	os.Stdout = oldStdout
	os.Stderr = oldStderr

	stdout := <-outChan
	stderr := <-errChan

	return stdout, stderr, err
}
