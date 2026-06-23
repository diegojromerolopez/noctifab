package storage

import (
	"context"
	"database/sql"
	"embed"
	"fmt"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

//go:embed migrations/sqlite/*.sql migrations/postgres/*.sql
var migrationFS embed.FS

// Migrate runs migrations for the given dbType ("sqlite" or "postgres") on the *sql.DB connection.
func Migrate(ctx context.Context, db *sql.DB, dbType string) error {
	// 1. Ensure migrations table exists
	var createTableQuery string
	switch dbType {
	case "sqlite":
		createTableQuery = `CREATE TABLE IF NOT EXISTS schema_migrations (
			version INTEGER PRIMARY KEY,
			applied_at DATETIME DEFAULT CURRENT_TIMESTAMP
		);`
	case "postgres":
		createTableQuery = `CREATE TABLE IF NOT EXISTS schema_migrations (
			version INT PRIMARY KEY,
			applied_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
		);`
	default:
		return fmt.Errorf("unsupported database type: %s", dbType)
	}

	if _, err := db.ExecContext(ctx, createTableQuery); err != nil {
		return fmt.Errorf("failed to create schema_migrations table: %w", err)
	}

	// 2. Read migration files
	dirPath := filepath.Join("migrations", dbType)
	entries, err := migrationFS.ReadDir(dirPath)
	if err != nil {
		return fmt.Errorf("failed to read migration directory %s: %w", dirPath, err)
	}

	type migrationFile struct {
		version int
		name    string
		content string
	}
	var migrations []migrationFile

	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".sql") {
			continue
		}
		parts := strings.SplitN(entry.Name(), "_", 2)
		if len(parts) < 2 {
			continue
		}
		version, err := strconv.Atoi(parts[0])
		if err != nil {
			return fmt.Errorf("invalid migration filename prefix: %s", entry.Name())
		}
		content, err := migrationFS.ReadFile(filepath.Join(dirPath, entry.Name()))
		if err != nil {
			return fmt.Errorf("failed to read migration file %s: %w", entry.Name(), err)
		}
		migrations = append(migrations, migrationFile{
			version: version,
			name:    entry.Name(),
			content: string(content),
		})
	}

	sort.Slice(migrations, func(i, j int) bool {
		return migrations[i].version < migrations[j].version
	})

	// 3. Apply missing migrations in a transaction
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("failed to start migration transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	applied := make(map[int]bool)
	rows, err := tx.QueryContext(ctx, "SELECT version FROM schema_migrations")
	if err != nil {
		return fmt.Errorf("failed to query applied migrations: %w", err)
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var v int
		if err := rows.Scan(&v); err != nil {
			return fmt.Errorf("failed to scan migration version: %w", err)
		}
		applied[v] = true
	}

	for _, m := range migrations {
		if applied[m.version] {
			continue
		}

		if _, err := tx.ExecContext(ctx, m.content); err != nil {
			return fmt.Errorf("failed executing migration %s: %w", m.name, err)
		}

		var insertQuery string
		if dbType == "sqlite" {
			insertQuery = "INSERT INTO schema_migrations (version) VALUES (?)"
		} else {
			insertQuery = "INSERT INTO schema_migrations (version) VALUES ($1)"
		}
		if _, err := tx.ExecContext(ctx, insertQuery, m.version); err != nil {
			return fmt.Errorf("failed recording migration %s: %w", m.name, err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit migration transaction: %w", err)
	}

	return nil
}
