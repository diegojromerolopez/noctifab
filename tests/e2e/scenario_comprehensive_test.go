package e2e

import (
	"context"
	"database/sql"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/diegojromerolopez/noctifab/pkg/domain"
	"github.com/diegojromerolopez/noctifab/pkg/infrastructure/storage"
	"github.com/diegojromerolopez/noctifab/pkg/usecase"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestScenario_ComprehensiveAutonomy(t *testing.T) {
	ctx := context.Background()
	tempDir := t.TempDir()

	t.Run("BudgetStore persists daily usage incrementally across sessions", func(t *testing.T) {
		db := openBudgetDB(t)
		defer func() { _ = db.Close() }()
		_, err := db.ExecContext(ctx, "TRUNCATE TABLE budget_usage CASCADE")
		require.NoError(t, err)
		budgetStore := storage.NewPostgresBudgetStore(db)

		usage, err := budgetStore.GetDailyUsage(ctx, "2026-07-02", "gpt-4o")
		require.NoError(t, err)
		assert.Equal(t, 0.0, usage)

		err = budgetStore.IncrementUsage(ctx, "2026-07-02", "gpt-4o", 0.05)
		require.NoError(t, err)

		usage, err = budgetStore.GetDailyUsage(ctx, "2026-07-02", "gpt-4o")
		require.NoError(t, err)
		assert.InDelta(t, 0.05, usage, 0.0001)

		err = budgetStore.IncrementUsage(ctx, "2026-07-02", "gpt-4o", 0.03)
		require.NoError(t, err)

		usage, err = budgetStore.GetDailyUsage(ctx, "2026-07-02", "gpt-4o")
		require.NoError(t, err)
		assert.InDelta(t, 0.08, usage, 0.0001)

		usage, err = budgetStore.GetDailyUsage(ctx, "2026-07-01", "gpt-4o")
		require.NoError(t, err)
		assert.Equal(t, 0.0, usage)
	})

	t.Run("OCC version conflict detection blocks stale saves", func(t *testing.T) {
		repo, cleanup := setupRepo(t, ctx, tempDir, "occ", "occ-conflict-session")
		defer cleanup()

		state := &domain.State{
			ID:          "occ-conflict-session",
			ProjectPath: filepath.Join(tempDir, "occ"),
			Version:     0,
			BuildStatus: domain.BuildUnknown,
			Metadata: domain.StateMetadata{
				InputSource: "local", InputPath: "req.md",
				FeatureName: "occ-test", TotalCostUSD: "0.0000",
			},
		}
		err := repo.Save(ctx, state)
		require.NoError(t, err)
		require.Equal(t, 1, state.Version)

		staleState := &domain.State{
			ID:          "occ-conflict-session",
			ProjectPath: filepath.Join(tempDir, "occ"),
			Version:     0,
			BuildStatus: domain.BuildFailing,
			Metadata: domain.StateMetadata{
				InputSource: "local", InputPath: "req.md",
				FeatureName: "occ-test", TotalCostUSD: "0.0000",
			},
		}
		err = repo.Save(ctx, staleState)
		assert.ErrorIs(t, err, domain.ErrVersionConflict)

		fresh, err := repo.Load(ctx)
		require.NoError(t, err)
		assert.Equal(t, 1, fresh.Version)
	})

	t.Run("Watchdog kills process exceeding max wall-clock duration", func(t *testing.T) {
		wd := usecase.Watchdog{MaxDuration: 100 * time.Millisecond}
		cmd := exec.CommandContext(ctx, "sleep", "5")
		_, err := wd.Run(ctx, cmd)
		require.Error(t, err)
		assert.ErrorIs(t, err, usecase.ErrWatchdogMaxDuration)
	})

	t.Run("FlakyDetector quarantines non-unanimous test runs", func(t *testing.T) {
		flaky := []usecase.TestRunResult{
			{RunID: 1, Passed: true, Output: "ok"},
			{RunID: 2, Passed: false, Output: "random failure"},
			{RunID: 3, Passed: true, Output: "ok"},
		}
		r := usecase.DetectFlaky(flaky)
		assert.True(t, r.Flaky)
		assert.Equal(t, 2, r.PassedCount)
		assert.Equal(t, 1, r.FailedCount)

		stable := []usecase.TestRunResult{
			{RunID: 1, Passed: true, Output: "ok"},
			{RunID: 2, Passed: true, Output: "ok"},
			{RunID: 3, Passed: true, Output: "ok"},
		}
		r2 := usecase.DetectFlaky(stable)
		assert.False(t, r2.Flaky)

		prompt := usecase.BuildFlakyStabilizationPrompt(flaky, "")
		assert.Contains(t, prompt, "random failure")
		assert.Contains(t, prompt, "time.Sleep")
	})

	t.Run("DependencyManager detects missing tool from error output", func(t *testing.T) {
		dm := usecase.NewDependencyManager([]string{"pip", "go", "npm"})

		tool, found := dm.DetectMissingTool("executable file not found: pytest")
		assert.True(t, found)
		assert.Equal(t, "pytest", tool)

		_, found = dm.DetectMissingTool("everything is fine")
		assert.False(t, found)

		assert.True(t, dm.IsAllowed("pip"))
		assert.False(t, dm.IsAllowed("brew"))
	})

	t.Run("HotReloadManager handoff file round-trips correctly", func(t *testing.T) {
		handoffPath := filepath.Join(t.TempDir(), "handoff.json")

		state := usecase.HandoffState{NewPID: 12345, Status: usecase.HandoffActive, Message: "healthy"}
		data, _ := json.Marshal(state)
		err := os.WriteFile(handoffPath, data, 0644)
		require.NoError(t, err)

		raw, err := os.ReadFile(handoffPath)
		require.NoError(t, err)
		var readBack usecase.HandoffState
		err = json.Unmarshal(raw, &readBack)
		require.NoError(t, err)
		assert.Equal(t, 12345, readBack.NewPID)
		assert.Equal(t, usecase.HandoffActive, readBack.Status)
		assert.Equal(t, "healthy", readBack.Message)
	})

	t.Run("IntentDisambiguator infers answer from context", func(t *testing.T) {
		repo, cleanup := setupRepo(t, ctx, tempDir, "disambiguate", "disambig-session")
		defer cleanup()

		gitClient := usecase.NewGitClient(filepath.Join(tempDir, "disambiguate"))
		llm := &mockLLMClient{
			completeFn: func(_ context.Context, _ string) (*domain.LLMResponse, error) {
				return &domain.LLMResponse{
					Actions: []domain.LLMAction{
						{Tool: "answer", Args: map[string]any{"answer": "SQLite"}},
					},
				}, nil
			},
		}
		disambiguator := usecase.NewIntentDisambiguator(gitClient, llm)

		state := &domain.State{
			ID:          "disambig-session",
			ProjectPath: filepath.Join(tempDir, "disambiguate"),
			Version:     0,
			BuildStatus: domain.BuildUnknown,
			Metadata: domain.StateMetadata{
				InputSource: "local", InputPath: "req.md",
				FeatureName: "disambiguate-test", TotalCostUSD: "0.0000",
				BaseBranch: "main",
			},
			Files: []domain.FileInfo{
				{Path: "models.py", Size: 100},
				{Path: "views.py", Size: 200},
			},
		}
		err := repo.Save(ctx, state)
		require.NoError(t, err)

		answer, err := disambiguator.Disambiguate(ctx, domain.Clarification{
			Question: "Which database should we use?",
		}, state)
		require.NoError(t, err)
		assert.Equal(t, "SQLite", answer)
	})

	t.Run("SAST scanner returns passed when disabled", func(t *testing.T) {
		scanner := &usecase.SASTScanner{
			Config: usecase.SASTConfig{Enabled: false},
		}
		result, err := scanner.Run(ctx, "/tmp")
		require.NoError(t, err)
		assert.True(t, result.Passed)
		assert.Empty(t, result.Issues)
	})

	t.Run("CostForTokens calculates correctly for known model tiers", func(t *testing.T) {
		cost := domain.CostForTokens("gpt-4o", 1000, 500)
		assert.InDelta(t, 0.025, cost, 0.0001)

		cost = domain.CostForTokens("gpt-3.5-turbo", 2000, 1000)
		assert.InDelta(t, 0.0025, cost, 0.0001)

		cost = domain.CostForTokens("unknown-model", 1000, 500)
		assert.Equal(t, 0.0, cost)

		cost = domain.CostForTokens("claude-3-opus", 1000, 500)
		assert.InDelta(t, 0.0525, cost, 0.0001)
	})

	t.Run("State repository persists and loads complex state through PostgreSQL", func(t *testing.T) {
		subDir := filepath.Join(tempDir, "complex-state")
		err := os.MkdirAll(subDir, 0755)
		require.NoError(t, err)
		reqPath := filepath.Join(subDir, "requirements.md")
		err = os.WriteFile(reqPath, []byte("complex state test"), 0644)
		require.NoError(t, err)

		repo, cleanup := setupRepo(t, ctx, tempDir, "complex-state", "complex-session")
		defer cleanup()

		state := &domain.State{
			ID:          "complex-session",
			ProjectPath: subDir,
			Version:     0,
			BuildStatus: domain.BuildUnknown,
			Metadata: domain.StateMetadata{
				InputSource: "markdown", InputPath: "requirements.md",
				FeatureName: "Complex State Test", BaseBranch: "main",
				IntegrationBranch: "feature/complex", ProjectVersion: "1.0.0",
				TotalCostUSD: "0.0000",
			},
			Tasks: []domain.Task{
				{ID: "task-1", Title: "First", Description: "First task", Status: domain.TaskPending, ChangeType: domain.ChangeTypeFeature},
				{ID: "task-2", Title: "Second", Description: "Second task", Status: domain.TaskSuccess, ChangeType: domain.ChangeTypeFeature},
			},
			Clarifications: []domain.Clarification{
				{Question: "Which DB?", Answer: "SQLite", Resolved: true, AskedAt: time.Now()},
			},
			LastActions: []domain.Action{
				{Timestamp: time.Now(), Tool: "plan", Success: true, Result: "Planned tasks"},
			},
			Files: []domain.FileInfo{
				{Path: "main.go", Size: 500},
			},
		}
		err = repo.Save(ctx, state)
		require.NoError(t, err)
		assert.Greater(t, state.Version, 0)

		loaded, err := repo.Load(ctx)
		require.NoError(t, err)
		assert.Equal(t, "complex-session", loaded.ID)
		assert.Equal(t, domain.BuildUnknown, loaded.BuildStatus)
		assert.Len(t, loaded.Tasks, 2)
		assert.Equal(t, "First", loaded.Tasks[0].Title)
		assert.Equal(t, domain.TaskSuccess, loaded.Tasks[1].Status)
		assert.Len(t, loaded.Clarifications, 1)
		assert.True(t, loaded.Clarifications[0].Resolved)
		assert.Len(t, loaded.LastActions, 1)
		assert.Len(t, loaded.Files, 1)
	})

	t.Run("Failover budget alert triggers below threshold", func(t *testing.T) {
		db := openBudgetDB(t)
		defer func() { _ = db.Close() }()
		_, err := db.ExecContext(ctx, "TRUNCATE TABLE budget_usage CASCADE")
		require.NoError(t, err)

		budgetStore := storage.NewPostgresBudgetStore(db)

		dates := []string{"2026-07-01", "2026-07-02", "2026-07-03"}
		for _, d := range dates {
			err = budgetStore.IncrementUsage(ctx, d, "deepseek", 0.04)
			require.NoError(t, err)
		}

		total := 0.0
		for _, d := range dates {
			usage, err := budgetStore.GetDailyUsage(ctx, d, "deepseek")
			require.NoError(t, err)
			total += usage
		}
		assert.InDelta(t, 0.12, total, 0.001)

		alert := domain.CostForTokens("deepseek", 25000, 10000)
		assert.InDelta(t, 0.0275, alert, 0.001)

		totalWithAlert := total + alert
		assert.InDelta(t, 0.1475, totalWithAlert, 0.001)
	})
}

func openBudgetDB(t *testing.T) *sql.DB {
	t.Helper()
	dsn := os.Getenv("NOCTIFAB_TEST_DB_DSN")
	if dsn == "" {
		dsn = "postgres://noctifab:noctifab_password@db:5432/noctifab_test?sslmode=disable"
	}
	db, err := sql.Open("pgx", dsn)
	require.NoError(t, err)
	return db
}
