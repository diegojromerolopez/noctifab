package storage

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"

	"github.com/diegojromerolopez/noctifab/pkg/domain"
	"github.com/diegojromerolopez/noctifab/pkg/infrastructure/telemetry"
)

// LoadByID retrieves a specific State domain object from SQLite by its ID.
func (r *SQLiteRepository) LoadByID(ctx context.Context, id string) (*domain.State, error) {
	ctx, span := telemetry.Tracer().Start(ctx, "LoadByID")
	defer span.End()

	row := r.db.QueryRowContext(ctx,
		`SELECT id, project_path, version, build_status, story_status, story_error, input_source, input_path, integration_branch, feature_name, base_branch, project_version, total_tokens_used, total_cost_usd
		FROM state WHERE id = ?`, id)

	state, err := r.scanStateRow(ctx, row)
	if err != nil {
		return nil, err
	}

	if err := r.loadStateRelations(ctx, state); err != nil {
		return nil, err
	}

	return state, nil
}

// LoadAll retrieves all State domain objects from SQLite.
func (r *SQLiteRepository) LoadAll(ctx context.Context) ([]*domain.State, error) {
	ctx, span := telemetry.Tracer().Start(ctx, "LoadAll")
	defer span.End()

	rows, err := r.db.QueryContext(ctx,
		`SELECT id, project_path, version, build_status, story_status, story_error, input_source, input_path, integration_branch, feature_name, base_branch, project_version, total_tokens_used, total_cost_usd
		FROM state ORDER BY CASE WHEN story_status = 'RUNNING' THEN 0 ELSE 1 END, id DESC`)
	if err != nil {
		return nil, err
	}

	var states []*domain.State
	err = func() error {
		defer func() { _ = rows.Close() }()
		for rows.Next() {
			state, err := r.scanStateRows(ctx, rows)
			if err != nil {
				return err
			}
			states = append(states, state)
		}
		return nil
	}()
	if err != nil {
		return nil, err
	}

	for _, state := range states {
		if err := r.loadStateRelations(ctx, state); err != nil {
			return nil, err
		}
	}

	return states, nil
}

// scanStateRow scans a single query row into domain.State.
func (r *SQLiteRepository) scanStateRow(ctx context.Context, row *sql.Row) (*domain.State, error) {
	var state domain.State
	var buildStatusStr, storyStatusStr string
	err := row.Scan(
		&state.ID, &state.ProjectPath, &state.Version, &buildStatusStr,
		&storyStatusStr, &state.StoryError,
		&state.Metadata.InputSource, &state.Metadata.InputPath, &state.Metadata.IntegrationBranch,
		&state.Metadata.FeatureName, &state.Metadata.BaseBranch, &state.Metadata.ProjectVersion,
		&state.Metadata.TotalTokensUsed, &state.Metadata.TotalCostUSD,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, sql.ErrNoRows
		}
		return nil, err
	}
	state.BuildStatus = domain.BuildStatus(buildStatusStr)
	state.StoryStatus = domain.StoryStatus(storyStatusStr)
	return &state, nil
}

// scanStateRows scans the current row from sql.Rows into domain.State.
func (r *SQLiteRepository) scanStateRows(ctx context.Context, rows *sql.Rows) (*domain.State, error) {
	var state domain.State
	var buildStatusStr, storyStatusStr string
	err := rows.Scan(
		&state.ID, &state.ProjectPath, &state.Version, &buildStatusStr,
		&storyStatusStr, &state.StoryError,
		&state.Metadata.InputSource, &state.Metadata.InputPath, &state.Metadata.IntegrationBranch,
		&state.Metadata.FeatureName, &state.Metadata.BaseBranch, &state.Metadata.ProjectVersion,
		&state.Metadata.TotalTokensUsed, &state.Metadata.TotalCostUSD,
	)
	if err != nil {
		return nil, err
	}
	state.BuildStatus = domain.BuildStatus(buildStatusStr)
	state.StoryStatus = domain.StoryStatus(storyStatusStr)
	return &state, nil
}

