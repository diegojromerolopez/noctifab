package e2e

import (
	"bytes"
	"context"
	"database/sql"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/diegojromerolopez/noctifab/pkg/infrastructure/storage"
	"github.com/diegojromerolopez/noctifab/pkg/services"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	_ "modernc.org/sqlite"
)

// getNoctifabBin returns the path to the compiled noctifab binary
func getNoctifabBin() string {
	bin := os.Getenv("NOCTIFAB_BIN")
	if bin == "" {
		bin = "/shared/noctifab"
	}
	return bin
}

func getTestEnv() []string {
	return append(os.Environ(),
		"GITHUB_TOKEN=test-token",
		"NOCTIFAB_VCS_REPO=owner/repo",
		"OPENAI_API_KEY=test-api-key",
		"NOCTIFAB_E2E=true",
	)
}

func TestE2E_Init_CleanDirectory(t *testing.T) {
	bin := getNoctifabBin()
	tempDir := t.TempDir()

	cmd := exec.Command(bin, "init")
	cmd.Dir = tempDir

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	require.NoError(t, err, "init failed: %s", stderr.String())

	// Verify directory structure
	noctifabDir := filepath.Join(tempDir, ".noctifab")
	assert.DirExists(t, filepath.Join(noctifabDir, "data"))
	assert.DirExists(t, filepath.Join(noctifabDir, "logs"))

	// Verify default config file
	assert.FileExists(t, filepath.Join(noctifabDir, "config.yaml"))

	// Verify database file
	assert.FileExists(t, filepath.Join(noctifabDir, "data", "noctifab.db"))

	// Verify .gitignore
	gitIgnorePath := filepath.Join(noctifabDir, ".gitignore")
	assert.FileExists(t, gitIgnorePath)
	content, err := os.ReadFile(gitIgnorePath)
	require.NoError(t, err)
	assert.True(t, strings.Contains(string(content), "data/noctifab.db"))
}

func TestE2E_Init_DirtyDirectory_SecurityExitCode4(t *testing.T) {
	bin := getNoctifabBin()
	tempDir := t.TempDir()

	// Create dummy project files
	err := os.WriteFile(filepath.Join(tempDir, "main.go"), []byte("package main"), 0644)
	require.NoError(t, err)

	cmd := exec.Command(bin, "init")
	cmd.Dir = tempDir

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err = cmd.Run()
	require.Error(t, err)

	// Check exit code
	exitErr, ok := err.(*exec.ExitError)
	require.True(t, ok, "expected ExitError")
	assert.Equal(t, 4, exitErr.ExitCode())

	// Verify security message
	assert.Contains(t, stderr.String(), "Security Warning")

	// Verify .noctifab directory was not created
	noctifabDir := filepath.Join(tempDir, ".noctifab")
	assert.NoDirExists(t, noctifabDir)
}

func TestE2E_Init_Idempotency(t *testing.T) {
	bin := getNoctifabBin()
	tempDir := t.TempDir()

	// First initialization
	cmd1 := exec.Command(bin, "init")
	cmd1.Dir = tempDir
	err := cmd1.Run()
	require.NoError(t, err)

	// Modify config to check that it isn't overwritten
	cfgPath := filepath.Join(tempDir, ".noctifab", "config.yaml")
	customConfig := "config_version: \"2.0\"\n"
	err = os.WriteFile(cfgPath, []byte(customConfig), 0644)
	require.NoError(t, err)

	// Second initialization (idempotency check)
	cmd2 := exec.Command(bin, "init")
	cmd2.Dir = tempDir
	err = cmd2.Run()
	require.NoError(t, err)

	// Verify custom configuration remains unchanged
	content, err := os.ReadFile(cfgPath)
	require.NoError(t, err)
	assert.Equal(t, customConfig, string(content))
}

