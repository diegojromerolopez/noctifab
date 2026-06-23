package e2e

import (
	"context"
	"database/sql"

	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/diegojromerolopez/noctifab/pkg/domain"
	"github.com/diegojromerolopez/noctifab/pkg/infrastructure/storage"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/stretchr/testify/require"
)

func setupRepo(t *testing.T, ctx context.Context, tempDir, subDir, sessionID string) (domain.StateRepository, func()) {
	dbProvider := os.Getenv("NOCTIFAB_STORAGE_PROVIDER")
	if dbProvider == "postgres" {
		dsn := os.Getenv("NOCTIFAB_TEST_DB_DSN")
		if dsn == "" {
			dsn = "postgres://noctifab:noctifab_password@db:5432/noctifab_test?sslmode=disable"
		}
		repo, err := storage.NewPostgresRepository(ctx, dsn, 10, 10)
		require.NoError(t, err)

		// Clean up previous test state for isolation
		db, err := sql.Open("pgx", dsn)
		require.NoError(t, err)
		_, err = db.ExecContext(ctx, "TRUNCATE TABLE state CASCADE")
		require.NoError(t, err)
		_ = db.Close()

		return repo, func() { _ = repo.Close() }
	}

	// Fallback to SQLite
	dbDir := filepath.Join(tempDir, subDir, ".noctifab", "config")
	err := os.MkdirAll(dbDir, 0755)
	require.NoError(t, err)

	dbPath := filepath.Join(dbDir, "noctifab.db")
	repo, err := storage.NewSQLiteRepository(ctx, dbPath)
	require.NoError(t, err)
	return repo, func() { _ = repo.Close() }
}

