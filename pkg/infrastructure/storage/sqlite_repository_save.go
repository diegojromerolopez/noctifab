package storage

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/diegojromerolopez/noctifab/pkg/domain"
	"github.com/google/uuid"
)

// rewriteRelationGroup performs the full DELETE+INSERT rewrite of a single
// relation group within the save transaction.
func (r *SQLiteRepository) rewriteRelationGroup(ctx context.Context, tx *sql.Tx, state *domain.State, group string) error {
	switch group {
	case groupStories:
		return r.saveStories(ctx, tx, state)
	case groupTasks:
		return r.saveTasks(ctx, tx, state)
	case groupClarifications:
		return r.saveClarifications(ctx, tx, state)
	case groupActions:
		return r.saveActions(ctx, tx, state)
	case groupWorkspaceFiles:
		return r.saveWorkspaceFiles(ctx, tx, state)
	case groupValidationCriteria:
		return r.saveValidationCriteria(ctx, tx, state)
	case groupActiveAgents:
		return r.saveActiveAgents(ctx, tx, state)
	case groupQAReviews:
		return r.saveQAReviews(ctx, tx, state)
	default:
		return fmt.Errorf("unknown relation group: %s", group)
	}
}

func placeholders(n int) string {
	if n <= 0 {
		return ""
	}
	var sb strings.Builder
	for i := 0; i < n; i++ {
		if i > 0 {
			sb.WriteString(", ")
		}
		sb.WriteString("?")
	}
	return sb.String()
}

func (r *SQLiteRepository) saveStories(ctx context.Context, tx *sql.Tx, state *domain.State) error {
	if len(state.Stories) == 0 {
		_, err := tx.ExecContext(ctx, "DELETE FROM stories WHERE state_id = ?", state.ID)
		return err
	}
	storyIDs := make([]any, len(state.Stories)+1)
	storyIDs[0] = state.ID
	for i, story := range state.Stories {
		storyIDs[i+1] = story.ID
		_, err := tx.ExecContext(ctx,
			`INSERT INTO stories (id, state_id, title, file_path, status, started_at, completed_at, input_tokens, output_tokens, tokens_used, created_at, updated_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
			ON CONFLICT(id) DO UPDATE SET
				state_id = excluded.state_id,
				title = excluded.title,
				file_path = excluded.file_path,
				status = excluded.status,
				started_at = excluded.started_at,
				completed_at = excluded.completed_at,
				input_tokens = excluded.input_tokens,
				output_tokens = excluded.output_tokens,
				tokens_used = excluded.tokens_used,
				updated_at = excluded.updated_at`,
			story.ID, state.ID, story.Title, story.FilePath, string(story.Status),
			nullTimePtr(story.StartedAt), nullTimePtr(story.CompletedAt), story.InputTokens, story.OutputTokens, story.TokensUsed, story.CreatedAt, story.UpdatedAt,
		)
		if err != nil {
			return err
		}
	}
	query := fmt.Sprintf("DELETE FROM stories WHERE state_id = ? AND id NOT IN (%s)", placeholders(len(state.Stories)))
	_, err := tx.ExecContext(ctx, query, storyIDs...)
	return err
}

