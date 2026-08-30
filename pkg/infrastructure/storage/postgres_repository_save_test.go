package storage

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/diegojromerolopez/noctifab/pkg/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// AnyTime is a mock helper to match any timestamp
type AnyTime struct{}

func (a AnyTime) Match(v driver.Value) bool {
	_, ok := v.(time.Time)
	return ok
}

func TestPostgresRepository_Save_Errors(t *testing.T) {
	ctx := context.Background()

	t.Run("when tx begin fails, it returns error", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		require.NoError(t, err)
		defer func() { _ = db.Close() }()

		repo := &PostgresRepository{db: db}
		mock.ExpectBegin().WillReturnError(errors.New("tx error"))

		err = repo.Save(ctx, &domain.State{ID: "state-1"})
		assert.ErrorContains(t, err, "tx error")
	})

	t.Run("when query version fails, it returns error", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		require.NoError(t, err)
		defer func() { _ = db.Close() }()

		repo := &PostgresRepository{db: db}
		mock.ExpectBegin()
		mock.ExpectQuery(`SELECT version FROM state WHERE id = \$1 FOR UPDATE`).
			WillReturnError(errors.New("query error"))
		mock.ExpectRollback()

		err = repo.Save(ctx, &domain.State{ID: "state-1"})
		assert.ErrorContains(t, err, "query error")
	})

	t.Run("when update state fails, it returns error", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		require.NoError(t, err)
		defer func() { _ = db.Close() }()

		repo := &PostgresRepository{db: db}
		mock.ExpectBegin()
		mock.ExpectQuery(`SELECT version FROM state WHERE id = \$1 FOR UPDATE`).
			WillReturnRows(sqlmock.NewRows([]string{"version"}).AddRow(1))
		mock.ExpectExec(`UPDATE state SET project_path = \$1`).
			WillReturnError(errors.New("exec error"))
		mock.ExpectRollback()

		err = repo.Save(ctx, &domain.State{ID: "state-1", Version: 1})
		assert.ErrorContains(t, err, "exec error")
	})

	t.Run("when delete tasks fails, it returns error", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		require.NoError(t, err)
		defer func() { _ = db.Close() }()

		repo := &PostgresRepository{db: db}
		mock.ExpectBegin()
		mock.ExpectQuery(`SELECT version FROM state WHERE id = \$1 FOR UPDATE`).
			WillReturnRows(sqlmock.NewRows([]string{"version"}).AddRow(1))
		mock.ExpectExec(`UPDATE state SET project_path = \$1`).
			WillReturnResult(sqlmock.NewResult(1, 1))
		mock.ExpectExec(`DELETE FROM tasks WHERE state_id = \$1`).
			WillReturnError(errors.New("delete error"))
		mock.ExpectRollback()

		err = repo.Save(ctx, &domain.State{ID: "state-1", Version: 1})
		assert.ErrorContains(t, err, "delete error")
	})

	t.Run("when insert tasks fails, it returns error", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		require.NoError(t, err)
		defer func() { _ = db.Close() }()

		repo := &PostgresRepository{db: db}
		mock.ExpectBegin()
		mock.ExpectQuery(`SELECT version FROM state WHERE id = \$1 FOR UPDATE`).
			WillReturnRows(sqlmock.NewRows([]string{"version"}).AddRow(1))
		mock.ExpectExec(`UPDATE state SET project_path = \$1`).
			WillReturnResult(sqlmock.NewResult(1, 1))
		mock.ExpectExec(`DELETE FROM tasks WHERE state_id = \$1`).
			WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectExec(`INSERT INTO tasks`).
			WillReturnError(errors.New("insert task error"))
		mock.ExpectRollback()

		state := &domain.State{
			ID:      "state-1",
			Version: 1,
			Tasks:   []domain.Task{{ID: "task-1"}},
		}
		err = repo.Save(ctx, state)
		assert.ErrorContains(t, err, "insert task error")
	})

	t.Run("when delete clarifications fails, it returns error", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		require.NoError(t, err)
		defer func() { _ = db.Close() }()

		repo := &PostgresRepository{db: db}
		mock.ExpectBegin()
		mock.ExpectQuery(`SELECT version FROM state WHERE id = \$1 FOR UPDATE`).
			WillReturnRows(sqlmock.NewRows([]string{"version"}).AddRow(1))
		mock.ExpectExec(`UPDATE state SET project_path = \$1`).
			WillReturnResult(sqlmock.NewResult(1, 1))
		mock.ExpectExec(`DELETE FROM tasks WHERE state_id = \$1`).
			WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectExec(`DELETE FROM clarifications WHERE state_id = \$1`).
			WillReturnError(errors.New("delete clar error"))
		mock.ExpectRollback()

		err = repo.Save(ctx, &domain.State{ID: "state-1", Version: 1})
		assert.ErrorContains(t, err, "delete clar error")
	})

	t.Run("when delete actions fails, it returns error", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		require.NoError(t, err)
		defer func() { _ = db.Close() }()

		repo := &PostgresRepository{db: db}
		mock.ExpectBegin()
		mock.ExpectQuery(`SELECT version FROM state WHERE id = \$1 FOR UPDATE`).
			WillReturnRows(sqlmock.NewRows([]string{"version"}).AddRow(1))
		mock.ExpectExec(`UPDATE state SET project_path = \$1`).
			WillReturnResult(sqlmock.NewResult(1, 1))
		mock.ExpectExec(`DELETE FROM tasks WHERE state_id = \$1`).
			WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectExec(`DELETE FROM clarifications WHERE state_id = \$1`).
			WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectExec(`DELETE FROM actions WHERE state_id = \$1`).
			WillReturnError(errors.New("delete action error"))
		mock.ExpectRollback()

		err = repo.Save(ctx, &domain.State{ID: "state-1", Version: 1})
		assert.ErrorContains(t, err, "delete action error")
	})

	t.Run("when delete workspace_files fails, it returns error", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		require.NoError(t, err)
		defer func() { _ = db.Close() }()

		repo := &PostgresRepository{db: db}
		mock.ExpectBegin()
		mock.ExpectQuery(`SELECT version FROM state WHERE id = \$1 FOR UPDATE`).
			WillReturnRows(sqlmock.NewRows([]string{"version"}).AddRow(1))
		mock.ExpectExec(`UPDATE state SET project_path = \$1`).
			WillReturnResult(sqlmock.NewResult(1, 1))
		mock.ExpectExec(`DELETE FROM tasks WHERE state_id = \$1`).
			WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectExec(`DELETE FROM clarifications WHERE state_id = \$1`).
			WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectExec(`DELETE FROM actions WHERE state_id = \$1`).
			WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectExec(`DELETE FROM workspace_files WHERE state_id = \$1`).
			WillReturnError(errors.New("delete files error"))
		mock.ExpectRollback()

		err = repo.Save(ctx, &domain.State{ID: "state-1", Version: 1})
		assert.ErrorContains(t, err, "delete files error")
	})

	t.Run("when delete validation_criteria fails, it returns error", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		require.NoError(t, err)
		defer func() { _ = db.Close() }()

		repo := &PostgresRepository{db: db}
		mock.ExpectBegin()
		mock.ExpectQuery(`SELECT version FROM state WHERE id = \$1 FOR UPDATE`).
			WillReturnRows(sqlmock.NewRows([]string{"version"}).AddRow(1))
		mock.ExpectExec(`UPDATE state SET project_path = \$1`).
			WillReturnResult(sqlmock.NewResult(1, 1))
		mock.ExpectExec(`DELETE FROM tasks WHERE state_id = \$1`).
			WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectExec(`DELETE FROM clarifications WHERE state_id = \$1`).
			WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectExec(`DELETE FROM actions WHERE state_id = \$1`).
			WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectExec(`DELETE FROM workspace_files WHERE state_id = \$1`).
			WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectExec(`DELETE FROM validation_criteria WHERE state_id = \$1`).
			WillReturnError(errors.New("delete criteria error"))
		mock.ExpectRollback()

		err = repo.Save(ctx, &domain.State{ID: "state-1", Version: 1})
		assert.ErrorContains(t, err, "delete criteria error")
	})

	t.Run("when delete active_agents fails, it returns error", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		require.NoError(t, err)
		defer func() { _ = db.Close() }()

		repo := &PostgresRepository{db: db}
		mock.ExpectBegin()
		mock.ExpectQuery(`SELECT version FROM state WHERE id = \$1 FOR UPDATE`).
			WillReturnRows(sqlmock.NewRows([]string{"version"}).AddRow(1))
		mock.ExpectExec(`UPDATE state SET project_path = \$1`).
			WillReturnResult(sqlmock.NewResult(1, 1))
		mock.ExpectExec(`DELETE FROM tasks WHERE state_id = \$1`).
			WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectExec(`DELETE FROM clarifications WHERE state_id = \$1`).
			WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectExec(`DELETE FROM actions WHERE state_id = \$1`).
			WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectExec(`DELETE FROM workspace_files WHERE state_id = \$1`).
			WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectExec(`DELETE FROM validation_criteria WHERE state_id = \$1`).
			WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectExec(`DELETE FROM active_agents WHERE state_id = \$1`).
			WillReturnError(errors.New("delete agents error"))
		mock.ExpectRollback()

		err = repo.Save(ctx, &domain.State{ID: "state-1", Version: 1})
		assert.ErrorContains(t, err, "delete agents error")
	})

	t.Run("when saving state fails on JSON marshal, it returns error", func(t *testing.T) {
		db, _, err := sqlmock.New()
		require.NoError(t, err)
		defer func() { _ = db.Close() }()

		repo := &PostgresRepository{db: db}

		state := &domain.State{
			ID:      "state-1",
			Version: 0,
			LastActions: []domain.Action{
				{
					Timestamp: time.Now(),
					Tool:      "test",
					Args:      map[string]any{"invalid": make(chan int)},
				},
			},
		}

		err = repo.Save(ctx, state)
		assert.Error(t, err)
	})
}

