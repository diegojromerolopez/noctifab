package e2e

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/diegojromerolopez/noctifab/pkg/domain"
)

func simulatePlanningStep(ctx context.Context, repo domain.StateRepository, client *mockLLMClient, state *domain.State, tokenLimit int64, currentTokens *int64) (bool, error) {
	state.ActiveAgents[0].Status = domain.AgentWorking
	_ = repo.Save(ctx, state)

	resp, err := client.Complete(ctx, "plan tasks")
	if err != nil {
		return false, err
	}

	state.Metadata.TotalTokensUsed += 500
	*currentTokens += 500
	if tokenLimit > 0 && *currentTokens > tokenLimit {
		state.BuildStatus = domain.BuildFailing
		_ = repo.Save(ctx, state)
		return false, fmt.Errorf("%w: limit %d, used %d", domain.ErrBudgetExhausted, tokenLimit, *currentTokens)
	}

	for _, act := range resp.Actions {
		if act.Tool == "request_clarification" {
			q, _ := act.Args["question"].(string)
			state.Clarifications = append(state.Clarifications, domain.Clarification{
				Question: q,
				Resolved: false,
			})
			state.LastActions = append(state.LastActions, domain.Action{
				Timestamp: time.Now(),
				Tool:      "request_clarification",
				Args:      act.Args,
				Success:   true,
				Result:    "Requested clarification: " + q,
			})
			_ = repo.Save(ctx, state)
			return true, nil
		}
	}

	state.LastActions = append(state.LastActions, domain.Action{
		Timestamp: time.Now(),
		Tool:      "write_file",
		Args:      map[string]any{"path": ".noctifab/config/tasks.json"},
		Success:   true,
		Result:    "Plan generated successfully",
	})

	for _, act := range resp.Actions {
		if act.Tool == "add_task" {
			tID, _ := act.Args["id"].(string)
			tTitle, _ := act.Args["title"].(string)

			var deps []string
			if depSlice, ok := act.Args["depends_on"].([]any); ok {
				for _, d := range depSlice {
					if ds, ok := d.(string); ok {
						deps = append(deps, ds)
					}
				}
			}

			var targetFiles []string
			if tfSlice, ok := act.Args["target_files"].([]any); ok {
				for _, tf := range tfSlice {
					if tfs, ok := tf.(string); ok {
						targetFiles = append(targetFiles, tfs)
					}
				}
			}

			state.Tasks = append(state.Tasks, domain.Task{
				ID:          tID,
				Title:       tTitle,
				Status:      domain.TaskPending,
				DependsOn:   deps,
				TargetFiles: targetFiles,
			})
		}
	}

	if len(resp.Actions) > 0 && resp.Actions[0].Args != nil {
		if tasksRaw, ok := resp.Actions[0].Args["tasks"].([]any); ok {
			for _, tr := range tasksRaw {
				tm, ok := tr.(map[string]any)
				if !ok {
					continue
				}
				tID, _ := tm["id"].(string)
				tTitle, _ := tm["title"].(string)

				var deps []string
				if depSlice, ok := tm["depends_on"].([]any); ok {
					for _, d := range depSlice {
						if ds, ok := d.(string); ok {
							deps = append(deps, ds)
						}
					}
				}

				var targetFiles []string
				if tfSlice, ok := tm["target_files"].([]any); ok {
					for _, tf := range tfSlice {
						if tfs, ok := tf.(string); ok {
							targetFiles = append(targetFiles, tfs)
						}
					}
				}

				state.Tasks = append(state.Tasks, domain.Task{
					ID:          tID,
					Title:       tTitle,
					Status:      domain.TaskPending,
					DependsOn:   deps,
					TargetFiles: targetFiles,
				})
			}
		}
	}

	if _, err := resolveDependencies(state.Tasks); err != nil {
		state.LastActions = append(state.LastActions, domain.Action{
			Timestamp: time.Now(),
			Tool:      "validate_dag",
			Args:      map[string]any{"tasks_count": len(state.Tasks)},
			Success:   false,
			Result:    err.Error(),
		})
		state.BuildStatus = domain.BuildFailing
		_ = repo.Save(ctx, state)
		return false, err
	}

	state.ActiveAgents[0].Status = domain.AgentCompleted
	state.ActiveAgents[0].CompletedAt = time.Now()
	if err := repo.Save(ctx, state); err != nil {
		return false, err
	}
	return false, nil
}

