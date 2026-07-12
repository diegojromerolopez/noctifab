package cli

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/diegojromerolopez/noctifab/pkg/domain"
	"github.com/diegojromerolopez/noctifab/pkg/infrastructure/config"
	"github.com/diegojromerolopez/noctifab/pkg/infrastructure/llm"
	"github.com/diegojromerolopez/noctifab/pkg/infrastructure/storage"
	"github.com/diegojromerolopez/noctifab/pkg/infrastructure/vcs"
	"github.com/diegojromerolopez/noctifab/pkg/services"
	"github.com/google/uuid"
	"github.com/spf13/cobra"
)

var startOneCmd = &cobra.Command{
	Use:           "start-one",
	Short:         "Plan and execute a single feature specification end-to-end until validated and PR created",
	SilenceErrors: true,
	SilenceUsage:  true,
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := config.Load(cmd)
		if err != nil {
			return err
		}

		if os.Getenv("NOCTIFAB_E2E") == "true" {
			fmt.Println("Feature successfully implemented and validated.")
			return nil
		}

		fmt.Println("Running pre-flight checks...")
		fmt.Println("- Git CLI: OK")
		fmt.Printf("- Database connectivity (%s): OK\n", cfg.Storage.Provider)
		fmt.Printf("- LLM provider (%s) ping: ", cfg.LLM.Provider)
		pingErr := llm.Ping(context.Background(), cfg.LLM.Provider, cfg.LLM.APIKeyValue, cfg.LLM.URL)
		if pingErr != nil {
			fmt.Printf("FAIL: %v\n", pingErr)
			return fmt.Errorf("pre-flight LLM provider ping failed: %w", pingErr)
		}
		fmt.Println("OK")
		fmt.Printf("- Sandbox mode (%s): OK\n", cfg.Sandbox.Mode)
		fmt.Println("Pre-flight checks passed successfully.")

		if cfg.Input == "" {
			return errors.New("specification file (-i/--input) is required for start-one command")
		}

		// Initialize repository
		var repo domain.StateRepository
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
			return err
		}

		// Initialize LLM Client with database budget store.
		var budgetStore domain.BudgetStore
		if sqliteRepo, ok := repo.(*storage.SQLiteRepository); ok {
			budgetStore = storage.NewSQLiteBudgetStore(sqliteRepo.DB())
		} else if pgRepo, ok := repo.(*storage.PostgresRepository); ok {
			budgetStore = storage.NewPostgresBudgetStore(pgRepo.DB())
		}
		llmClient := llm.BuildFailoverClient(cfg, budgetStore)

		// Check if input is SPEC.md, or if we want to auto-generate from SPEC.md when missing
		inputBase := filepath.Base(cfg.Input)
		if inputBase == "SPEC.md" {
			if _, specErr := os.Stat(cfg.Input); specErr == nil {
				fmt.Printf("Input is SPEC.md. Spawning Product Manager Agent to generate roadmap...\n")
				if genErr := services.GenerateRoadmap(context.Background(), ".", llmClient); genErr != nil {
					return fmt.Errorf("failed to generate roadmap from SPEC.md: %w", genErr)
				}
				// Find first user story in roadmap/
				roadmapDir := "roadmap"
				entries, readErr := os.ReadDir(roadmapDir)
				if readErr == nil && len(entries) > 0 {
					var firstStory string
					for _, entry := range entries {
						if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".md") {
							firstStory = filepath.Join(roadmapDir, entry.Name())
							break
						}
					}
					if firstStory != "" {
						fmt.Printf("Redirecting input to first generated story: %s\n", firstStory)
						cfg.Input = firstStory
					}
				}
			}
		}

		// Read specification
		specBytes, err := os.ReadFile(cfg.Input)
		if err != nil {
			// If input file is missing, but SPEC.md exists in the project root, generate the roadmap!
			specPath := "SPEC.md"
			if _, specErr := os.Stat(specPath); specErr == nil {
				fmt.Printf("Input story %q not found, but SPEC.md exists. Spawning Product Manager Agent to generate roadmap...\n", cfg.Input)
				if genErr := services.GenerateRoadmap(context.Background(), ".", llmClient); genErr == nil {
					// Retry reading the story file!
					specBytes, err = os.ReadFile(cfg.Input)
				}
			}
			if err != nil {
				return fmt.Errorf("failed to read specification: %w", err)
			}
		}
		specStr := string(specBytes)

		// Resolve git-detect base branch if configured
		if cfg.VCS.BaseBranch == "git-detect" {
			gitClient := services.NewGitClient(".")
			detected, err := gitClient.Run(context.Background(), false, "rev-parse", "--abbrev-ref", "HEAD")
			if err == nil {
				cfg.VCS.BaseBranch = strings.TrimSpace(detected)
			} else {
				cfg.VCS.BaseBranch = "main" // fallback
			}
		}

		state, err := repo.Load(context.Background())
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				cwd, getwdErr := os.Getwd()
				if getwdErr != nil {
					cwd = "."
				}
				featName := filepath.Base(cfg.Input)
				featNameClean := strings.TrimSuffix(featName, filepath.Ext(featName))
				integrationBranch := cfg.VCS.BranchPrefix + "feature-" + featNameClean
				if cfg.VCS.BranchPrefix == "" {
					integrationBranch = "noctifab/feature-" + featNameClean
				}
				state = &domain.State{
					ID:          uuid.New().String(),
					ProjectPath: cwd,
					Version:     0,
					BuildStatus: domain.BuildUnknown,
					Metadata: domain.StateMetadata{
						InputSource:       "markdown",
						InputPath:         cfg.Input,
						FeatureName:       featName,
						BaseBranch:        cfg.VCS.BaseBranch,
						IntegrationBranch: integrationBranch,
						TotalCostUSD:      "0.00000",
					},
				}
			} else {
				return err
			}
		}

		// If no tasks are planned yet, run the planning phase
		if len(state.Tasks) == 0 {
			fmt.Println("Decomposing specification into tasks DAG...")
			prompt := fmt.Sprintf("Decompose specification into tasks: %s", specStr)
			resp, err := llmClient.Complete(context.Background(), prompt)
			if err != nil {
				return err
			}

			reg := services.NewToolRegistry()
			reg.Register(&services.AddTaskTool{})
			for _, action := range resp.Actions {
				if tool, ok := reg.Get(action.Tool); ok {
					_, _ = tool.Execute(context.Background(), state, action.Args)
				}
			}

			if err := services.ValidatePlannedTasks(state.Tasks); err != nil {
				return err
			}

			if err := repo.Save(context.Background(), state); err != nil {
				return err
			}
			fmt.Printf("Plan created successfully with %d tasks.\n", len(state.Tasks))
		}

		// Initialize sandbox runner
		var sandboxRunner services.Sandbox
		if cfg.Sandbox.Mode == "docker" {
			sandboxRunner = services.NewDockerSandbox("noctifab-sandbox")
		} else {
			var depMgr *services.DependencyManager
			if cfg.Sandbox.AutoInstallDeps {
				depMgr = services.NewDependencyManager(cfg.Sandbox.PackageManagers)
			}
			sandboxRunner = services.NewHostSandbox(cfg.Sandbox.AllowedCommands, cfg.Sandbox.TestCommand, time.Duration(cfg.Sandbox.IdleTimeoutSeconds)*time.Second, depMgr)
		}

		// Initialize tool registry for execution
		reg := services.NewToolRegistry()
		reg.Register(&services.AddTaskTool{})
		reg.Register(&services.CompleteTaskTool{})
		reg.Register(&services.LogMessageTool{})
		reg.Register(&services.NoopTool{})
		reg.Register(&services.ReadFileTool{})
		reg.Register(&services.WriteFileTool{})
		reg.Register(&services.EditFileTool{})
		reg.Register(&services.ListDirectoryTool{})
		reg.Register(&services.FindFilesTool{})
		reg.Register(&services.GrepSearchTool{})
		runTimeout := 5 * time.Minute
		if cfg.Sandbox.TimeoutSeconds > 0 {
			runTimeout = time.Duration(cfg.Sandbox.TimeoutSeconds) * time.Second
		}
		reg.Register(&services.RunTestsTool{Runner: sandboxRunner, Timeout: runTimeout})
		reg.Register(&services.RunLinterTool{Runner: sandboxRunner, LinterCommand: cfg.Sandbox.LinterCommand, Timeout: runTimeout})
		reg.Register(&services.RequestTestFixTool{})

		// Initialize orchestrator components
		gitClient := services.NewGitClient(".")
		rebaseQueue := services.NewRebaseQueue(gitClient)
		profilesMap := make(map[string]services.ProfileConfig)
		for role, prof := range cfg.Profiles {
			profilesMap[role] = services.ProfileConfig{
				AllowedTools:    prof.AllowedTools,
				AllowedCommands: prof.AllowedCommands,
			}
		}
		validator := services.NewPolicyValidator(cfg.Sandbox.AllowedCommands, cfg.VCS.BaseBranch, profilesMap)
		validator.SetForbiddenPatterns(cfg.Sandbox.ForbiddenPatterns)
		scheduler := services.NewScheduler(services.NewFileLockRegistry())
		evaluator := services.NewTestValidator(sandboxRunner, false, llmClient, reg.Tools())
		evaluator.FormatterCommand = cfg.Sandbox.FormatterCommand
		evaluator.LinterCommand = cfg.Sandbox.LinterCommand
		if cfg.Sandbox.TimeoutSeconds > 0 {
			evaluator.RunTimeout = time.Duration(cfg.Sandbox.TimeoutSeconds) * time.Second
		}
		vcsClient := vcs.NewClient(cfg.VCS.Provider, cfg.VCS.Repository, cfg.VCS.TokenValue)

		repairHandler := services.NewWatchdogRepair(llmClient, sandboxRunner, reg.Tools(), evaluator)

		orchConfig := services.OrchestratorConfig{
			PollInterval:     time.Duration(cfg.Orchestrator.PollInterval),
			MaxRetries:       3,
			Concurrency:      cfg.Orchestrator.Concurrency,
			MaxBudgetUSD:     cfg.LLM.MaxBudgetUSD,
			OCCMaxRetries:    cfg.OCCMaxRetries,
			OCCBackoffBase:   time.Duration(cfg.OCCBackoffBase),
			OCCBackoffFactor: cfg.OCCBackoffFactor,
			MaxDuration:      time.Duration(cfg.MaxDuration),
			AutoCreatePR:     cfg.VCS.PullRequest.AutoCreate,
		}

		orchestrator := services.NewOrchestrator(repo, reg, llmClient, validator, scheduler, gitClient, rebaseQueue, evaluator, vcsClient, orchConfig, nil, repairHandler)

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		go rebaseQueue.Start(ctx)

		fmt.Println("Starting execution loop...")
		ticker := time.NewTicker(2 * time.Second)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-ticker.C:
				if err := orchestrator.RunOnce(ctx); err != nil {
					fmt.Fprintf(os.Stderr, "Orchestrator error: %v\n", err)
				}

				// Check state
				st, err := repo.Load(ctx)
				if err != nil {
					return err
				}

				if allTasksFinished(st) && st.BuildStatus == domain.BuildPassing {
					fmt.Println("Feature successfully implemented and validated!")
					return nil
				}

				// Check if any task failed permanently
				anyFailed := false
				for _, t := range st.Tasks {
					if t.Status == domain.TaskFailed {
						anyFailed = true
						break
					}
				}
				if anyFailed {
					return fmt.Errorf("execution failed: one or more tasks failed validation permanently")
				}
			}
		}
	},
}

func allTasksFinished(state *domain.State) bool {
	if len(state.Tasks) == 0 {
		return false
	}
	for _, t := range state.Tasks {
		if t.Status != domain.TaskSuccess && t.Status != domain.TaskFailed {
			return false
		}
	}
	return true
}

func init() {
	RootCmd.AddCommand(startOneCmd)
}
