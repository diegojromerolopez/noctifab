package cli

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/diegojromerolopez/noctifab/pkg/infrastructure/config"
	"github.com/spf13/cobra"
)

var WorkspaceDir = "."

var initCmd = &cobra.Command{
	Use:           "init",
	Short:         "Initialize noctifab workspace and config",
	SilenceErrors: true,
	SilenceUsage:  true,
	RunE: func(cmd *cobra.Command, args []string) error {
		if len(args) > 0 {
			WorkspaceDir = args[0]
		}

		// 1. Directory cleanliness check
		entries, err := os.ReadDir(WorkspaceDir)
		if err != nil {
			return fmt.Errorf("failed to read target directory: %w", err)
		}

		hasNoctifab := false
		hasGit := false
		hasOtherFiles := false
		for _, entry := range entries {
			name := entry.Name()
			if name == ".noctifab" {
				hasNoctifab = true
			} else if name == ".git" {
				hasGit = true
			} else if name != ".tool-versions" && name != "." && name != ".." {
				hasOtherFiles = true
			}
		}

		// If no .noctifab folder AND it's not a git repository AND contains other project files, abort with code 4
		if !hasNoctifab && !hasGit && hasOtherFiles {
			return &ExitError{
				Code: 4,
				Msg:  "Security Warning: Target directory contains existing project files but is not initialized with .noctifab or .git. Aborting to prevent overwrite.",
			}
		}

		// 2. Create directory structure
		noctifabDir := filepath.Join(WorkspaceDir, ".noctifab")
		subDirs := []string{
			"data",
			"holdout",
			"logs",
			"profiles",
		}
		for _, sub := range subDirs {
			p := filepath.Join(noctifabDir, sub)
			if err := os.MkdirAll(p, 0755); err != nil {
				return fmt.Errorf("failed to create directory %s: %w", p, err)
			}
		}

		// 3. Generate default config.yaml if it doesn't exist
		cfgPath := filepath.Join(noctifabDir, "config.yaml")
		if _, err := os.Stat(cfgPath); os.IsNotExist(err) {
			if err := config.WriteDefaultConfig(cfgPath); err != nil {
				return fmt.Errorf("failed to write default config: %w", err)
			}
		}

		// 4. Initialize local SQLite database file if it doesn't exist
		dbPath := filepath.Join(noctifabDir, "data", "noctifab.db")
		if _, err := os.Stat(dbPath); os.IsNotExist(err) {
			f, err := os.OpenFile(dbPath, os.O_CREATE|os.O_WRONLY, 0644)
			if err != nil {
				return fmt.Errorf("failed to initialize database file: %w", err)
			}
			_ = f.Close()
		}

		// 5. Create local VCS ignore file (.noctifab/.gitignore) to exclude database, logs and lock files
		gitIgnorePath := filepath.Join(noctifabDir, ".gitignore")
		if _, err := os.Stat(gitIgnorePath); os.IsNotExist(err) {
			ignoreContent := "data/noctifab.db\nlogs/\nworktrees/\nnoctifab.pid\n"
			if err := os.WriteFile(gitIgnorePath, []byte(ignoreContent), 0644); err != nil {
				return fmt.Errorf("failed to create .gitignore: %w", err)
			}
		}

		// 6. Write default agent permission profiles
		defaultProfilePath := filepath.Join(noctifabDir, "profiles", "default.yaml")
		if _, err := os.Stat(defaultProfilePath); os.IsNotExist(err) {
			defaultProfile := `permissions:
  allowed_tools:
    - "run_tests"
    - "read_file"
    - "noop"
  network:
    allow_ai_provider: true
    allow_external: false
`
			if err := os.WriteFile(defaultProfilePath, []byte(defaultProfile), 0644); err != nil {
				return fmt.Errorf("failed to write default profile: %w", err)
			}
		}

		orchestratorProfilePath := filepath.Join(noctifabDir, "profiles", "orchestrator.yaml")
		if _, err := os.Stat(orchestratorProfilePath); os.IsNotExist(err) {
			orchestratorProfile := `permissions:
  allowed_tools:
    - "*"
  network:
    allow_ai_provider: true
    allow_external: true
`
			if err := os.WriteFile(orchestratorProfilePath, []byte(orchestratorProfile), 0644); err != nil {
				return fmt.Errorf("failed to write orchestrator profile: %w", err)
			}
		}

		fmt.Println("noctifab workspace initialized successfully.")
		return nil
	},
}

func init() {
	initCmd.Flags().String("vcs-clone-protocol", "https", "VCS clone protocol: https, ssh")
	RootCmd.AddCommand(initCmd)
}