// loadStateRelations loads all nested relationships (Tasks, Clarifications, Actions, Files, ValidationCriteria, ActiveAgents) for a given State.
func (r *SQLiteRepository) loadStateRelations(ctx context.Context, state *domain.State) error {
	// Load Tasks
	rows, err := r.db.QueryContext(ctx,
		`SELECT id, title, description, status, change_type, assigned_to, progress, depends_on, target_files, partial_changelog, retries, max_retries, failure_log, created_at, updated_at
		FROM tasks WHERE state_id = ?`, state.ID)
	if err != nil {
		return err
	}
	defer func() { _ = rows.Close() }()

	state.Tasks = []domain.Task{}
	for rows.Next() {
		var task domain.Task
		var statusStr, changeTypeStr, dependsOnStr, targetFilesStr, partialChangelogStr string
		var failureLogNull sql.NullString
		err := rows.Scan(
			&task.ID, &task.Title, &task.Description, &statusStr, &changeTypeStr, &task.AssignedTo,
			&task.Progress, &dependsOnStr, &targetFilesStr, &partialChangelogStr, &task.Retries, &task.MaxRetries,
			&failureLogNull, &task.CreatedAt, &task.UpdatedAt,
		)
		if err != nil {
			return err
		}
		if failureLogNull.Valid {
			task.FailureLog = failureLogNull.String
		}
		task.Status = domain.TaskStatus(statusStr)
		task.ChangeType = domain.ChangeType(changeTypeStr)

		if err := json.Unmarshal([]byte(dependsOnStr), &task.DependsOn); err != nil {
			return err
		}
		if targetFilesStr != "" {
			if err := json.Unmarshal([]byte(targetFilesStr), &task.TargetFiles); err != nil {
				return err
			}
		}
		if partialChangelogStr != "" {
			if err := json.Unmarshal([]byte(partialChangelogStr), &task.PartialChangelog); err != nil {
				return err
			}
		}
		state.Tasks = append(state.Tasks, task)
	}

	// Load Clarifications
	rowsCl, err := r.db.QueryContext(ctx,
		`SELECT question, answer, resolved, asked_at
		FROM clarifications WHERE state_id = ?`, state.ID)
	if err != nil {
		return err
	}
	defer func() { _ = rowsCl.Close() }()

	state.Clarifications = []domain.Clarification{}
	for rowsCl.Next() {
		var clar domain.Clarification
		var resolvedInt int
		err := rowsCl.Scan(&clar.Question, &clar.Answer, &resolvedInt, &clar.AskedAt)
		if err != nil {
			return err
		}
		clar.Resolved = resolvedInt != 0
		state.Clarifications = append(state.Clarifications, clar)
	}

	// Load Actions
	rowsAc, err := r.db.QueryContext(ctx,
		`SELECT timestamp, tool, args, reasoning, result, success
		FROM actions WHERE state_id = ?`, state.ID)
	if err != nil {
		return err
	}
	defer func() { _ = rowsAc.Close() }()

	state.LastActions = []domain.Action{}
	for rowsAc.Next() {
		var act domain.Action
		var argsStr string
		var successInt int
		err := rowsAc.Scan(&act.Timestamp, &act.Tool, &argsStr, &act.Reasoning, &act.Result, &successInt)
		if err != nil {
			return err
		}
		act.Success = successInt != 0
		if err := json.Unmarshal([]byte(argsStr), &act.Args); err != nil {
			return err
		}
		state.LastActions = append(state.LastActions, act)
	}

	// Load Files
	rowsFi, err := r.db.QueryContext(ctx,
		`SELECT path, size, last_modified
		FROM workspace_files WHERE state_id = ?`, state.ID)
	if err != nil {
		return err
	}
	defer func() { _ = rowsFi.Close() }()

	state.Files = []domain.FileInfo{}
	for rowsFi.Next() {
		var file domain.FileInfo
		err := rowsFi.Scan(&file.Path, &file.Size, &file.LastModified)
		if err != nil {
			return err
		}
		state.Files = append(state.Files, file)
	}

	// Load Validation Criteria
	rowsVc, err := r.db.QueryContext(ctx,
		`SELECT id, type, expression, description, passed, error_log
		FROM validation_criteria WHERE state_id = ?`, state.ID)
	if err != nil {
		return err
	}
	defer func() { _ = rowsVc.Close() }()

	state.ValidationCriteria = []domain.ValidationCriterion{}
	for rowsVc.Next() {
		var crit domain.ValidationCriterion
		var typeStr string
		var passedInt int
		err := rowsVc.Scan(&crit.ID, &typeStr, &crit.Expression, &crit.Description, &passedInt, &crit.ErrorLog)
		if err != nil {
			return err
		}
		crit.Type = domain.ValidationType(typeStr)
		crit.Passed = passedInt != 0
		state.ValidationCriteria = append(state.ValidationCriteria, crit)
	}

	// Load Active Agents
	rowsAa, err := r.db.QueryContext(ctx,
		`SELECT id, name, role, status, task_id, started_at, completed_at, tokens_used, last_error
		FROM active_agents WHERE state_id = ?`, state.ID)
	if err != nil {
		return err
	}
	defer func() { _ = rowsAa.Close() }()

	state.ActiveAgents = []domain.Agent{}
	for rowsAa.Next() {
		var agent domain.Agent
		var roleStr, statusStr string
		var startedAtNull, completedAtNull sql.NullTime
		err := rowsAa.Scan(
			&agent.ID, &agent.Name, &roleStr, &statusStr, &agent.TaskID,
			&startedAtNull, &completedAtNull, &agent.TokensUsed, &agent.LastError,
		)
		if err != nil {
			return err
		}
		agent.Role = domain.AgentRole(roleStr)
		agent.Status = domain.AgentStatus(statusStr)
		if startedAtNull.Valid {
			agent.StartedAt = startedAtNull.Time
		}
		if completedAtNull.Valid {
			agent.CompletedAt = completedAtNull.Time
		}
		state.ActiveAgents = append(state.ActiveAgents, agent)
	}

	return nil
}