func simulateTaskExecutionStep(ctx context.Context, repo domain.StateRepository, client *mockLLMClient, state *domain.State, readyTask *domain.Task, workspace string, tokenLimit int64, currentTokens *int64) error {
	if strings.Contains(state.Metadata.FeatureName, "compaction") {
		state.LastActions = append(state.LastActions, domain.Action{
			Timestamp: time.Now(),
			Tool:      "compact_history",
			Success:   true,
			Result:    "Compacted history successfully",
		})
	}

	if tokenLimit > 0 && *currentTokens+800 > tokenLimit {
		state.BuildStatus = domain.BuildFailing
		_ = repo.Save(ctx, state)
		return fmt.Errorf("%w: limit %d, used %d", domain.ErrBudgetExhausted, tokenLimit, *currentTokens)
	}

	state.ActiveAgents[1].Status = domain.AgentWorking
	readyTask.Status = domain.TaskInProgress
	_ = repo.Save(ctx, state)

	state.LastActions = append(state.LastActions, domain.Action{
		Timestamp: time.Now(),
		Tool:      "git_checkout",
		Args:      map[string]any{"branch": "noctifab/task-" + readyTask.ID + "-agent-generator"},
		Success:   true,
		Result:    "Switched to branch noctifab/task-" + readyTask.ID + "-agent-generator",
	})

	resp, err := client.Complete(ctx, "execute task "+readyTask.ID)
	if err != nil {
		return err
	}

	state.Metadata.TotalTokensUsed += 800
	*currentTokens += 800

	for _, act := range resp.Actions {
		if act.Tool == "write_file" {
			relPath := act.Args["path"].(string)
			content := act.Args["content"].(string)

			if strings.HasPrefix(relPath, "/") || strings.Contains(relPath, "..") {
				state.LastActions = append(state.LastActions, domain.Action{
					Timestamp: time.Now(),
					Tool:      "write_file",
					Args:      map[string]any{"path": relPath},
					Success:   false,
					Result:    "Sandbox violation: path '" + relPath + "' resolves outside the workspace boundary",
				})
				readyTask.Status = domain.TaskFailed
				state.ActiveAgents[1].Status = domain.AgentIdle
				_ = repo.Save(ctx, state)
				return fmt.Errorf("Sandbox violation: path resolves outside workspace")
			}

			fullPath := filepath.Join(workspace, relPath)
			if err := os.MkdirAll(filepath.Dir(fullPath), 0755); err != nil {
				return err
			}
			if err := os.WriteFile(fullPath, []byte(content), 0644); err != nil {
				return err
			}
			state.LastActions = append(state.LastActions, domain.Action{
				Timestamp: time.Now(),
				Tool:      "write_file",
				Args:      map[string]any{"path": relPath},
				Success:   true,
				Result:    "File written successfully",
			})
		}
	}

	passed := true
	var errorLog string
	if readyTask.ID == "task-views" {
		_, errViews := os.Stat(filepath.Join(workspace, "contacts/views.py"))
		_, errTemp := os.Stat(filepath.Join(workspace, "contacts/templates/contacts/contact_list.html"))
		if errViews != nil || errTemp != nil {
			passed = false
			errorLog = "missing HTML template contact_list.html"
		}
	}
	if strings.Contains(state.Metadata.FeatureName, "pruning") && readyTask.ID == "task-a" {
		passed = false
		errorLog = "Task A failed validation continuously"
	}
	if strings.Contains(state.Metadata.FeatureName, "rollback") && readyTask.Retries == 0 {
		passed = false
		errorLog = "Build breakage: syntax error in main.go"
	}

	if readyTask.ID == "task-setup" && passed {
		state.LastActions = append(state.LastActions, domain.Action{
			Timestamp: time.Now(),
			Tool:      "validate_task_run_1",
			Success:   false,
			Result:    "flaky mock error on run 1",
		})
		state.LastActions = append(state.LastActions, domain.Action{
			Timestamp: time.Now(),
			Tool:      "validate_task_majority_vote",
			Success:   true,
			Result:    "Warning: Potentially Flaky Build",
		})
	}

	if passed {
		if strings.Contains(state.Metadata.FeatureName, "conflict") && readyTask.ID == "task-agent-2" {
			readyTask.Status = domain.TaskConflictBlocked
			state.LastActions = append(state.LastActions, domain.Action{
				Timestamp: time.Now(),
				Tool:      "git_merge",
				Args:      map[string]any{"branch": "noctifab/task-" + readyTask.ID + "-agent-generator"},
				Success:   false,
				Result:    "Merge Conflict: conflict detected on common.py",
			})

			resolverAgent := domain.Agent{
				ID:        "agent-resolver",
				Name:      "Conflict Resolver",
				Role:      domain.AgentRoleResolver,
				Status:    domain.AgentWorking,
				TaskID:    readyTask.ID,
				StartedAt: time.Now(),
			}
			for j := range state.ActiveAgents {
				if state.ActiveAgents[j].ID == "agent-generator" {
					state.ActiveAgents[j].Status = domain.AgentIdle
				}
			}
			state.ActiveAgents = append(state.ActiveAgents, resolverAgent)

			if err := repo.Save(ctx, state); err != nil {
				return err
			}

			resp, err := client.Complete(ctx, "resolve git conflict on common.py")
			if err != nil {
				return err
			}

			state.Metadata.TotalTokensUsed += 800

			for _, act := range resp.Actions {
				if act.Tool == "write_file" {
					relPath := act.Args["path"].(string)
					content := act.Args["content"].(string)
					fullPath := filepath.Join(workspace, relPath)
					if err := os.MkdirAll(filepath.Dir(fullPath), 0755); err != nil {
						return err
					}
					if err := os.WriteFile(fullPath, []byte(content), 0644); err != nil {
						return err
					}
					state.LastActions = append(state.LastActions, domain.Action{
						Timestamp: time.Now(),
						Tool:      "write_file",
						Args:      map[string]any{"path": relPath},
						Success:   true,
						Result:    "File written successfully",
					})
				}
			}

			readyTask.Status = domain.TaskSuccess
			readyTask.UpdatedAt = time.Now()

			state.LastActions = append(state.LastActions, domain.Action{
				Timestamp: time.Now(),
				Tool:      "resolve_conflict",
				Args:      map[string]any{"file": "common.py"},
				Success:   true,
				Result:    "Conflict resolved successfully via Resolver agent",
			})

			for j := range state.ActiveAgents {
				if state.ActiveAgents[j].ID == "agent-resolver" {
					state.ActiveAgents[j].Status = domain.AgentCompleted
					state.ActiveAgents[j].CompletedAt = time.Now()
				}
			}

			return repo.Save(ctx, state)
		}

		readyTask.Status = domain.TaskSuccess
		state.LastActions = append(state.LastActions, domain.Action{
			Timestamp: time.Now(),
			Tool:      "git_commit",
			Args:      map[string]any{"message": "feat: " + readyTask.Title},
			Success:   true,
			Result:    "Committed changes successfully",
		})
	} else {
		readyTask.Status = domain.TaskFailed
		state.LastActions = append(state.LastActions, domain.Action{
			Timestamp: time.Now(),
			Tool:      "validate_task",
			Success:   false,
			Result:    errorLog,
		})

		if strings.Contains(state.Metadata.FeatureName, "rollback") || readyTask.ID == "task-views" {
			if strings.Contains(state.Metadata.FeatureName, "rollback") {
				state.LastActions = append(state.LastActions, domain.Action{
					Timestamp: time.Now(),
					Tool:      "git_reset_hard",
					Args:      map[string]any{"commit": "HEAD"},
					Success:   true,
					Result:    "Rolled back changes due to build failure",
				})
			}
			readyTask.Retries++
			readyTask.MaxRetries = 2
			if readyTask.Retries < readyTask.MaxRetries {
				readyTask.Status = domain.TaskPending
			}
		} else {
			var prune func(failedID string)
			prune = func(failedID string) {
				for i := range state.Tasks {
					t := &state.Tasks[i]
					if t.Status == domain.TaskPending {
						for _, dep := range t.DependsOn {
							if dep == failedID {
								t.Status = domain.TaskConflictFailed
								prune(t.ID)
								break
							}
						}
					}
				}
			}
			prune(readyTask.ID)
		}
	}

	readyTask.UpdatedAt = time.Now()
	state.ActiveAgents[1].Status = domain.AgentIdle
	return repo.Save(ctx, state)
}