func (r *SQLiteRepository) saveTasks(ctx context.Context, tx *sql.Tx, state *domain.State) error {
	if len(state.Tasks) == 0 {
		_, err := tx.ExecContext(ctx, "DELETE FROM tasks WHERE state_id = ?", state.ID)
		return err
	}
	seen := make(map[string]bool, len(state.Tasks))
	taskIDs := make([]any, len(state.Tasks)+1)
	taskIDs[0] = state.ID
	for i, task := range state.Tasks {
		if seen[task.ID] {
			return fmt.Errorf("duplicate task ID in state: %s", task.ID)
		}
		seen[task.ID] = true
		taskIDs[i+1] = task.ID

		dependsOnJSON, err := json.Marshal(task.DependsOn)
		if err != nil {
			return err
		}
		targetFilesJSON, err := json.Marshal(task.TargetFiles)
		if err != nil {
			return err
		}
		partialChangelogJSON, err := json.Marshal(task.PartialChangelog)
		if err != nil {
			return err
		}

		_, err = tx.ExecContext(ctx,
			`INSERT INTO tasks (id, state_id, title, description, status, change_type, assigned_to, progress, depends_on, target_files, partial_changelog, retries, max_retries, failure_log, created_at, updated_at, story_id, started_at, completed_at, input_tokens, output_tokens, tokens_used)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
			ON CONFLICT(id) DO UPDATE SET
				state_id = excluded.state_id,
				title = excluded.title,
				description = excluded.description,
				status = excluded.status,
				change_type = excluded.change_type,
				assigned_to = excluded.assigned_to,
				progress = excluded.progress,
				depends_on = excluded.depends_on,
				target_files = excluded.target_files,
				partial_changelog = excluded.partial_changelog,
				retries = excluded.retries,
				max_retries = excluded.max_retries,
				failure_log = excluded.failure_log,
				updated_at = excluded.updated_at,
				story_id = excluded.story_id,
				started_at = excluded.started_at,
				completed_at = excluded.completed_at,
				input_tokens = excluded.input_tokens,
				output_tokens = excluded.output_tokens,
				tokens_used = excluded.tokens_used`,
			task.ID, state.ID, task.Title, task.Description, string(task.Status), string(task.ChangeType),
			task.AssignedTo, task.Progress, string(dependsOnJSON), string(targetFilesJSON), string(partialChangelogJSON),
			task.Retries, task.MaxRetries, task.FailureLog, task.CreatedAt, task.UpdatedAt,
			task.StoryID, nullTimePtr(task.StartedAt), nullTimePtr(task.CompletedAt), task.InputTokens, task.OutputTokens, task.TokensUsed,
		)
		if err != nil {
			return err
		}
	}
	query := fmt.Sprintf("DELETE FROM tasks WHERE state_id = ? AND id NOT IN (%s)", placeholders(len(state.Tasks)))
	_, err := tx.ExecContext(ctx, query, taskIDs...)
	return err
}

func (r *SQLiteRepository) saveClarifications(ctx context.Context, tx *sql.Tx, state *domain.State) error {
	if _, err := tx.ExecContext(ctx, "DELETE FROM clarifications WHERE state_id = ?", state.ID); err != nil {
		return err
	}
	for _, clar := range state.Clarifications {
		clarID := clar.ID
		if clarID == "" {
			clarID = uuid.New().String()
		}
		_, err := tx.ExecContext(ctx,
			`INSERT INTO clarifications (id, state_id, question, answer, resolved, asked_at)
			VALUES (?, ?, ?, ?, ?, ?)`,
			clarID, state.ID, clar.Question, clar.Answer, boolToInt(clar.Resolved), clar.AskedAt,
		)
		if err != nil {
			return err
		}
	}
	return nil
}

func (r *SQLiteRepository) saveActions(ctx context.Context, tx *sql.Tx, state *domain.State) error {
	if _, err := tx.ExecContext(ctx, "DELETE FROM actions WHERE state_id = ?", state.ID); err != nil {
		return err
	}
	for _, act := range state.LastActions {
		argsJSON, err := json.Marshal(act.Args)
		if err != nil {
			return err
		}
		_, err = tx.ExecContext(ctx,
			`INSERT INTO actions (state_id, timestamp, tool, args, reasoning, result, success)
			VALUES (?, ?, ?, ?, ?, ?, ?)`,
			state.ID, act.Timestamp, act.Tool, string(argsJSON), act.Reasoning, act.Result, boolToInt(act.Success),
		)
		if err != nil {
			return err
		}
	}
	return nil
}

