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
		`SELECT id, project_path, version, build_status, story_status, story_error, input_source, input_path, integration_branch, feature_name, base_branch, project_version, total_input_tokens, total_output_tokens, total_tokens_used
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
		`SELECT id, project_path, version, build_status, story_status, story_error, input_source, input_path, integration_branch, feature_name, base_branch, project_version, total_input_tokens, total_output_tokens, total_tokens_used
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
		return rows.Err()
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
		&state.Metadata.TotalInputTokens, &state.Metadata.TotalOutputTokens, &state.Metadata.TotalTokensUsed,
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
		&state.Metadata.TotalInputTokens, &state.Metadata.TotalOutputTokens, &state.Metadata.TotalTokensUsed,
	)
	if err != nil {
		return nil, err
	}
	state.BuildStatus = domain.BuildStatus(buildStatusStr)
	state.StoryStatus = domain.StoryStatus(storyStatusStr)
	return &state, nil
}

// loadStateRelations loads all nested relationships (Stories, Tasks, Clarifications, Actions, Files, ValidationCriteria, ActiveAgents) for a given State.
func (r *SQLiteRepository) loadStateRelations(ctx context.Context, state *domain.State) error {
	// Load Stories
	rowsSt, err := r.db.QueryContext(ctx,
		`SELECT id, state_id, title, file_path, status, started_at, completed_at, input_tokens, output_tokens, tokens_used, created_at, updated_at
		FROM stories WHERE state_id = ? ORDER BY id ASC`, state.ID)
	if err != nil {
		return err
	}
	defer func() { _ = rowsSt.Close() }()
	state.Stories = []domain.Story{}
	for rowsSt.Next() {
		var story domain.Story
		var statusStr string
		var startedAt, completedAt sql.NullTime
		if err := rowsSt.Scan(
			&story.ID, &story.StateID, &story.Title, &story.FilePath, &statusStr,
			&startedAt, &completedAt, &story.InputTokens, &story.OutputTokens, &story.TokensUsed, &story.CreatedAt, &story.UpdatedAt,
		); err != nil {
			return err
		}
		story.Status = domain.StoryStatus(statusStr)
		if startedAt.Valid {
			story.StartedAt = &startedAt.Time
		}
		if completedAt.Valid {
			story.CompletedAt = &completedAt.Time
		}
		state.Stories = append(state.Stories, story)
	}
	if err := rowsSt.Err(); err != nil {
		return err
	}

	// Load Tasks
	rows, err := r.db.QueryContext(ctx,
		`SELECT id, title, description, status, change_type, assigned_to, progress, depends_on, target_files, partial_changelog, retries, max_retries, failure_log, created_at, updated_at, COALESCE(story_id, ''), started_at, completed_at, input_tokens, output_tokens, tokens_used
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
		var storyID string
		var taskStartedAt, taskCompletedAt sql.NullTime
		err := rows.Scan(
			&task.ID, &task.Title, &task.Description, &statusStr, &changeTypeStr, &task.AssignedTo,
			&task.Progress, &dependsOnStr, &targetFilesStr, &partialChangelogStr, &task.Retries, &task.MaxRetries,
			&failureLogNull, &task.CreatedAt, &task.UpdatedAt, &storyID, &taskStartedAt, &taskCompletedAt,
			&task.InputTokens, &task.OutputTokens, &task.TokensUsed,
		)
		if err != nil {
			return err
		}
		if failureLogNull.Valid {
			task.FailureLog = failureLogNull.String
		}
		task.StoryID = storyID
		if taskStartedAt.Valid {
			task.StartedAt = &taskStartedAt.Time
		}
		if taskCompletedAt.Valid {
			task.CompletedAt = &taskCompletedAt.Time
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
	if err := rows.Err(); err != nil {
		return err
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
	if err := rowsCl.Err(); err != nil {
		return err
	}

	// Load Actions (bounded to most recent MaxLastActions in chronological order)
	rowsAc, err := r.db.QueryContext(ctx,
		`SELECT action_id, timestamp, tool, args, reasoning, result, success
		FROM (
			SELECT id, action_id, timestamp, tool, args, reasoning, result, success
			FROM actions WHERE state_id = ?
			ORDER BY id DESC LIMIT ?
		) sub ORDER BY id ASC`, state.ID, domain.MaxLastActions)
	if err != nil {
		return err
	}
	defer func() { _ = rowsAc.Close() }()

	state.LastActions = []domain.Action{}
	for rowsAc.Next() {
		var act domain.Action
		var argsStr string
		var successInt int
		err := rowsAc.Scan(&act.ID, &act.Timestamp, &act.Tool, &argsStr, &act.Reasoning, &act.Result, &successInt)
		if err != nil {
			return err
		}
		act.Success = successInt != 0
		if err := json.Unmarshal([]byte(argsStr), &act.Args); err != nil {
			return err
		}
		state.LastActions = append(state.LastActions, act)
	}
	if err := rowsAc.Err(); err != nil {
		return err
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
	if err := rowsFi.Err(); err != nil {
		return err
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
	if err := rowsVc.Err(); err != nil {
		return err
	}

	// Load Active Agents
	rowsAa, err := r.db.QueryContext(ctx,
		`SELECT id, name, role, status, task_id, started_at, completed_at, input_tokens, output_tokens, tokens_used, last_error
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
			&startedAtNull, &completedAtNull, &agent.InputTokens, &agent.OutputTokens, &agent.TokensUsed, &agent.LastError,
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
	if err := rowsAa.Err(); err != nil {
		return err
	}

	return r.loadQAReviews(ctx, state)
}
