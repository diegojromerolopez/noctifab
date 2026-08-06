package storage

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/diegojromerolopez/noctifab/pkg/domain"
	"github.com/diegojromerolopez/noctifab/pkg/infrastructure/telemetry"
)

// PruneFinishedStates deletes terminal (SUCCESS/FAILED) states beyond the
// most recent keepLast (ordered by their latest task activity, newest
// first), cascading deletion of all their relation rows. It returns the
// number of pruned states.
func (r *PostgresRepository) PruneFinishedStates(ctx context.Context, keepLast int) (int, error) {
	ctx, span := telemetry.Tracer().Start(ctx, "PruneFinishedStates")
	defer span.End()

	if keepLast < 0 {
		keepLast = 0
	}

	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer func() { _ = tx.Rollback() }()

	rows, err := tx.QueryContext(ctx,
		`SELECT id FROM state
		WHERE story_status IN ($1, $2)
		ORDER BY (SELECT MAX(updated_at) FROM tasks WHERE tasks.state_id = state.id) DESC NULLS LAST, id DESC`,
		string(domain.StorySuccess), string(domain.StoryFailed))
	if err != nil {
		return 0, err
	}

	var terminalIDs []string
	err = func() error {
		defer func() { _ = rows.Close() }()
		for rows.Next() {
			var id string
			if err := rows.Scan(&id); err != nil {
				return err
			}
			terminalIDs = append(terminalIDs, id)
		}
		return rows.Err()
	}()
	if err != nil {
		return 0, err
	}

	if len(terminalIDs) <= keepLast {
		return 0, tx.Commit()
	}
	pruneIDs := terminalIDs[keepLast:]

	placeholderList := make([]string, len(pruneIDs))
	args := make([]any, len(pruneIDs))
	for i, id := range pruneIDs {
		placeholderList[i] = fmt.Sprintf("$%d", i+1)
		args[i] = id
	}
	placeholders := strings.Join(placeholderList, ", ")

	for _, table := range stateRelationGroups {
		query := fmt.Sprintf("DELETE FROM %s WHERE state_id IN (%s)", table, placeholders) // #nosec G201 -- table names come from a fixed constant list
		if _, err := tx.ExecContext(ctx, query, args...); err != nil {
			return 0, err
		}
	}
	query := fmt.Sprintf("DELETE FROM state WHERE id IN (%s)", placeholders)
	if _, err := tx.ExecContext(ctx, query, args...); err != nil {
		return 0, err
	}

	if err := tx.Commit(); err != nil {
		return 0, err
	}

	for _, id := range pruneIDs {
		r.fingerprints.invalidate(id)
	}
	return len(pruneIDs), nil
}

// LoadAllSummaries returns lightweight per-story summaries (state row plus
// task status counts and timestamps) without loading actions, files, or
// task bodies.
func (r *PostgresRepository) LoadAllSummaries(ctx context.Context) ([]domain.StateSummary, error) {
	ctx, span := telemetry.Tracer().Start(ctx, "LoadAllSummaries")
	defer span.End()

	acc := newSummaryAccumulator()

	rows, err := r.db.QueryContext(ctx,
		`SELECT id, feature_name, input_path, integration_branch, base_branch, story_status, story_error, build_status, version
		FROM state ORDER BY CASE WHEN story_status = 'RUNNING' THEN 0 ELSE 1 END, id DESC`)
	if err != nil {
		return nil, err
	}
	err = func() error {
		defer func() { _ = rows.Close() }()
		for rows.Next() {
			var s domain.StateSummary
			if err := rows.Scan(&s.ID, &s.FeatureName, &s.InputPath, &s.IntegrationBranch,
				&s.BaseBranch, &s.StoryStatus, &s.StoryError, &s.BuildStatus, &s.Version); err != nil {
				return err
			}
			acc.addState(s)
		}
		return rows.Err()
	}()
	if err != nil {
		return nil, err
	}

	taskRows, err := r.db.QueryContext(ctx,
		`SELECT state_id, status, created_at, updated_at FROM tasks`)
	if err != nil {
		return nil, err
	}
	err = func() error {
		defer func() { _ = taskRows.Close() }()
		for taskRows.Next() {
			var stateID, status string
			var createdAt, updatedAt time.Time
			if err := taskRows.Scan(&stateID, &status, &createdAt, &updatedAt); err != nil {
				return err
			}
			acc.addTask(stateID, status, createdAt, updatedAt)
		}
		return taskRows.Err()
	}()
	if err != nil {
		return nil, err
	}

	return acc.result(), nil
}
