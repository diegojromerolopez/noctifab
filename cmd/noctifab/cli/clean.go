package cli

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"strings"

	_ "github.com/jackc/pgx/v5/stdlib"

	"github.com/diegojromerolopez/noctifab/pkg/infrastructure/config"
	"github.com/diegojromerolopez/noctifab/pkg/usecase"
	"github.com/spf13/cobra"
)

var cleanCmd = &cobra.Command{
	Use:   "clean",
	Short: "Remove all noctifab state and reset the project to a pristine condition",
	Long: `clean removes the noctifab database, PID file, and per-story logs.
Use this to start completely fresh. In-flight daemon work will be lost.
Stop the daemon first with 'noctifab stop' if it is running.`,
	SilenceErrors: true,
	SilenceUsage:  true,
	RunE: func(cmd *cobra.Command, args []string) error {
		// Warn if daemon is still running.
		if pid, err := usecase.ReadPIDFile(daemonPIDFile); err == nil {
			fmt.Fprintf(os.Stderr,
				"⚠ noctifab daemon is running (PID %d). Run 'noctifab stop' before clean to save state.\n", pid)
			force, _ := cmd.Flags().GetBool("force")
			if !force {
				return fmt.Errorf("aborting: daemon is still running. Use --force to override")
			}
		}

		cfg, err := config.Load(cmd)
		if err != nil {
			return err
		}

		// Drop and recreate the database schema (effectively wiping all state).
		if strings.ToLower(cfg.Storage.Provider) == "postgres" {
			// For PostgreSQL, drop all noctifab tables directly using database/sql.
			db, dbErr := sql.Open("pgx", cfg.Storage.ConnString)
			if dbErr != nil {
				return fmt.Errorf("cannot connect to database: %w", dbErr)
			}
			defer func() { _ = db.Close() }()
			tables := []string{"actions", "clarifications", "tasks", "workspace_files", "token_usage", "state", "schema_migrations"}
			if err := validateTables(tables); err != nil {
				return err
			}
			for _, tbl := range tables {
				if _, dropErr := db.ExecContext(context.Background(), "DROP TABLE IF EXISTS "+tbl+" CASCADE"); dropErr != nil {
					return fmt.Errorf("failed to drop table %s: %w", tbl, dropErr)
				}
			}
			fmt.Println("Dropped all PostgreSQL noctifab tables.")

		} else {
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
		}

		// Remove PID file if present.
		if _, err := os.Stat(daemonPIDFile); err == nil {
			if err := usecase.RemovePIDFile(daemonPIDFile); err != nil {
				fmt.Fprintf(os.Stderr, "⚠ Could not remove PID file: %v\n", err)
			} else {
				fmt.Println("Removed PID file.")
			}
		} else if !os.IsNotExist(err) {
			fmt.Fprintf(os.Stderr, "⚠ Could not access PID file: %v\n", err)
		}

		// Remove per-story log files.
		logDir := ".noctifab/logs/roadmap"
		if _, err := os.Stat(logDir); err == nil {
			if err := os.RemoveAll(logDir); err != nil {
				fmt.Fprintf(os.Stderr, "⚠ Could not remove story logs at %s: %v\n", logDir, err)
			} else {
				fmt.Printf("Removed story logs: %s\n", logDir)
			}
		} else if !os.IsNotExist(err) {
			fmt.Fprintf(os.Stderr, "⚠ Could not access story logs at %s: %v\n", logDir, err)
		}

		// Remove daemon log.
		if _, err := os.Stat(daemonLogFile); err == nil {
			if err := os.Remove(daemonLogFile); err != nil {
				fmt.Fprintf(os.Stderr, "⚠ Could not remove daemon log at %s: %v\n", daemonLogFile, err)
			} else {
				fmt.Printf("Removed daemon log: %s\n", daemonLogFile)
			}
		} else if !os.IsNotExist(err) {
			fmt.Fprintf(os.Stderr, "⚠ Could not access daemon log at %s: %v\n", daemonLogFile, err)
		}

		fmt.Println("✅ noctifab state cleared. Run 'noctifab init' and 'noctifab start' to begin fresh.")
		return nil
	},
}

func init() {
	cleanCmd.Flags().Bool("force", false, "Force clean even if the daemon is still running (state will be lost)")
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
	}
	for _, tbl := range tables {
		if !allowedTables[tbl] {
			return fmt.Errorf("failed to drop table: table name %q is not in the allowlist", tbl)
		}
	}
	return nil
}