func TestE2E_Validate_Configuration(t *testing.T) {
	bin := getNoctifabBin()
	tempDir := t.TempDir()

	// Initialize config first
	cmdInit := exec.Command(bin, "init")
	cmdInit.Dir = tempDir
	err := cmdInit.Run()
	require.NoError(t, err)

	// Run validation command
	cmdVal := exec.Command(bin, "validate")
	cmdVal.Dir = tempDir

	var stdout, stderr bytes.Buffer
	cmdVal.Stdout = &stdout
	cmdVal.Stderr = &stderr

	// Set required VCS token env variable and repo
	cmdVal.Env = getTestEnv()

	err = cmdVal.Run()
	require.NoError(t, err, "validation failed: %s", stderr.String())
	assert.Contains(t, stdout.String(), "Configuration is valid")
}

func TestE2E_StartCommand(t *testing.T) {
	bin := getNoctifabBin()
	tempDir := t.TempDir()

	cmdInit := exec.Command(bin, "init")
	cmdInit.Dir = tempDir
	err := cmdInit.Run()
	require.NoError(t, err)

	cmdStart := exec.Command(bin, "start")
	cmdStart.Dir = tempDir
	cmdStart.Env = getTestEnv()

	var stdout, stderr bytes.Buffer
	cmdStart.Stdout = &stdout
	cmdStart.Stderr = &stderr

	err = cmdStart.Run()
	require.NoError(t, err, "start failed: %s", stderr.String())
	assert.Contains(t, stdout.String(), "Pre-flight checks passed successfully.")
}

func TestE2E_MaintenanceCommand(t *testing.T) {
	bin := getNoctifabBin()
	tempDir := t.TempDir()

	cmdInit := exec.Command(bin, "init")
	cmdInit.Dir = tempDir
	err := cmdInit.Run()
	require.NoError(t, err)

	cmdMaint := exec.Command(bin, "maintenance")
	cmdMaint.Dir = tempDir
	cmdMaint.Env = getTestEnv()

	var stdout, stderr bytes.Buffer
	cmdMaint.Stdout = &stdout
	cmdMaint.Stderr = &stderr

	err = cmdMaint.Run()
	require.NoError(t, err, "maintenance failed: %s", stderr.String())
	assert.Contains(t, stdout.String(), "Maintenance completed successfully.")
}

func TestE2E_Database_Migration_Parity(t *testing.T) {
	bin := getNoctifabBin()
	tempDir := t.TempDir()

	cmd := exec.Command(bin, "init")
	cmd.Dir = tempDir
	err := cmd.Run()
	require.NoError(t, err)

	dbPath := filepath.Join(tempDir, ".noctifab", "data", "noctifab.db")
	require.FileExists(t, dbPath)

	// SQLite check
	repo, err := storage.NewSQLiteRepository(context.Background(), dbPath)
	require.NoError(t, err)
	defer func() { _ = repo.Close() }()

	db, err := sql.Open("sqlite", dbPath)
	require.NoError(t, err)
	defer func() { _ = db.Close() }()

	var version int
	err = db.QueryRow("SELECT version FROM schema_migrations WHERE version = 1").Scan(&version)
	assert.NoError(t, err)
	assert.Equal(t, 1, version)

	tables := []string{"state", "tasks", "clarifications", "actions", "workspace_files", "token_usage"}
	for _, table := range tables {
		rows, err := db.Query("SELECT * FROM " + table + " LIMIT 0")
		assert.NoError(t, err, "table %s should exist", table)
		if err == nil {
			_ = rows.Close()
		}
	}

	// PostgreSQL check if DSN is configured
	dsn := os.Getenv("NOCTIFAB_TEST_DB_DSN")
	if dsn != "" {
		pgRepo, err := storage.NewPostgresRepository(context.Background(), dsn, 1, 1)
		require.NoError(t, err)
		defer func() { _ = pgRepo.Close() }()

		pgDB, err := sql.Open("pgx", dsn)
		require.NoError(t, err)
		defer func() { _ = pgDB.Close() }()

		var pgVersion int
		err = pgDB.QueryRow("SELECT version FROM schema_migrations WHERE version = 1").Scan(&pgVersion)
		assert.NoError(t, err)
		assert.Equal(t, 1, pgVersion)

		for _, table := range tables {
			rows, err := pgDB.Query("SELECT * FROM " + table + " LIMIT 0")
			assert.NoError(t, err, "PostgreSQL table %s should exist", table)
			if err == nil {
				_ = rows.Close()
			}
		}
	}
}

