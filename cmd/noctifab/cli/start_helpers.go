package cli

import (
	"context"
	"strings"
	"time"

	"github.com/diegojromerolopez/noctifab/pkg/domain"
	"github.com/diegojromerolopez/noctifab/pkg/infrastructure/config"
	"github.com/diegojromerolopez/noctifab/pkg/infrastructure/storage"
	"github.com/diegojromerolopez/noctifab/pkg/services"
)

func initStorageRepo(cfg *config.Config) (domain.StateRepository, domain.BudgetStore, error) {
	var repo domain.StateRepository
	var err error

	if strings.ToLower(cfg.Storage.Provider) == "postgres" {
		repo, err = storage.NewPostgresRepository(context.Background(), cfg.Storage.ConnString, 10, 10)
	} else {
		dbPath := cfg.Storage.ConnString
		if dbPath == "" {
			dbPath = ".noctifab/data/noctifab.db"
		}
		repo, err = storage.NewSQLiteRepository(context.Background(), dbPath)
	}
	if err != nil {
		return nil, nil, err
	}

	var budgetStore domain.BudgetStore
	if sqliteRepo, ok := repo.(*storage.SQLiteRepository); ok {
		budgetStore = storage.NewSQLiteBudgetStore(sqliteRepo.DB())
	} else if pgRepo, ok := repo.(*storage.PostgresRepository); ok {
		budgetStore = storage.NewPostgresBudgetStore(pgRepo.DB())
	}

	return repo, budgetStore, nil
}

func initToolRegistry(cfg *config.Config, sandboxRunner services.Sandbox) *services.ToolRegistry {
	reg := services.NewToolRegistry()
	reg.Register(&services.AddTaskTool{})
	reg.Register(&services.CompleteTaskTool{})
	reg.Register(&services.LogMessageTool{})
	reg.Register(&services.NoopTool{})
	reg.Register(&services.ReadFileTool{})
	reg.Register(&services.WriteFileTool{})
	reg.Register(&services.WriteFilesTool{})
	reg.Register(&services.DeleteFileTool{})
	reg.Register(&services.EditFileTool{})
	reg.Register(&services.ListDirectoryTool{ExcludePaths: cfg.Sandbox.ExcludePaths})
	reg.Register(&services.FindFilesTool{ExcludePaths: cfg.Sandbox.ExcludePaths})
	reg.Register(&services.GrepSearchTool{ExcludePaths: cfg.Sandbox.ExcludePaths})

	runTimeout := 5 * time.Minute
	if cfg.Sandbox.TimeoutSeconds > 0 {
		runTimeout = time.Duration(cfg.Sandbox.TimeoutSeconds) * time.Second
	}
	reg.Register(&services.RunTestsTool{Runner: sandboxRunner, Timeout: runTimeout})
	reg.Register(&services.RunLinterTool{
		Runner:           sandboxRunner,
		LinterCommand:    cfg.Sandbox.GetLinterCommand(),
		FormatterCommand: cfg.Sandbox.FormatterCommand,
		MaxLinterIssues:  cfg.Sandbox.GetMaxLinterIssues(),
		Timeout:          runTimeout,
	})
	reg.Register(&services.RequestTestFixTool{})
	depMgr := services.NewDependencyManager(cfg.Sandbox.PackageManagers)
	reg.Register(&services.InstallPackageTool{DepMgr: depMgr, Runner: sandboxRunner})
	return reg
}

func buildOrchestratorConfig(cfg *config.Config) services.OrchestratorConfig {
	return services.OrchestratorConfig{
		Architecture:           cfg.Agents.Architecture,
		TaskExecutionOrder:     cfg.Agents.TaskExecutionOrder,
		GeneratorsNumber:       cfg.Agents.Generators.Number,
		GeneratorsIterations:   cfg.Agents.Generators.Iterations,
		TestersNumber:          cfg.Agents.Testers.Number,
		TestersIterations:      cfg.Agents.Testers.Iterations,
		PollInterval:           time.Duration(cfg.PollInterval),
		MaxRetries:             10,
		Concurrency:            effectiveConcurrency(cfg.VCS.UseWorktrees, cfg.Agents.Generators.Number),
		UseWorktrees:           cfg.VCS.UseWorktrees,
		OCCMaxRetries:          cfg.Storage.OCC.MaxRetries,
		OCCBackoffBase:         time.Duration(cfg.Storage.OCC.BackoffBase),
		OCCBackoffFactor:       cfg.Storage.OCC.BackoffFactor,
		MaxDuration:            time.Duration(cfg.Runtime.MaxDuration),
		MaxSilentStallDuration: time.Duration(cfg.Runtime.MaxSilentStallDuration),
		MaxTokensPerStory:      cfg.Runtime.MaxTokensPerStory,
		MaxTokensPerTask:       cfg.Runtime.MaxTokensPerTask,
		MaxTokens:              cfg.Runtime.MaxTokens,
		MaxActions:             cfg.Runtime.MaxActions,
		AutoCreatePR:           cfg.VCS.PullRequest.AutoCreate,
		CreateBranch:           cfg.VCS.IsCreateBranchEnabled(),
		ExcludePaths:           cfg.Sandbox.ExcludePaths,
		WorkspaceCache:         cfg.GetWorkspaceCache(),
		QA:                     cfg.Agents.QA,
		LastResort:             cfg.Agents.LastResort,
	}
}
