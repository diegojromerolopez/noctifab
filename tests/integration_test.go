package tests

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/diegojromerolopez/noctifab/cmd/noctifab/cli"
	"github.com/spf13/cobra"
)

func TestInitCommand(t *testing.T) {
	// Set required environment variables for validation inside commands
	_ = os.Setenv("GITHUB_TOKEN", "test-token")
	defer func() { _ = os.Unsetenv("GITHUB_TOKEN") }()
	_ = os.Setenv("OPENAI_API_KEY", "test-api-key")
	defer func() { _ = os.Unsetenv("OPENAI_API_KEY") }()

	t.Run("Clean directory initialization", func(t *testing.T) {
		tmpDir := t.TempDir()

		// Save and restore global WorkspaceDir
		oldDir := cli.WorkspaceDir
		cli.WorkspaceDir = tmpDir
		defer func() { cli.WorkspaceDir = oldDir }()

		// Run init subcommand
		cli.RootCmd.SetArgs([]string{"init", "--vcs-clone-protocol", "https"})
		err := cli.RootCmd.Execute()
		if err != nil {
			t.Fatalf("expected init to succeed on clean dir, got: %v", err)
		}

		// Verify files created
		cfgPath := filepath.Join(tmpDir, ".noctifab", "config.yaml")
		if _, err := os.Stat(cfgPath); err != nil {
			t.Errorf("expected config.yaml to be created, got: %v", err)
		}

		dbPath := filepath.Join(tmpDir, ".noctifab", "config", "noctifab.db")
		if _, err := os.Stat(dbPath); err != nil {
			t.Errorf("expected noctifab.db to be created, got: %v", err)
		}

		gitIgnorePath := filepath.Join(tmpDir, ".noctifab", ".gitignore")
		if _, err := os.Stat(gitIgnorePath); os.IsNotExist(err) {
			t.Errorf("expected .gitignore to be created, got: %v", err)
		}

		profilePath := filepath.Join(tmpDir, ".noctifab", "profiles", "default.yaml")
		if _, err := os.Stat(profilePath); err != nil {
			t.Errorf("expected profiles/default.yaml to be created, got: %v", err)
		}

		// Run again (idempotent check)
		err = cli.RootCmd.Execute()
		if err != nil {
			t.Fatalf("expected init to be idempotent and succeed, got: %v", err)
		}
	})

	t.Run("Dirty directory (existing project files but no .noctifab) returns Exit Code 4", func(t *testing.T) {
		tmpDir := t.TempDir()

		// Create a mock project file
		dummyProjectFile := filepath.Join(tmpDir, "main.go")
		if err := os.WriteFile(dummyProjectFile, []byte("package main"), 0644); err != nil {
			t.Fatalf("failed to write dummy project file: %v", err)
		}

		// Save and restore global WorkspaceDir
		oldDir := cli.WorkspaceDir
		cli.WorkspaceDir = tmpDir
		defer func() { cli.WorkspaceDir = oldDir }()

		// Run init subcommand
		cli.RootCmd.SetArgs([]string{"init"})
		err := cli.RootCmd.Execute()
		if err == nil {
			t.Fatal("expected init to return exit error, got nil")
		}

		exitErr, ok := err.(*cli.ExitError)
		if !ok {
			t.Fatalf("expected ExitError, got type: %T (error: %v)", err, err)
		}

		if exitErr.Code != 4 {
			t.Errorf("expected exit code 4, got %d", exitErr.Code)
		}

		if !strings.Contains(exitErr.Msg, "Security Warning") {
			t.Errorf("expected security warning message, got: %s", exitErr.Msg)
		}
	})

	t.Run("Error if directory does not exist", func(t *testing.T) {
		oldDir := cli.WorkspaceDir
		cli.WorkspaceDir = "/nonexistent-path-987654"
		defer func() { cli.WorkspaceDir = oldDir }()

		cli.RootCmd.SetArgs([]string{"init"})
		err := cli.RootCmd.Execute()
		if err == nil {
			t.Fatal("expected error reading nonexistent directory")
		}
	})
}

