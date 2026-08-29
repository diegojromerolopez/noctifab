package storage

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/diegojromerolopez/noctifab/pkg/domain"
	"github.com/diegojromerolopez/noctifab/pkg/infrastructure/telemetry"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
	_ "modernc.org/sqlite"
)

type SQLiteRepository struct {
	db         *sql.DB
	writeMutex sync.Mutex
	// fingerprints caches the per-group content hashes last committed for
	// each state ID so Save can skip rewriting unchanged relation groups.
	fingerprints fingerprintCache
}

var _ domain.StateRepository = (*SQLiteRepository)(nil)

// NewSQLiteRepository creates, initializes and runs migrations on a SQLite database.
func NewSQLiteRepository(ctx context.Context, dsn string) (*SQLiteRepository, error) {
	// Ensure parent directory exists for file-based sqlite databases
	if dsn != ":memory:" && !strings.HasPrefix(dsn, "file::memory:") && !strings.Contains(dsn, "mode=memory") {
		dir := filepath.Dir(dsn)
		if dir != "." && dir != "/" {
			if err := os.MkdirAll(dir, 0755); err != nil {
				return nil, fmt.Errorf("failed to create sqlite parent directory %q: %w", dir, err)
			}
		}
	}

	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("failed to open sqlite database: %w", err)
	}

	// Serialized writes by using MaxOpenConns = 1
	db.SetMaxOpenConns(1)

	// Set WAL journal mode and busy timeout via PRAGMA queries
	if _, err := db.ExecContext(ctx, "PRAGMA journal_mode = WAL;"); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("failed to set SQLite journal_mode pragma: %w", err)
	}
	if _, err := db.ExecContext(ctx, "PRAGMA busy_timeout = 5000;"); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("failed to set SQLite busy_timeout pragma: %w", err)
	}

	if err := Migrate(ctx, db, "sqlite"); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("migration failure: %w", err)
	}

	return &SQLiteRepository{db: db}, nil
}

// Close closes the database connection.
func (r *SQLiteRepository) Close() error {
	return r.db.Close()
}

// DB returns the underlying sql.DB instance.
func (r *SQLiteRepository) DB() *sql.DB {
	return r.db
}

// Save persists the domain State in a transaction using OCC. Relation
// groups whose content fingerprint is unchanged since the last committed
// save from this repository instance are skipped (no DELETE+INSERT).
func (r *SQLiteRepository) Save(ctx context.Context, state *domain.State) error {
	ctx, span := telemetry.Tracer().Start(ctx, "Save",
		trace.WithAttributes(
			attribute.String("state.id", state.ID),
			attribute.Int("state.version", state.Version),
			attribute.Int("task_count", len(state.Tasks)),
		))
	defer span.End()
	r.writeMutex.Lock()
	defer r.writeMutex.Unlock()

	freshFingerprints, err := computeStateFingerprints(state)
	if err != nil {
		return err
	}

	nextVersion, err := r.saveTx(ctx, state, freshFingerprints)
	if err != nil {
		// The transaction did not commit (or was rejected with a version
		// conflict): another writer may have changed rows, so the cached
		// fingerprints no longer reflect DB content.
		r.fingerprints.invalidate(state.ID)
		return err
	}

	// Hashes may only be updated AFTER the transaction commits successfully.
	r.fingerprints.set(state.ID, freshFingerprints)
	state.Version = nextVersion
	return nil
}

// saveTx runs the OCC-checked save transaction and returns the committed
// next version.
func (r *SQLiteRepository) saveTx(ctx context.Context, state *domain.State, freshFingerprints stateFingerprints) (int, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer func() { _ = tx.Rollback() }()

	// 1. Fetch current version to check for conflict
	var currentVersion int
	err = tx.QueryRowContext(ctx, "SELECT version FROM state WHERE id = ?", state.ID).Scan(&currentVersion)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			currentVersion = 0
		} else {
			return 0, err
		}
	}

	// 2. Perform optimistic concurrency version check
	if state.Version != currentVersion {
		return 0, domain.ErrVersionConflict
	}

	// 3. Increment version and save state updates
	nextVersion := state.Version + 1
	if currentVersion == 0 {
		_, err = tx.ExecContext(ctx,
			`INSERT INTO state (id, project_path, version, build_status, story_status, story_error, input_source, input_path, integration_branch, feature_name, base_branch, project_version, total_tokens_used)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			state.ID, state.ProjectPath, nextVersion, string(state.BuildStatus),
			string(state.StoryStatus), state.StoryError,
			state.Metadata.InputSource, state.Metadata.InputPath, state.Metadata.IntegrationBranch,
			state.Metadata.FeatureName, state.Metadata.BaseBranch, state.Metadata.ProjectVersion,
			state.Metadata.TotalTokensUsed,
		)
	} else {
		_, err = tx.ExecContext(ctx,
			`UPDATE state SET project_path = ?, version = ?, build_status = ?, story_status = ?, story_error = ?, input_source = ?, input_path = ?, integration_branch = ?, feature_name = ?, base_branch = ?, project_version = ?, total_tokens_used = ?
			WHERE id = ?`,
			state.ProjectPath, nextVersion, string(state.BuildStatus),
			string(state.StoryStatus), state.StoryError,
			state.Metadata.InputSource, state.Metadata.InputPath, state.Metadata.IntegrationBranch,
			state.Metadata.FeatureName, state.Metadata.BaseBranch, state.Metadata.ProjectVersion,
			state.Metadata.TotalTokensUsed,
			state.ID,
		)
	}
	if err != nil {
		return 0, err
	}

	// 4. Rewrite only the relation groups whose content changed since the
	// last committed save (dirty-group incremental save).
	cached := r.fingerprints.get(state.ID)
	for _, group := range stateRelationGroups {
		if isGroupClean(cached, freshFingerprints, group) {
			continue
		}
		if err := r.rewriteRelationGroup(ctx, tx, state, group); err != nil {
			return 0, err
		}
	}

	if err = tx.Commit(); err != nil {
		return 0, err
	}

	return nextVersion, nil
}

// Load retrieves the State domain object from SQLite.
func (r *SQLiteRepository) Load(ctx context.Context) (*domain.State, error) {
	ctx, span := telemetry.Tracer().Start(ctx, "Load")
	defer span.End()
	row := r.db.QueryRowContext(ctx,
		`SELECT id, project_path, version, build_status, story_status, story_error, input_source, input_path, integration_branch, feature_name, base_branch, project_version, total_tokens_used
		FROM state
		ORDER BY CASE WHEN story_status = 'RUNNING' THEN 0 ELSE 1 END, id DESC
		LIMIT 1`)

	state, err := r.scanStateRow(ctx, row)
	if err != nil {
		return nil, err
	}

	if err := r.loadStateRelations(ctx, state); err != nil {
		return nil, err
	}

	return state, nil
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

func nullTime(t time.Time) any {
	if t.IsZero() {
		return nil
	}
	return t
}

func nullTimePtr(t *time.Time) any {
	if t == nil || t.IsZero() {
		return nil
	}
	return *t
}