func runSimulatedOrchestrator(ctx context.Context, repo domain.StateRepository, client *mockLLMClient, workspace string, maxBudgetUSD float64) error {
	const pricingRate = 0.000015

	state, err := repo.Load(context.Background())
	if err != nil {
		return err
	}

	// Requirements file verification
	reqPath := filepath.Join(workspace, state.Metadata.InputPath)
	if _, err := os.Stat(reqPath); err != nil {
		return fmt.Errorf("requirements file not found: %w", err)
	}

	state.ActiveAgents = []domain.Agent{
		{ID: "agent-planner", Name: "Planner", Role: domain.AgentRolePlanner, Status: domain.AgentIdle},
		{ID: "agent-generator", Name: "Generator", Role: domain.AgentRoleGenerator, Status: domain.AgentIdle},
		{ID: "agent-evaluator", Name: "Evaluator", Role: domain.AgentRoleEvaluator, Status: domain.AgentIdle},
	}

	// Recover interrupted tasks on startup
	for i := range state.Tasks {
		if state.Tasks[i].Status == domain.TaskInterrupted {
			state.Tasks[i].Status = domain.TaskPending
		}
	}

	if err := repo.Save(context.Background(), state); err != nil {
		return err
	}

	for cycle := 0; cycle < 15; cycle++ {
		// Graceful shutdown context check
		select {
		case <-ctx.Done():
			state, loadErr := repo.Load(context.Background())
			if loadErr == nil {
				// Mark any in-progress tasks as INTERRUPTED
				for i := range state.Tasks {
					if state.Tasks[i].Status == domain.TaskInProgress {
						state.Tasks[i].Status = domain.TaskInterrupted
					}
				}
				state.LastActions = append(state.LastActions, domain.Action{
					Timestamp: time.Now(),
					Tool:      "graceful_shutdown",
					Success:   true,
					Result:    "Daemon execution interrupted and saved state",
				})
				_ = repo.Save(context.Background(), state)
			}
			return ctx.Err()
		default:
		}

		state, err = repo.Load(ctx)
		if err != nil {
			return err
		}

		// Mock compaction check
		if strings.Contains(state.Metadata.FeatureName, "compaction") && len(state.LastActions) >= 1 {
			hasCompacted := false
			for _, act := range state.LastActions {
				if act.Tool == "compact_history" {
					hasCompacted = true
					break
				}
			}
			if !hasCompacted {
				state.LastActions = append(state.LastActions, domain.Action{
					Timestamp: time.Now(),
					Tool:      "compact_history",
					Success:   true,
					Result:    "Compacted history successfully, summarized 10 messages to 1",
				})
				if err := repo.Save(ctx, state); err != nil {
					return err
				}
			}
		}

		// Phase 1 Clarification Block
		hasUnresolved := false
		for _, c := range state.Clarifications {
			if !c.Resolved {
				hasUnresolved = true
				break
			}
		}
		if hasUnresolved {
			return nil
		}

		files, err := scanWorkspaceFiles(workspace)
		if err != nil {
			return err
		}
		state.Files = files

		// Pre-flight cost estimation & budget check
		var currentCost float64
		if state.Metadata.TotalCostUSD != "" {
			currentCost, _ = strconv.ParseFloat(state.Metadata.TotalCostUSD, 64)
		}

		if len(state.Tasks) == 0 {
			// Estimate cost for Planner call (1500 input + 1000 output)
			estCost := float64(2500) * pricingRate
			if currentCost+estCost > maxBudgetUSD {
				return domain.ErrBudgetExhausted
			}

			state.ActiveAgents[0].Status = domain.AgentWorking
			_ = repo.Save(ctx, state)

			resp, err := client.Complete(ctx, "plan django contact CRUD")
			if err != nil {
				return err
			}

			state.Metadata.TotalTokensUsed += 1500
			state.Metadata.TotalCostUSD = fmt.Sprintf("%.4f", currentCost+(float64(1500)*pricingRate))

			askedClarification := false
			for _, act := range resp.Actions {
				if act.Tool == "request_clarification" {
					c := domain.Clarification{
						Question: act.Args["question"].(string),
						Resolved: false,
						AskedAt:  time.Now(),
					}
					state.Clarifications = append(state.Clarifications, c)
					askedClarification = true
				}
			}

			if askedClarification {
				state.ActiveAgents[0].Status = domain.AgentIdle
				state.LastActions = append(state.LastActions, domain.Action{
					Timestamp: time.Now(),
					Tool:      "request_clarification",
					Success:   true,
					Result:    resp.Reasoning,
				})
				if err := repo.Save(ctx, state); err != nil {
					return err
				}
				return nil
			}

			for _, act := range resp.Actions {
				if act.Tool == "add_task" {
					var dependsOn []string
					if depRaw, ok := act.Args["depends_on"]; ok {
						if depSlice, ok := depRaw.([]any); ok {
							for _, d := range depSlice {
								dependsOn = append(dependsOn, d.(string))
							}
						}
					}
					tTask := domain.Task{
						ID:          act.Args["id"].(string),
						Title:       act.Args["title"].(string),
						Description: act.Args["description"].(string),
						Status:      domain.TaskPending,
						ChangeType:  domain.ChangeType(act.Args["change_type"].(string)),
						DependsOn:   dependsOn,
						CreatedAt:   time.Now(),
						UpdatedAt:   time.Now(),
					}
					state.Tasks = append(state.Tasks, tTask)
				}
			}

			// DAG cycle validation check
			if _, err := resolveDependencies(state.Tasks); err != nil {
				state.LastActions = append(state.LastActions, domain.Action{
					Timestamp: time.Now(),
					Tool:      "validate_dag",
					Success:   false,
					Result:    err.Error(),
				})
				_ = repo.Save(ctx, state)
				return err
			}

			state.ActiveAgents[0].Status = domain.AgentCompleted
			state.LastActions = append(state.LastActions, domain.Action{
				Timestamp: time.Now(),
				Tool:      "plan",
				Success:   true,
				Result:    resp.Reasoning,
			})
			if err := repo.Save(ctx, state); err != nil {
				return err
			}
			continue
		}

		// Downstream dependency pruning: if an upstream dependency has failed or is pruned,
		// mark downstream dependent tasks as CONFLICT_FAILED.
		pruned := false
		for i := range state.Tasks {
			if state.Tasks[i].Status == domain.TaskFailed || state.Tasks[i].Status == domain.TaskConflictFailed {
				for j := range state.Tasks {
					for _, depID := range state.Tasks[j].DependsOn {
						if (depID == state.Tasks[i].ID || depID == state.Tasks[i].Title) && state.Tasks[j].Status != domain.TaskConflictFailed {
							state.Tasks[j].Status = domain.TaskConflictFailed
							pruned = true
						}
					}
				}
			}
		}
		if pruned {
			if err := repo.Save(ctx, state); err != nil {
				return err
			}
		}

		var readyTask *domain.Task
		for i := range state.Tasks {
			if state.Tasks[i].Status == domain.TaskPending || state.Tasks[i].Status == domain.TaskFailed {
				depsMet := true
				for _, depID := range state.Tasks[i].DependsOn {
					for _, other := range state.Tasks {
						if (other.ID == depID || other.Title == depID) && other.Status != domain.TaskSuccess {
							depsMet = false
							break
						}
					}
				}
				if depsMet {
					readyTask = &state.Tasks[i]
					break
				}
			}
		}

		if readyTask != nil {
			// Estimate cost for Generator (800 input + 800 output)
			estCost := float64(1600) * pricingRate
			if currentCost+estCost > maxBudgetUSD {
				return domain.ErrBudgetExhausted
			}

			state.ActiveAgents[1].Status = domain.AgentWorking
			readyTask.Status = domain.TaskInProgress
			_ = repo.Save(ctx, state)

			// Switched to task branch sandbox
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
			state.Metadata.TotalCostUSD = fmt.Sprintf("%.4f", currentCost+(float64(800)*pricingRate))

			for _, act := range resp.Actions {
				if act.Tool == "write_file" {
					relPath := act.Args["path"].(string)
					content := act.Args["content"].(string)

					// Sandbox violation simulation: path resolves outside workspace boundary
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

			// Evaluator checks task
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

			// Simulate BDD Flaky Validation Quarantine (3x majority vote) for task-setup
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
				// Simulating Git Merge Conflict:
				// If the feature is a conflict test, and readyTask is task-agent-2,
				// simulate a merge conflict since task-agent-1 has already been integrated.
				if strings.Contains(state.Metadata.FeatureName, "conflict") && readyTask.ID == "task-agent-2" {
					readyTask.Status = domain.TaskConflictBlocked
					state.LastActions = append(state.LastActions, domain.Action{
						Timestamp: time.Now(),
						Tool:      "git_merge",
						Args:      map[string]any{"branch": "noctifab/task-" + readyTask.ID + "-agent-generator"},
						Success:   false,
						Result:    "Merge Conflict: conflict detected on common.py",
					})

					// Spawn a new agent-resolver agent (Name: "Conflict Resolver", Role: domain.AgentRoleResolver, Status: domain.AgentWorking)
					resolverAgent := domain.Agent{
						ID:        "agent-resolver",
						Name:      "Conflict Resolver",
						Role:      domain.AgentRoleResolver,
						Status:    domain.AgentWorking,
						TaskID:    readyTask.ID,
						StartedAt: time.Now(),
					}
					// Find generator agent and set status to Idle
					for j := range state.ActiveAgents {
						if state.ActiveAgents[j].ID == "agent-generator" {
							state.ActiveAgents[j].Status = domain.AgentIdle
						}
					}
					state.ActiveAgents = append(state.ActiveAgents, resolverAgent)

					if err := repo.Save(ctx, state); err != nil {
						return err
					}

					// Call client.Complete(ctx, "resolve git conflict on common.py")
					resp, err := client.Complete(ctx, "resolve git conflict on common.py")
					if err != nil {
						return err
					}

					state.Metadata.TotalTokensUsed += 800
					if currentCost, parseErr := strconv.ParseFloat(state.Metadata.TotalCostUSD, 64); parseErr == nil {
						state.Metadata.TotalCostUSD = fmt.Sprintf("%.4f", currentCost+(float64(800)*pricingRate))
					}

					// Apply conflict resolution write_file actions
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

					// Set task status to domain.TaskSuccess
					readyTask.Status = domain.TaskSuccess
					readyTask.UpdatedAt = time.Now()

					// Log a successful resolve_conflict action
					state.LastActions = append(state.LastActions, domain.Action{
						Timestamp: time.Now(),
						Tool:      "resolve_conflict",
						Args:      map[string]any{"file": "common.py"},
						Success:   true,
						Result:    "Conflict resolved successfully via Resolver agent",
					})

					// Set resolver agent status to domain.AgentCompleted
					for j := range state.ActiveAgents {
						if state.ActiveAgents[j].ID == "agent-resolver" {
							state.ActiveAgents[j].Status = domain.AgentCompleted
							state.ActiveAgents[j].CompletedAt = time.Now()
						}
					}

					if err := repo.Save(ctx, state); err != nil {
						return err
					}
					continue
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

				// If it's a rollback scenario, perform a git rollback and retry
				if strings.Contains(state.Metadata.FeatureName, "rollback") {
					state.LastActions = append(state.LastActions, domain.Action{
						Timestamp: time.Now(),
						Tool:      "git_reset_hard",
						Args:      map[string]any{"commit": "HEAD"},
						Success:   true,
						Result:    "Rolled back changes due to build failure",
					})
					readyTask.Retries++
					readyTask.MaxRetries = 2
					if readyTask.Retries < readyTask.MaxRetries {
						readyTask.Status = domain.TaskPending
					}
				}
			}

			readyTask.UpdatedAt = time.Now()
			state.ActiveAgents[1].Status = domain.AgentIdle
			if err := repo.Save(ctx, state); err != nil {
				return err
			}
			continue
		}

		allCompleted := true
		for _, tTask := range state.Tasks {
			if tTask.Status != domain.TaskSuccess {
				allCompleted = false
				break
			}
		}

		if allCompleted {
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
			if strings.Contains(state.Metadata.FeatureName, "rollback") {
				reqFiles = []string{"main.go"}
			}
			if strings.Contains(state.Metadata.FeatureName, "Refactor") {
				reqFiles = []string{"main.go"}
			}
			if strings.Contains(state.Metadata.FeatureName, "shutdown") {
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
			if err := repo.Save(ctx, state); err != nil {
				return err
			}
			break
		}
	}
	return nil
}
