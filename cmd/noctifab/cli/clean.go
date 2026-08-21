package cli

import (
	"bufio"
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	_ "github.com/jackc/pgx/v5/stdlib"

	"github.com/diegojromerolopez/noctifab/pkg/infrastructure/config"
	"github.com/diegojromerolopez/noctifab/pkg/services"
	"github.com/spf13/cobra"
)

var cleanCmd = &cobra.Command{
	Use:   "clean",
	Short: "Remove all noctifab state and reset the project to a pristine condition",
	Long: `clean removes the noctifab database, PID file, logs, worktrees, steer stories, and ephemeral runtime data.
Use this to start completely fresh. In-flight daemon work will be lost.
Stop the daemon first with 'noctifab stop' if it is running.`,
	SilenceErrors: true,
	SilenceUsage:  true,
	RunE:          runClean,
}

// runClean is the main handler for the clean command (extracted for testability).
func runClean(cmd *cobra.Command, _ []string) error {
	dryRun, _ := cmd.Flags().GetBool("dry-run")
	yes, _ := cmd.Flags().GetBool("yes")

	// When --dry-run is set we skip the daemon check entirely (safe preview).
	if !dryRun {
		if pid, err := services.ReadPIDFile(daemonPIDFile); err == nil {
			fmt.Fprintf(os.Stderr,
				"⚠ noctifab daemon is running (PID %d). Run 'noctifab stop' before clean to save state.\n", pid)
			if !yes {
				confirmed, askErr := askConfirmation(cmd)
				if askErr != nil {
					return askErr
				}
				if !confirmed {
					fmt.Println("Aborted.")
					return nil
				}
			}
		}
	}

	cfg, err := config.Load(cmd)
	if err != nil {
		return err
	}

	if dryRun {
		return runDryClean(cfg)
	}

	if !yes {
		confirmed, askErr := askConfirmation(cmd)
		if askErr != nil {
			return askErr
		}
		if !confirmed {
			fmt.Println("Aborted.")
			return nil
		}
	}

	return runActualClean(cfg)
}

// askConfirmation prints the warning and reads a yes/no response from stdin.
func askConfirmation(cmd *cobra.Command) (bool, error) {
	fmt.Fprintln(os.Stderr, "⚠ This will permanently delete the database, PID file, and all log directories.")
	fmt.Fprint(os.Stderr, "Are you sure? [y/N]: ")

	reader := bufio.NewReader(cmd.InOrStdin())
	line, err := reader.ReadString('\n')
	if err != nil {
		return false, fmt.Errorf("failed to read confirmation: %w", err)
	}
	answer := strings.TrimSpace(strings.ToLower(line))
	return answer == "y" || answer == "yes", nil
}

// runDryClean prints what WOULD be deleted without performing any deletions.
func runDryClean(cfg *config.Config) error {
	// Collect items to check.
	dbPath := cfg.Storage.ConnString
	if strings.ToLower(cfg.Storage.Provider) != "postgres" {
		if dbPath == "" {
			dbPath = ".noctifab/data/noctifab.db"
		}
		printDryRunItem(dbPath)
		for _, ext := range []string{"-wal", "-shm", "-journal"} {
			printDryRunItem(dbPath + ext)
		}
	}

	printDryRunItem(".noctifab/data/metrics.json")
	printDryRunItem(daemonPIDFile)
	printDryRunItem(".noctifab/logs")
	printDryRunItem(".noctifab/worktrees")
	printDryRunItem(".noctifab/stories")

	fmt.Println("[dry-run] No files were deleted.")
	return nil
}

// printDryRunItem prints [dry-run] Would remove: <path> only when path exists.
func printDryRunItem(path string) {
	if _, err := os.Stat(path); err == nil {
		fmt.Printf("[dry-run] Would remove: %s\n", path)
	}
}

// runActualClean performs the real deletion of state files.
func runActualClean(cfg *config.Config) error {
	if strings.ToLower(cfg.Storage.Provider) == "postgres" {
		if err := cleanPostgres(cfg); err != nil {
			return err
		}
	} else {
		if err := cleanSQLiteDB(cfg); err != nil {
			return err
		}
	}

	cleanDataArtifacts()
	removePIDFile()
	removeLogs()
	cleanWorktrees()
	removeStories()

	fmt.Println("✅ noctifab state cleared. Run 'noctifab init' and 'noctifab start' to begin fresh.")
	return nil
}

func cleanPostgres(cfg *config.Config) error {
	db, err := sql.Open("pgx", cfg.Storage.ConnString)
	if err != nil {
		return fmt.Errorf("cannot connect to database: %w", err)
	}
	defer func() { _ = db.Close() }()

	tables := []string{
		"actions",
		"clarifications",
		"tasks",
		"workspace_files",
		"token_usage",
		"validation_criteria",
		"active_agents",
		"budget_usage",
		"state",
		"schema_migrations",
	}
	if err := validateTables(tables); err != nil {
		return err
	}
	for _, tbl := range tables {
		if _, dropErr := db.ExecContext(context.Background(), "DROP TABLE IF EXISTS "+tbl+" CASCADE"); dropErr != nil {
			return fmt.Errorf("failed to drop table %s: %w", tbl, dropErr)
		}
	}
	fmt.Println("Dropped all PostgreSQL noctifab tables.")
	return nil
}

