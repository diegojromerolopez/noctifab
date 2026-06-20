package e2e

import (
	"context"
	"database/sql"
	"errors"
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

func scanWorkspaceFiles(dir string) ([]domain.FileInfo, error) {
	var files []domain.FileInfo
	err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			name := info.Name()
			if name == ".noctifab" || name == ".git" || name == "node_modules" {
				return filepath.SkipDir
			}
			return nil
		}
		rel, err := filepath.Rel(dir, path)
		if err != nil {
			return err
		}
		files = append(files, domain.FileInfo{
			Path:         rel,
			Size:         info.Size(),
			LastModified: info.ModTime(),
		})
		return nil
	})
	return files, err
}

func resolveDependencies(tasks []domain.Task) ([]string, error) {
	idMap := make(map[string]string)
	for _, t := range tasks {
		idMap[t.Title] = t.ID
	}

	adj := make(map[string][]string)
	for _, t := range tasks {
		var deps []string
		for _, dep := range t.DependsOn {
			if id, exists := idMap[dep]; exists {
				deps = append(deps, id)
			} else {
				deps = append(deps, dep)
			}
		}
		adj[t.ID] = deps
	}

	visited := make(map[string]int)
	var order []string
	var dfs func(node string) error
	dfs = func(node string) error {
		visited[node] = 1
		for _, dep := range adj[node] {
			if visited[dep] == 1 {
				return errors.New("Cycle detected in task DAG: circular reference")
			}
			if visited[dep] == 0 {
				if err := dfs(dep); err != nil {
					return err
				}
			}
		}
		visited[node] = 2
		order = append(order, node)
		return nil
	}

	for _, t := range tasks {
		if visited[t.ID] == 0 {
			if err := dfs(t.ID); err != nil {
				return nil, err
			}
		}
	}
	return order, nil
}

func runSimulatedOrchestrator(ctx context.Context, repo domain.StateRepository, client *mockLLMClient, workspace string, maxBudgetUSD float64) error {
	const pricingRate = 0.000015

	state, err := repo.Load(ctx)
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
	if err := repo.Save(ctx, state); err != nil {
		return err
	}

	for cycle := 0; cycle < 15; cycle++ {
		state, err = repo.Load(ctx)
		if err != nil {
			return err
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
