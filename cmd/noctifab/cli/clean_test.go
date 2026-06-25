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
			tables:  []string{"actions", "clarifications", "tasks", "state", "schema_migrations"},
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

func TestCleanCmd_OutputRedirected(t *testing.T) {
	_ = os.Setenv("GITHUB_TOKEN", "test-token")
	defer func() { _ = os.Unsetenv("GITHUB_TOKEN") }()
	_ = os.Setenv("OPENAI_API_KEY", "test-api-key")
	defer func() { _ = os.Unsetenv("OPENAI_API_KEY") }()

	// Keep track of old CWD and WorkspaceDir
	oldWd, err := os.Getwd()
	if err != nil {
		t.Fatalf("failed to get wd: %v", err)
	}
	defer func() { _ = os.Chdir(oldWd) }()

	tmpDir := t.TempDir()
	oldDir := WorkspaceDir
	WorkspaceDir = tmpDir
	defer func() { WorkspaceDir = oldDir }()

	err = os.Chdir(tmpDir)
	if err != nil {
		t.Fatalf("failed to chdir: %v", err)
	}

	// Create config directory and configuration file
	noctifabDir := filepath.Join(tmpDir, ".noctifab")
	if err := os.MkdirAll(filepath.Join(noctifabDir, "data"), 0755); err != nil {
		t.Fatalf("failed to create data dir: %v", err)
	}

	configYaml := `
config_version: "1.0"
vcs:
  repository: "owner/repo"
`
	if err := os.WriteFile(filepath.Join(noctifabDir, "config.yaml"), []byte(configYaml), 0644); err != nil {
		t.Fatalf("failed to write config file: %v", err)
	}

	// Test 1: Clean when no files exist
	stdout1, _, err1 := captureOutput(func() error {
		RootCmd.SetArgs([]string{"clean", "--config", filepath.Join(noctifabDir, "config.yaml")})
		return RootCmd.Execute()
	})
	if err1 != nil {
		t.Fatalf("RootCmd.Execute() failed: %v", err1)
	}

	if strings.Contains(stdout1, "Removed database:") {
		t.Errorf("expected no database removal log since db doesn't exist, got: %s", stdout1)
	}
	if strings.Contains(stdout1, "Removed PID file.") {
		t.Errorf("expected no PID file removal log, got: %s", stdout1)
	}
	if strings.Contains(stdout1, "Removed story logs:") {
		t.Errorf("expected no story logs removal log, got: %s", stdout1)
	}
	if strings.Contains(stdout1, "Removed daemon log:") {
		t.Errorf("expected no daemon log removal log, got: %s", stdout1)
	}

	// Test 2: Clean when files exist
	// Create mock files
	dbFile := filepath.Join(noctifabDir, "data", "noctifab.db")
	_ = os.WriteFile(dbFile, []byte("sqlite data"), 0644)

	pidFile := filepath.Join(noctifabDir, "noctifab.pid")
	_ = os.WriteFile(pidFile, []byte("12345"), 0644)

	logDir := filepath.Join(noctifabDir, "logs", "roadmap")
	_ = os.MkdirAll(logDir, 0755)
	_ = os.WriteFile(filepath.Join(logDir, "story.log"), []byte("log data"), 0644)

	daemonLog := filepath.Join(noctifabDir, "logs", "daemon.log")
	_ = os.MkdirAll(filepath.Dir(daemonLog), 0755)
	_ = os.WriteFile(daemonLog, []byte("daemon log"), 0644)

	stdout2, _, err2 := captureOutput(func() error {
		RootCmd.SetArgs([]string{"clean", "--config", filepath.Join(noctifabDir, "config.yaml"), "--force"})
		return RootCmd.Execute()
	})
	if err2 != nil {
		t.Fatalf("RootCmd.Execute() failed: %v", err2)
	}

	if !strings.Contains(stdout2, "Removed database:") {
		t.Errorf("expected database removal log, got: %s", stdout2)
	}
	if !strings.Contains(stdout2, "Removed PID file.") {
		t.Errorf("expected PID file removal log, got: %s", stdout2)
	}
	if !strings.Contains(stdout2, "Removed story logs:") {
		t.Errorf("expected story logs removal log, got: %s", stdout2)
	}
	if !strings.Contains(stdout2, "Removed daemon log:") {
		t.Errorf("expected daemon log removal log, got: %s", stdout2)
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
