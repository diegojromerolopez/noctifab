package usecase

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/diegojromerolopez/noctifab/pkg/domain"
	"github.com/google/uuid"
)

// AddTaskTool implements the add_task tool.
type AddTaskTool struct{}

func (t *AddTaskTool) Name() string { return "add_task" }
func (t *AddTaskTool) Description() string {
	return "add_task adds a new task to the scheduling DAG. Arguments: id (optional, string), title (string), description (string), depends_on ([]string), change_type (string: FEATURE/FIX/BREAKING), max_retries (optional, int)."
}

func (t *AddTaskTool) Execute(ctx context.Context, state *domain.State, args map[string]any) (string, error) {
	title, ok := args["title"].(string)
	if !ok || title == "" {
		return "", errors.New("missing or invalid 'title' argument")
	}
	desc, ok := args["description"].(string)
	if !ok || desc == "" {
		return "", errors.New("missing or invalid 'description' argument")
	}

	changeTypeStr, _ := args["change_type"].(string)
	if changeTypeStr == "" {
		changeTypeStr = "FEATURE"
	}
	changeType := domain.ChangeType(changeTypeStr)

	var dependsOn []string
	if depRaw, ok := args["depends_on"]; ok {
		if depSlice, ok := depRaw.([]any); ok {
			for _, d := range depSlice {
				if s, ok := d.(string); ok {
					dependsOn = append(dependsOn, s)
				}
			}
		} else if depStringSlice, ok := depRaw.([]string); ok {
			dependsOn = depStringSlice
		}
	}

	id, _ := args["id"].(string)
	if id == "" {
		id = "task-" + uuid.New().String()[:8]
	}

	maxRetriesVal := 3
	if mrRaw, ok := args["max_retries"]; ok {
		switch v := mrRaw.(type) {
		case float64:
			maxRetriesVal = int(v)
		case int:
			maxRetriesVal = v
		}
	}

	// Append target files if present
	var targetFiles []string
	if tfRaw, ok := args["target_files"]; ok {
		if tfSlice, ok := tfRaw.([]any); ok {
			for _, f := range tfSlice {
				if s, ok := f.(string); ok {
					targetFiles = append(targetFiles, s)
				}
			}
		} else if tfStringSlice, ok := tfRaw.([]string); ok {
			targetFiles = tfStringSlice
		}
	}

	task := domain.Task{
		ID:          id,
		Title:       title,
		Description: desc,
		Status:      domain.TaskPending,
		ChangeType:  changeType,
		DependsOn:   dependsOn,
		TargetFiles: targetFiles,
		Retries:     0,
		MaxRetries:  maxRetriesVal,
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}

	state.Tasks = append(state.Tasks, task)
	return id, nil
}

// CompleteTaskTool implements the complete_task tool.
type CompleteTaskTool struct{}

func (t *CompleteTaskTool) Name() string { return "complete_task" }
func (t *CompleteTaskTool) Description() string {
	return "complete_task marks an in-progress task as SUCCESS. Arguments: id (string)."
}

func (t *CompleteTaskTool) Execute(ctx context.Context, state *domain.State, args map[string]any) (string, error) {
	id, ok := args["id"].(string)
	if !ok || id == "" {
		return "", errors.New("missing or invalid 'id' argument")
	}

	for i := range state.Tasks {
		if state.Tasks[i].ID == id {
			if state.Tasks[i].Status != domain.TaskInProgress {
				return "", fmt.Errorf("task %s is not in progress (current status: %s)", id, state.Tasks[i].Status)
			}
			state.Tasks[i].Status = domain.TaskSuccess
			state.Tasks[i].UpdatedAt = time.Now()
			return fmt.Sprintf("Task %s completed successfully", id), nil
		}
	}

	return "", domain.ErrTaskNotFound
}

// LogMessageTool implements the log_message tool.
type LogMessageTool struct{}

func (t *LogMessageTool) Name() string { return "log_message" }
func (t *LogMessageTool) Description() string {
	return "log_message appends a diagnostic or status message to the execution state trace. Arguments: message (string)."
}

func (t *LogMessageTool) Execute(ctx context.Context, state *domain.State, args map[string]any) (string, error) {
	msg, ok := args["message"].(string)
	if !ok {
		return "", errors.New("missing or invalid 'message' argument")
	}
	// Action log message is implicitly handled by the orchestrator recording the Action result.
	return msg, nil
}

// NoopTool implements the noop tool.
type NoopTool struct{}

func (t *NoopTool) Name() string { return "noop" }
func (t *NoopTool) Description() string {
	return "noop performs no operation. Arguments: none."
}

func (t *NoopTool) Execute(ctx context.Context, state *domain.State, args map[string]any) (string, error) {
	return "success", nil
}