func TestE2E_StartOneCommand(t *testing.T) {
	bin := getNoctifabBin()
	tempDir := t.TempDir()

	cmdInit := exec.Command(bin, "init")
	cmdInit.Dir = tempDir
	err := cmdInit.Run()
	require.NoError(t, err)

	cmdStartOne := exec.Command(bin, "start")
	cmdStartOne.Dir = tempDir
	cmdStartOne.Env = getTestEnv()

	var stdout, stderr bytes.Buffer
	cmdStartOne.Stdout = &stdout
	cmdStartOne.Stderr = &stderr

	err = cmdStartOne.Run()
	require.NoError(t, err, "start failed: %s", stderr.String())
	assert.Contains(t, stdout.String(), "Feature successfully implemented and validated.")
}

// ---------------------------------------------------------------------------
// Idle Timeout E2E tests – tests the full HostSandbox → Watchdog stack
// with real subprocess execution. These verify that the idle timeout
// configured via sandbox.idle_timeout_seconds kills hanging commands,
// returns partial output, and that periodic output resets the timer.
// ---------------------------------------------------------------------------

func TestE2E_IdleTimeout_KillsHangingCommand(t *testing.T) {
	s := services.NewHostSandbox([]string{"sleep"}, "", 100*time.Millisecond, nil)
	ctx := context.Background()
	_, err := s.RunCommand(ctx, t.TempDir(), "sleep 60", "")

	require.Error(t, err)
	assert.ErrorIs(t, err, services.ErrWatchdogIdleTimeout)
}

func TestE2E_IdleTimeout_OutputResetsTimer(t *testing.T) {
	tmp := t.TempDir()
	script := filepath.Join(tmp, "ping.sh")
	err := os.WriteFile(script, []byte("#!/bin/sh\nfor i in 1 2 3 4 5; do echo \"tick $i\"; sleep 0.02; done\n"), 0755)
	require.NoError(t, err)

	s := services.NewHostSandbox([]string{"sh"}, "", 200*time.Millisecond, nil)
	ctx := context.Background()
	out, err := s.RunCommand(ctx, tmp, "sh "+script, "")

	require.NoError(t, err)
	assert.Contains(t, out, "tick 5")
}

func TestE2E_IdleTimeout_ReturnsPartialOutputOnKill(t *testing.T) {
	tmp := t.TempDir()
	script := filepath.Join(tmp, "partial.sh")
	err := os.WriteFile(script, []byte("#!/bin/sh\necho \"BEFORE\"\nsleep 10\necho \"AFTER\"\n"), 0755)
	require.NoError(t, err)

	s := services.NewHostSandbox([]string{"sh"}, "", 100*time.Millisecond, nil)
	ctx := context.Background()
	out, err := s.RunCommand(ctx, tmp, "sh "+script, "")

	require.Error(t, err)
	assert.ErrorIs(t, err, services.ErrWatchdogIdleTimeout)
	assert.Contains(t, out, "BEFORE")
	assert.NotContains(t, out, "AFTER")
}

func TestE2E_IdleTimeout_WhitelistBlocksDisallowedCommand(t *testing.T) {
	s := services.NewHostSandbox([]string{"echo"}, "", 100*time.Millisecond, nil)

	ctx := context.Background()
	_, err := s.RunCommand(ctx, t.TempDir(), "sleep 1", "")

	require.Error(t, err)
	assert.Contains(t, err.Error(), "Sandbox violation")
	assert.NotContains(t, err.Error(), "idle timeout")
}

func TestE2E_IdleTimeout_ZeroTimeoutCompletesNormally(t *testing.T) {
	s := services.NewHostSandbox([]string{"echo"}, "", 0, nil)

	ctx := context.Background()
	out, err := s.RunCommand(ctx, t.TempDir(), "echo hello world", "")

	require.NoError(t, err)
	assert.Contains(t, out, "hello world")
}