func simulateFinalValidationStep(ctx context.Context, repo domain.StateRepository, state *domain.State, workspace string) error {
	state.ActiveAgents[2].Status = domain.AgentWorking
	_ = repo.Save(ctx, state)

	reqFiles := []string{
		"manage.py",
		"notebook/settings.py",
		"contacts/models.py",
		"contacts/views.py",
		"contacts/templates/contacts/contact_list.html",
	}
	if strings.Contains(state.Metadata.FeatureName, "conflict") {
		reqFiles = []string{"common.py"}
	}
	if strings.Contains(state.Metadata.FeatureName, "Flaky") {
		reqFiles = []string{"manage.py"}
	}
	if strings.Contains(state.Metadata.FeatureName, "rollback") ||
		strings.Contains(state.Metadata.FeatureName, "Refactor") ||
		strings.Contains(state.Metadata.FeatureName, "shutdown") {
		reqFiles = []string{"main.go"}
	}
	if strings.Contains(state.Metadata.FeatureName, "migration") {
		reqFiles = []string{"migrations/0001_add_age.sql", "models.py"}
	}
	if strings.Contains(state.Metadata.FeatureName, "compaction") {
		reqFiles = []string{}
	}
	passed := true
	for _, rf := range reqFiles {
		if _, err := os.Stat(filepath.Join(workspace, rf)); err != nil {
			passed = false
			break
		}
	}

	state.ValidationCriteria = []domain.ValidationCriterion{
		{
			ID:          "val-django-files",
			Type:        domain.ValidationFileExists,
			Expression:  strings.Join(reqFiles, ","),
			Description: "Check that all Django CRUD files exist",
			Passed:      passed,
		},
	}

	if passed {
		state.BuildStatus = domain.BuildPassing

		state.Metadata.ProjectVersion = "0.1.1"
		versionPath := filepath.Join(workspace, "VERSION")
		_ = os.WriteFile(versionPath, []byte("0.1.1"), 0644)

		changelogPath := filepath.Join(workspace, "CHANGELOG.md")
		changelogContent := "## [0.1.1] - 2026-06-20\n- Added Django contact CRUD notebook\n"
		if strings.Contains(state.Metadata.FeatureName, "conflict") {
			changelogContent = "## [0.1.1] - 2026-06-20\n- Resolved conflict on common.py\n"
		}
		_ = os.WriteFile(changelogPath, []byte(changelogContent), 0644)

		state.LastActions = append(state.LastActions, domain.Action{
			Timestamp: time.Now(),
			Tool:      "git_commit",
			Args:      map[string]any{"message": "chore: bump version to 0.1.1 and update CHANGELOG.md"},
			Success:   true,
			Result:    "Committed release details successfully",
		})

		state.LastActions = append(state.LastActions, domain.Action{
			Timestamp: time.Now(),
			Tool:      "create_pr",
			Args:      map[string]any{"title": "feat: Django Contact CRUD Notebook"},
			Success:   true,
			Result:    "VCS PR #42 opened successfully",
		})
	} else {
		state.BuildStatus = domain.BuildFailing
	}

	state.ActiveAgents[2].Status = domain.AgentCompleted
	state.LastActions = append(state.LastActions, domain.Action{
		Timestamp: time.Now(),
		Tool:      "validate",
		Success:   passed,
		Result:    "Validation complete",
	})
	return repo.Save(ctx, state)
}
