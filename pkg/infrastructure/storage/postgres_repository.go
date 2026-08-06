package storage

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/diegojromerolopez/noctifab/pkg/domain"
	"github.com/diegojromerolopez/noctifab/pkg/infrastructure/telemetry"
	_ "github.com/jackc/pgx/v5/stdlib"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

type PostgresRepository struct {
	db *sql.DB
	// fingerprints caches the per-group content hashes last committed for
	// each state ID so Save can skip rewriting unchanged relation groups.
	fingerprints fingerprintCache
}

var _ domain.StateRepository = (*PostgresRepository)(nil)

// NewPostgresRepository creates, initializes and runs migrations on a PostgreSQL database.
func NewPostgresRepository(ctx context.Context, dsn string, maxOpen, maxIdle int) (*PostgresRepository, error) {
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		return nil, fmt.Errorf("failed to open postgres database: %w", err)
	}

	db.SetMaxOpenConns(maxOpen)
	db.SetMaxIdleConns(maxIdle)
	// Recycle connections periodically so the pool never holds on to broken
	// or server-side-expired connections (pool hygiene).
	db.SetConnMaxLifetime(30 * time.Minute)
	db.SetConnMaxIdleTime(5 * time.Minute)

	if err := Migrate(ctx, db, "postgres"); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("migration failure: %w", err)
	}

	return &PostgresRepository{db: db}, nil
}

// Close closes the database connection.
func (r *PostgresRepository) Close() error {
	return r.db.Close()
}

// DB returns the underlying sql.DB instance.
func (r *PostgresRepository) DB() *sql.DB {
	return r.db
}

// Save persists the domain State in a transaction using SELECT FOR UPDATE
// row locking and OCC version checking. Relation groups whose content
// fingerprint is unchanged since the last committed save from this
// repository instance are skipped (no DELETE+INSERT).
func (r *PostgresRepository) Save(ctx context.Context, state *domain.State) error {
	ctx, span := telemetry.Tracer().Start(ctx, "Save",
		trace.WithAttributes(
			attribute.String("state.id", state.ID),
			attribute.Int("state.version", state.Version),
			attribute.Int("task_count", len(state.Tasks)),
		))
	defer span.End()

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
func (r *PostgresRepository) saveTx(ctx context.Context, state *domain.State, freshFingerprints stateFingerprints) (int, error) {
	tx, err := r.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelRepeatableRead})
	if err != nil {
		return 0, err
	}
	defer func() { _ = tx.Rollback() }()

	// 1. Fetch current version and lock the row
	var currentVersion int
	err = tx.QueryRowContext(ctx, "SELECT version FROM state WHERE id = $1 FOR UPDATE", state.ID).Scan(&currentVersion)
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
			`INSERT INTO state (id, project_path, version, build_status, story_status, story_error, input_source, input_path, integration_branch, feature_name, base_branch, project_version, total_tokens_used, total_cost_usd)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14)`,
			state.ID, state.ProjectPath, nextVersion, string(state.BuildStatus),
			string(state.StoryStatus), state.StoryError,
			state.Metadata.InputSource, state.Metadata.InputPath, state.Metadata.IntegrationBranch,
			state.Metadata.FeatureName, state.Metadata.BaseBranch, state.Metadata.ProjectVersion,
			state.Metadata.TotalTokensUsed, state.Metadata.TotalCostUSD,
		)
	} else {
		_, err = tx.ExecContext(ctx,
			`UPDATE state SET project_path = $1, version = $2, build_status = $3, story_status = $4, story_error = $5, input_source = $6, input_path = $7, integration_branch = $8, feature_name = $9, base_branch = $10, project_version = $11, total_tokens_used = $12, total_cost_usd = $13
			WHERE id = $14`,
			state.ProjectPath, nextVersion, string(state.BuildStatus),
			string(state.StoryStatus), state.StoryError,
			state.Metadata.InputSource, state.Metadata.InputPath, state.Metadata.IntegrationBranch,
			state.Metadata.FeatureName, state.Metadata.BaseBranch, state.Metadata.ProjectVersion,
			state.Metadata.TotalTokensUsed, state.Metadata.TotalCostUSD,
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

// Load retrieves the State domain object from PostgreSQL.
func (r *PostgresRepository) Load(ctx context.Context) (*domain.State, error) {
	ctx, span := telemetry.Tracer().Start(ctx, "Load")
	defer span.End()
	row := r.db.QueryRowContext(ctx,
		`SELECT id, project_path, version, build_status, story_status, story_error, input_source, input_path, integration_branch, feature_name, base_branch, project_version, total_tokens_used, total_cost_usd
		FROM state
		ORDER BY CASE WHEN story_status = 'RUNNING' THEN 0 ELSE 1 END, id DESC
		LIMIT 1`)

	state, err := r.scanPostgresStateRow(ctx, row)
	if err != nil {
		return nil, err
	}

	if err := r.loadPostgresStateRelations(ctx, state); err != nil {
		return nil, err
	}

	return state, nil
}