func (r *SQLiteRepository) saveWorkspaceFiles(ctx context.Context, tx *sql.Tx, state *domain.State) error {
	if len(state.Files) == 0 {
		_, err := tx.ExecContext(ctx, "DELETE FROM workspace_files WHERE state_id = ?", state.ID)
		return err
	}
	filePaths := make([]any, len(state.Files)+1)
	filePaths[0] = state.ID
	for i, file := range state.Files {
		filePaths[i+1] = file.Path
		_, err := tx.ExecContext(ctx,
			`INSERT INTO workspace_files (path, state_id, size, last_modified)
			VALUES (?, ?, ?, ?)
			ON CONFLICT(path, state_id) DO UPDATE SET
				size = excluded.size,
				last_modified = excluded.last_modified`,
			file.Path, state.ID, file.Size, file.LastModified,
		)
		if err != nil {
			return err
		}
	}
	query := fmt.Sprintf("DELETE FROM workspace_files WHERE state_id = ? AND path NOT IN (%s)", placeholders(len(state.Files)))
	_, err := tx.ExecContext(ctx, query, filePaths...)
	return err
}

func (r *SQLiteRepository) saveValidationCriteria(ctx context.Context, tx *sql.Tx, state *domain.State) error {
	if len(state.ValidationCriteria) == 0 {
		_, err := tx.ExecContext(ctx, "DELETE FROM validation_criteria WHERE state_id = ?", state.ID)
		return err
	}
	critIDs := make([]any, len(state.ValidationCriteria)+1)
	critIDs[0] = state.ID
	for i, crit := range state.ValidationCriteria {
		critIDs[i+1] = crit.ID
		_, err := tx.ExecContext(ctx,
			`INSERT INTO validation_criteria (id, state_id, type, expression, description, passed, error_log)
			VALUES (?, ?, ?, ?, ?, ?, ?)
			ON CONFLICT(id) DO UPDATE SET
				state_id = excluded.state_id,
				type = excluded.type,
				expression = excluded.expression,
				description = excluded.description,
				passed = excluded.passed,
				error_log = excluded.error_log`,
			crit.ID, state.ID, string(crit.Type), crit.Expression, crit.Description, boolToInt(crit.Passed), crit.ErrorLog,
		)
		if err != nil {
			return err
		}
	}
	query := fmt.Sprintf("DELETE FROM validation_criteria WHERE state_id = ? AND id NOT IN (%s)", placeholders(len(state.ValidationCriteria)))
	_, err := tx.ExecContext(ctx, query, critIDs...)
	return err
}

func (r *SQLiteRepository) saveActiveAgents(ctx context.Context, tx *sql.Tx, state *domain.State) error {
	if len(state.ActiveAgents) == 0 {
		_, err := tx.ExecContext(ctx, "DELETE FROM active_agents WHERE state_id = ?", state.ID)
		return err
	}
	agentIDs := make([]any, len(state.ActiveAgents)+1)
	agentIDs[0] = state.ID
	for i, agent := range state.ActiveAgents {
		agentIDs[i+1] = agent.ID
		_, err := tx.ExecContext(ctx,
			`INSERT INTO active_agents (id, state_id, name, role, status, task_id, started_at, completed_at, input_tokens, output_tokens, tokens_used, last_error)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
			ON CONFLICT(id) DO UPDATE SET
				state_id = excluded.state_id,
				name = excluded.name,
				role = excluded.role,
				status = excluded.status,
				task_id = excluded.task_id,
				started_at = excluded.started_at,
				completed_at = excluded.completed_at,
				input_tokens = excluded.input_tokens,
				output_tokens = excluded.output_tokens,
				tokens_used = excluded.tokens_used,
				last_error = excluded.last_error`,
			agent.ID, state.ID, agent.Name, string(agent.Role), string(agent.Status), agent.TaskID,
			nullTime(agent.StartedAt), nullTime(agent.CompletedAt), agent.InputTokens, agent.OutputTokens, agent.TokensUsed, agent.LastError,
		)
		if err != nil {
			return err
		}
	}
	query := fmt.Sprintf("DELETE FROM active_agents WHERE state_id = ? AND id NOT IN (%s)", placeholders(len(state.ActiveAgents)))
	_, err := tx.ExecContext(ctx, query, agentIDs...)
	return err
}