func TestSubcommands_Success(t *testing.T) {
	// Setup valid config directory using t.TempDir()
	tmpDir := t.TempDir()

	// Write a valid config file and mock DB
	noctifabDir := filepath.Join(tmpDir, ".noctifab")
	if err := os.MkdirAll(filepath.Join(noctifabDir, "config"), 0755); err != nil {
		t.Fatalf("failed to create config dir: %v", err)
	}

	validYaml := `
config_version: "1.0"
vcs:
  repository: "owner/repo"
  token_env: "MOCK_VCS_TOKEN"
llm:
  provider: "openai"
  api_key_env: "MOCK_LLM_KEY"
`
	if err := os.WriteFile(filepath.Join(noctifabDir, "config.yaml"), []byte(validYaml), 0644); err != nil {
		t.Fatalf("failed to write config file: %v", err)
	}

	_ = os.Setenv("MOCK_VCS_TOKEN", "mock-vcs-token-val")
	defer func() { _ = os.Unsetenv("MOCK_VCS_TOKEN") }()
	_ = os.Setenv("MOCK_LLM_KEY", "mock-llm-key-val")
	defer func() { _ = os.Unsetenv("MOCK_LLM_KEY") }()

	// Set args to override config path and point to our temp config
	configFlag := filepath.Join(noctifabDir, "config.yaml")

	subcommands := []string{"start", "run-once", "validate", "plan", "maintenance"}

	for _, sub := range subcommands {
		t.Run("Command "+sub, func(t *testing.T) {
			cli.RootCmd.SetArgs([]string{sub, "--config", configFlag})
			err := cli.RootCmd.Execute()
			if err != nil {
				t.Fatalf("expected subcommand %s to succeed, got: %v", sub, err)
			}
		})
	}
}

func TestSubcommands_ValidationError(t *testing.T) {
	// Run start command with no token set, leading to validation error
	tmpDir := t.TempDir()
	noctifabDir := filepath.Join(tmpDir, ".noctifab")
	if err := os.MkdirAll(noctifabDir, 0755); err != nil {
		t.Fatalf("failed to create dir: %v", err)
	}

	invalidYaml := `
config_version: "1.0"
vcs:
  repository: ""
`
	if err := os.WriteFile(filepath.Join(noctifabDir, "config.yaml"), []byte(invalidYaml), 0644); err != nil {
		t.Fatalf("failed to write config file: %v", err)
	}

	configFlag := filepath.Join(noctifabDir, "config.yaml")

	cli.RootCmd.SetArgs([]string{"start", "--config", configFlag})
	err := cli.RootCmd.Execute()
	if err == nil {
		t.Fatal("expected validation error, got nil")
	}
}

func TestExitErrorString(t *testing.T) {
	err := &cli.ExitError{Code: 5, Msg: "custom error message"}
	if err.Error() != "custom error message" {
		t.Errorf("expected custom error message, got: %s", err.Error())
	}
}

func TestExecuteAndMain(t *testing.T) {
	// Save and restore OsExit
	oldExit := cli.OsExit
	defer func() { cli.OsExit = oldExit }()

	var exitedCode int
	cli.OsExit = func(code int) {
		exitedCode = code
	}

	t.Run("Execute success path", func(t *testing.T) {
		exitedCode = -1
		// Execute with a valid command
		cli.RootCmd.SetArgs([]string{"validate", "--config", "nonexistent.yaml"}) // Will fail validation but return error
		cli.Execute()
		if exitedCode != 1 {
			t.Errorf("expected exitedCode 1, got %d", exitedCode)
		}
	})

	t.Run("Execute ExitError path", func(t *testing.T) {
		exitedCode = -1
		// We'll execute a custom command that returns ExitError
		dummyCmd := &cobra.Command{
			Use: "dummy",
			RunE: func(cmd *cobra.Command, args []string) error {
				return &cli.ExitError{Code: 42, Msg: "dummy exit error"}
			},
		}
		cli.RootCmd.AddCommand(dummyCmd)
		defer cli.RootCmd.RemoveCommand(dummyCmd)

		cli.RootCmd.SetArgs([]string{"dummy"})
		cli.Execute()
		if exitedCode != 42 {
			t.Errorf("expected exitedCode 42, got %d", exitedCode)
		}
	})

	t.Run("Execute generic error path", func(t *testing.T) {
		exitedCode = -1
		// Execute with invalid flags
		cli.RootCmd.SetArgs([]string{"validate", "--nonexistent-flag-xyz"})
		cli.Execute()
		if exitedCode != 1 {
			t.Errorf("expected exitedCode 1, got %d", exitedCode)
		}
	})

	t.Run("Execute standard error path", func(t *testing.T) {
		exitedCode = -1
		// Register a subcommand that returns a standard error
		stdErrCmd := &cobra.Command{
			Use: "stderr-cmd",
			RunE: func(cmd *cobra.Command, args []string) error {
				return errors.New("some standard error")
			},
		}
		cli.RootCmd.AddCommand(stdErrCmd)
		defer cli.RootCmd.RemoveCommand(stdErrCmd)

		cli.RootCmd.SetArgs([]string{"stderr-cmd"})
		cli.Execute()
		if exitedCode != 1 {
			t.Errorf("expected exitedCode 1, got %d", exitedCode)
		}
	})
}