func cleanSQLiteDB(cfg *config.Config) error {
	dbPath := cfg.Storage.ConnString
	if dbPath == "" {
		dbPath = ".noctifab/data/noctifab.db"
	}
	if _, err := os.Stat(dbPath); err == nil {
		if err := os.Remove(dbPath); err != nil {
			return fmt.Errorf("failed to remove database file %s: %w", dbPath, err)
		}
		fmt.Printf("Removed database: %s\n", dbPath)
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("failed to access database file %s: %w", dbPath, err)
	}

	// Clean auxiliary SQLite WAL, SHM, and rollback journal files if present
	for _, ext := range []string{"-wal", "-shm", "-journal"} {
		auxPath := dbPath + ext
		if _, err := os.Stat(auxPath); err == nil {
			if err := os.Remove(auxPath); err != nil {
				return fmt.Errorf("failed to remove database auxiliary file %s: %w", auxPath, err)
			}
		} else if !os.IsNotExist(err) {
			return fmt.Errorf("failed to access database auxiliary file %s: %w", auxPath, err)
		}
	}

	return nil
}

func cleanDataArtifacts() {
	dataDir := ".noctifab/data"
	entries, err := os.ReadDir(dataDir)
	if err != nil {
		return
	}
	for _, entry := range entries {
		name := entry.Name()
		if name == "metrics.json" || (strings.HasPrefix(name, "qa-test-") && strings.HasSuffix(name, ".patch")) {
			p := filepath.Join(dataDir, name)
			_ = os.Remove(p)
		}
	}
}

func removePIDFile() {
	if _, err := os.Stat(daemonPIDFile); err == nil {
		if err := services.RemovePIDFile(daemonPIDFile); err != nil {
			fmt.Fprintf(os.Stderr, "⚠ Could not remove PID file: %v\n", err)
		} else {
			fmt.Println("Removed PID file.")
		}
	} else if !os.IsNotExist(err) {
		fmt.Fprintf(os.Stderr, "⚠ Could not access PID file: %v\n", err)
	}
}

func removeLogs() {
	logDir := ".noctifab/logs"
	if _, err := os.Stat(logDir); err == nil {
		if err := os.RemoveAll(logDir); err != nil {
			fmt.Fprintf(os.Stderr, "⚠ Could not remove logs directory at %s: %v\n", logDir, err)
		} else {
			fmt.Printf("Removed logs: %s\n", logDir)
		}
	} else if !os.IsNotExist(err) {
		fmt.Fprintf(os.Stderr, "⚠ Could not access logs directory at %s: %v\n", logDir, err)
	}
}

func cleanWorktrees() {
	worktreeDir := ".noctifab/worktrees"
	git := services.NewGitClient(".")
	_, _ = git.Run(context.Background(), false, "worktree", "prune")

	if _, err := os.Stat(worktreeDir); err == nil {
		if err := os.RemoveAll(worktreeDir); err != nil {
			fmt.Fprintf(os.Stderr, "⚠ Could not remove worktrees at %s: %v\n", worktreeDir, err)
		} else {
			fmt.Printf("Removed worktrees: %s\n", worktreeDir)
		}
	} else if !os.IsNotExist(err) {
		fmt.Fprintf(os.Stderr, "⚠ Could not access worktrees at %s: %v\n", worktreeDir, err)
	}
}

func removeStories() {
	storiesDir := ".noctifab/stories"
	if _, err := os.Stat(storiesDir); err == nil {
		if err := os.RemoveAll(storiesDir); err != nil {
			fmt.Fprintf(os.Stderr, "⚠ Could not remove stories at %s: %v\n", storiesDir, err)
		} else {
			fmt.Printf("Removed steer stories: %s\n", storiesDir)
		}
	} else if !os.IsNotExist(err) {
		fmt.Fprintf(os.Stderr, "⚠ Could not access stories at %s: %v\n", storiesDir, err)
	}
}

func init() {
	cleanCmd.Flags().BoolP("yes", "y", false, "Skip confirmation prompt and proceed with clean")
	cleanCmd.Flags().Bool("dry-run", false, "Preview what would be deleted without actually deleting anything")
	RootCmd.AddCommand(cleanCmd)
}

func validateTables(tables []string) error {
	allowedTables := map[string]bool{
		"actions":             true,
		"clarifications":      true,
		"tasks":               true,
		"workspace_files":     true,
		"token_usage":         true,
		"state":               true,
		"schema_migrations":   true,
		"validation_criteria": true,
		"active_agents":       true,
		"budget_usage":        true,
	}
	for _, tbl := range tables {
		if !allowedTables[tbl] {
			return fmt.Errorf("failed to drop table: table name %q is not in the allowlist", tbl)
		}
	}
	return nil
}