func TestPostgresRepository_Save(t *testing.T) {
	ctx := context.Background()

	t.Run("when saving new state successfully, it inserts version 1 and commits transaction", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		require.NoError(t, err)
		defer func() { _ = db.Close() }()

		repo := &PostgresRepository{db: db}
		now := time.Now()

		state := &domain.State{
			ID:          "state-1",
			ProjectPath: "/src",
			Version:     0,
			BuildStatus: domain.BuildPassing,
			Tasks: []domain.Task{
				{
					ID:          "task-1",
					Title:       "Task Title",
					Description: "Desc",
					Status:      domain.TaskPending,
					ChangeType:  domain.ChangeTypeFix,
					AssignedTo:  "agent-1",
					DependsOn:   []string{"task-0"},
					CreatedAt:   now,
					UpdatedAt:   now,
				},
			},
			Clarifications: []domain.Clarification{
				{
					Question: "Q?",
					Answer:   "A",
					Resolved: true,
					AskedAt:  now,
				},
			},
			LastActions: []domain.Action{
				{
					Timestamp: now,
					Tool:      "write",
					Args:      map[string]any{"x": 1.0},
					Reasoning: "reason",
					Result:    "ok",
					Success:   true,
				},
			},
			Files: []domain.FileInfo{
				{
					Path:         "main.go",
					Size:         100,
					LastModified: now,
				},
			},
			ValidationCriteria: []domain.ValidationCriterion{
				{
					ID:          "crit-1",
					Type:        domain.ValidationCommand,
					Expression:  "test",
					Description: "desc",
					Passed:      true,
					ErrorLog:    "",
				},
			},
			ActiveAgents: []domain.Agent{
				{
					ID:          "agent-1",
					Name:        "agent",
					Role:        domain.AgentRoleGenerator,
					Status:      domain.AgentWorking,
					TaskID:      "task-1",
					StartedAt:   now,
					CompletedAt: now,
					TokensUsed:  100,
					LastError:   "",
				},
			},
		}

		mock.ExpectBegin()
		mock.ExpectQuery(`SELECT version FROM state WHERE id = \$1 FOR UPDATE`).
			WithArgs(state.ID).
			WillReturnError(sql.ErrNoRows)

		mock.ExpectExec(`INSERT INTO state`).
			WithArgs(state.ID, state.ProjectPath, 1, string(state.BuildStatus),
				string(state.StoryStatus), state.StoryError,
				state.Metadata.InputSource, state.Metadata.InputPath, state.Metadata.IntegrationBranch,
				state.Metadata.FeatureName, state.Metadata.BaseBranch, state.Metadata.ProjectVersion,
				state.Metadata.TotalInputTokens, state.Metadata.TotalOutputTokens, state.Metadata.TotalTokensUsed).
			WillReturnResult(sqlmock.NewResult(1, 1))

		mock.ExpectExec(`DELETE FROM tasks WHERE state_id = \$1`).WithArgs(state.ID).WillReturnResult(sqlmock.NewResult(0, 0))
		dependsOnJSON, _ := json.Marshal(state.Tasks[0].DependsOn)
		targetFilesJSON, _ := json.Marshal(state.Tasks[0].TargetFiles)
		partialChangelogJSON, _ := json.Marshal(state.Tasks[0].PartialChangelog)
		mock.ExpectExec(`INSERT INTO tasks`).
			WithArgs(state.Tasks[0].ID, state.ID, state.Tasks[0].Title, state.Tasks[0].Description,
				string(state.Tasks[0].Status), string(state.Tasks[0].ChangeType), state.Tasks[0].AssignedTo,
				state.Tasks[0].Progress, dependsOnJSON, targetFilesJSON, partialChangelogJSON,
				state.Tasks[0].Retries, state.Tasks[0].MaxRetries, state.Tasks[0].FailureLog, state.Tasks[0].CreatedAt, state.Tasks[0].UpdatedAt,
				state.Tasks[0].StoryID, nullTimePtr(state.Tasks[0].StartedAt), nullTimePtr(state.Tasks[0].CompletedAt), state.Tasks[0].InputTokens, state.Tasks[0].OutputTokens, state.Tasks[0].TokensUsed).
			WillReturnResult(sqlmock.NewResult(1, 1))

		mock.ExpectExec(`DELETE FROM clarifications WHERE state_id = \$1`).WithArgs(state.ID).WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectExec(`INSERT INTO clarifications`).
			WithArgs(sqlmock.AnyArg(), state.ID, state.Clarifications[0].Question, state.Clarifications[0].Answer, 1, state.Clarifications[0].AskedAt).
			WillReturnResult(sqlmock.NewResult(1, 1))

		mock.ExpectExec(`DELETE FROM actions WHERE state_id = \$1`).WithArgs(state.ID).WillReturnResult(sqlmock.NewResult(0, 0))
		argsJSON, _ := json.Marshal(state.LastActions[0].Args)
		mock.ExpectExec(`INSERT INTO actions`).
			WithArgs(state.ID, state.LastActions[0].Timestamp, state.LastActions[0].Tool, argsJSON,
				state.LastActions[0].Reasoning, state.LastActions[0].Result, 1).
			WillReturnResult(sqlmock.NewResult(1, 1))

		mock.ExpectExec(`DELETE FROM workspace_files WHERE state_id = \$1`).WithArgs(state.ID).WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectExec(`INSERT INTO workspace_files`).
			WithArgs(state.Files[0].Path, state.ID, state.Files[0].Size, state.Files[0].LastModified).
			WillReturnResult(sqlmock.NewResult(1, 1))

		mock.ExpectExec(`DELETE FROM validation_criteria WHERE state_id = \$1`).WithArgs(state.ID).WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectExec(`INSERT INTO validation_criteria`).
			WithArgs(state.ValidationCriteria[0].ID, state.ID, string(state.ValidationCriteria[0].Type),
				state.ValidationCriteria[0].Expression, state.ValidationCriteria[0].Description, 1, state.ValidationCriteria[0].ErrorLog).
			WillReturnResult(sqlmock.NewResult(1, 1))

		mock.ExpectExec(`DELETE FROM active_agents WHERE state_id = \$1`).WithArgs(state.ID).WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectExec(`INSERT INTO active_agents`).
			WithArgs(state.ActiveAgents[0].ID, state.ID, state.ActiveAgents[0].Name, string(state.ActiveAgents[0].Role),
				string(state.ActiveAgents[0].Status), state.ActiveAgents[0].TaskID, state.ActiveAgents[0].StartedAt,
				state.ActiveAgents[0].CompletedAt, state.ActiveAgents[0].InputTokens, state.ActiveAgents[0].OutputTokens, state.ActiveAgents[0].TokensUsed, state.ActiveAgents[0].LastError).
			WillReturnResult(sqlmock.NewResult(1, 1))
		mock.ExpectExec(`DELETE FROM qa_findings WHERE state_id = \$1`).WithArgs(state.ID).WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectExec(`DELETE FROM qa_scenarios WHERE state_id = \$1`).WithArgs(state.ID).WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectExec(`DELETE FROM review_phases WHERE state_id = \$1`).WithArgs(state.ID).WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectExec(`DELETE FROM story_contracts WHERE state_id = \$1`).WithArgs(state.ID).WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectExec(`DELETE FROM stories WHERE state_id = \$1`).WithArgs(state.ID).WillReturnResult(sqlmock.NewResult(0, 0))

		mock.ExpectCommit()

		err = repo.Save(ctx, state)
		assert.NoError(t, err)
		assert.Equal(t, 1, state.Version)
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("when saving existing state successfully, it updates version and commits transaction", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		require.NoError(t, err)
		defer func() { _ = db.Close() }()

		repo := &PostgresRepository{db: db}

		state := &domain.State{
			ID:          "state-1",
			ProjectPath: "/src",
			Version:     1,
			BuildStatus: domain.BuildPassing,
		}

		mock.ExpectBegin()
		mock.ExpectQuery(`SELECT version FROM state WHERE id = \$1 FOR UPDATE`).
			WithArgs(state.ID).
			WillReturnRows(sqlmock.NewRows([]string{"version"}).AddRow(1))

		mock.ExpectExec(`UPDATE state SET project_path = \$1`).
			WithArgs(state.ProjectPath, 2, string(state.BuildStatus),
				string(state.StoryStatus), state.StoryError,
				state.Metadata.InputSource, state.Metadata.InputPath, state.Metadata.IntegrationBranch,
				state.Metadata.FeatureName, state.Metadata.BaseBranch, state.Metadata.ProjectVersion,
				state.Metadata.TotalInputTokens, state.Metadata.TotalOutputTokens, state.Metadata.TotalTokensUsed, state.ID).
			WillReturnResult(sqlmock.NewResult(1, 1))

		mock.ExpectExec(`DELETE FROM tasks WHERE state_id = \$1`).WithArgs(state.ID).WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectExec(`DELETE FROM clarifications WHERE state_id = \$1`).WithArgs(state.ID).WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectExec(`DELETE FROM actions WHERE state_id = \$1`).WithArgs(state.ID).WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectExec(`DELETE FROM workspace_files WHERE state_id = \$1`).WithArgs(state.ID).WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectExec(`DELETE FROM validation_criteria WHERE state_id = \$1`).WithArgs(state.ID).WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectExec(`DELETE FROM active_agents WHERE state_id = \$1`).WithArgs(state.ID).WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectExec(`DELETE FROM qa_findings WHERE state_id = \$1`).WithArgs(state.ID).WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectExec(`DELETE FROM qa_scenarios WHERE state_id = \$1`).WithArgs(state.ID).WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectExec(`DELETE FROM review_phases WHERE state_id = \$1`).WithArgs(state.ID).WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectExec(`DELETE FROM story_contracts WHERE state_id = \$1`).WithArgs(state.ID).WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectExec(`DELETE FROM stories WHERE state_id = \$1`).WithArgs(state.ID).WillReturnResult(sqlmock.NewResult(0, 0))

		mock.ExpectCommit()

		err = repo.Save(ctx, state)
		assert.NoError(t, err)
		assert.Equal(t, 2, state.Version)
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("when version conflict occurs, it rolls back and returns ErrVersionConflict", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		require.NoError(t, err)
		defer func() { _ = db.Close() }()

		repo := &PostgresRepository{db: db}

		state := &domain.State{
			ID:      "state-1",
			Version: 1,
		}

		mock.ExpectBegin()
		mock.ExpectQuery(`SELECT version FROM state WHERE id = \$1 FOR UPDATE`).
			WithArgs(state.ID).
			WillReturnRows(sqlmock.NewRows([]string{"version"}).AddRow(2))

		mock.ExpectRollback()

		err = repo.Save(ctx, state)
		assert.ErrorIs(t, err, domain.ErrVersionConflict)
		assert.Equal(t, 1, state.Version)
		assert.NoError(t, mock.ExpectationsWereMet())
	})
}
