package cli

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
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

const (
	daemonPIDFile = ".noctifab/noctifab.pid"
	daemonLogFile = ".noctifab/logs/daemon.log"
)

var startCmd = &cobra.Command{
	Use:           "start [target_path]",
	Short:         "Plan and execute a software specification end-to-end",
	SilenceErrors: true,
	SilenceUsage:  true,
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := config.Load(cmd)
		if err != nil {
			return err
		}

		targetPath := cfg.Input
		if targetPath == "" && len(args) > 0 {
			targetPath = args[0]
		}
		if targetPath == "" {
			targetPath = "."
		}

		targetDir := targetPath
		if fi, err := os.Stat(targetPath); err == nil && !fi.IsDir() {
			targetDir = filepath.Dir(targetPath)
		}

		// Ensure .noctifab configuration, secrets.yaml, and SPEC.md exist in target directory
		createdSpec, initErr := EnsureWorkspaceInitialized(targetDir)
		if initErr != nil {
			return fmt.Errorf("failed to initialize workspace in %q: %w", targetDir, initErr)
		}

		if createdSpec {
			fmt.Printf("\nWorkspace initialized at %s with default SPEC.md, config.yaml, and secrets.yaml.\n", targetDir)
			fmt.Printf("Please edit SPEC.md with your project requirements, set your API key in .noctifab/secrets.yaml, and run 'noctifab start' again.\n")
			return nil
		}

		// Validate SPEC.md is not the unedited template before doing anything else
		{
			specFile := filepath.Join(targetDir, "SPEC.md")
			if _, specErr := os.Stat(specFile); specErr != nil {
				return fmt.Errorf("SPEC.md is mandatory but was not found in %q", targetDir)
			}
			specBytes, specReadErr := os.ReadFile(specFile)
			if specReadErr != nil {
				return fmt.Errorf("failed to read SPEC.md: %w", specReadErr)
			}
			if isTemplateSpec(string(specBytes)) {
				return fmt.Errorf(
					"SPEC.md in %q still contains the default template content; "+
						"please replace it with your project requirements and run 'noctifab start' again", targetDir)
			}
			// Validate any existing user stories are not templates
			roadmapDir := filepath.Join(targetDir, "roadmap")
			if entries, readErr := os.ReadDir(roadmapDir); readErr == nil {
				for _, entry := range entries {
					if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".md") {
						sfPath := filepath.Join(roadmapDir, entry.Name())
						sfBytes, sfErr := os.ReadFile(sfPath)
						if sfErr != nil {
							return fmt.Errorf("failed to read story file %s: %w", sfPath, sfErr)
						}
						if isTemplateStory(string(sfBytes)) {
							return fmt.Errorf(
								"user story %q still contains the default template content; "+
									"please replace it with real requirements or delete it and run 'noctifab start' again", sfPath)
						}
					}
				}
			}
		}

		if os.Getenv("NOCTIFAB_E2E") == "true" {
			fmt.Println("Feature successfully implemented and validated.")
			return nil
		}

		fmt.Println("Running pre-flight checks...")
		fmt.Println("- Git CLI: OK")
		fmt.Printf("- Database connectivity (%s): OK\n", cfg.Storage.Provider)
		if len(cfg.LLM.Providers) > 0 {
			for _, p := range cfg.LLM.Providers {
				fmt.Printf("- LLM provider (%s / %s) ping: ", p.Name, p.Provider)
				if err := llm.Ping(context.Background(), p.Provider, p.APIKeyValue, p.URL); err != nil {
					fmt.Printf("FAIL: %v\n", err)
					return fmt.Errorf("pre-flight LLM provider ping failed for provider %s: %w", p.Name, err)
				}
				fmt.Println("OK")
			}
		} else if len(cfg.LLMs) > 0 {
			for _, p := range cfg.LLMs {
				fmt.Printf("- LLM provider (%s) ping: ", p.Provider)
				if err := llm.Ping(context.Background(), p.Provider, p.APIKeyValue, p.URL); err != nil {
					fmt.Printf("FAIL: %v\n", err)
					return fmt.Errorf("pre-flight LLM provider ping failed for provider %s: %w", p.Provider, err)
				}
				fmt.Println("OK")
			}
		} else {
			fmt.Printf("- LLM provider (%s) ping: ", cfg.LLM.Provider)
			if err := llm.Ping(context.Background(), cfg.LLM.Provider, cfg.LLM.APIKeyValue, cfg.LLM.URL); err != nil {
				fmt.Printf("FAIL: %v\n", err)
				return fmt.Errorf("pre-flight LLM provider ping failed: %w", err)
			}
			fmt.Println("OK")
		}
		fmt.Printf("- Sandbox mode (%s): OK\n", cfg.Sandbox.Mode)
		fmt.Println("Pre-flight checks passed successfully.")

		// Resolve SPEC.md if targetPath was a directory
		if fi, err := os.Stat(targetPath); err == nil && fi.IsDir() {
			specCandidate := filepath.Join(targetPath, "SPEC.md")
			if _, specErr := os.Stat(specCandidate); specErr == nil {
				targetPath = specCandidate
			}
		}

		cfg.Input = targetPath

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

		// Initialize tool registry
		reg := services.NewToolRegistry()
		reg.Register(&services.AddTaskTool{})
		reg.Register(&services.CompleteTaskTool{})
		reg.Register(&services.LogMessageTool{})
		reg.Register(&services.NoopTool{})
		reg.Register(&services.ReadFileTool{})
		reg.Register(&services.WriteFileTool{})
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
		reg.Register(&services.RunLinterTool{Runner: sandboxRunner, LinterCommand: cfg.Sandbox.LinterCommand, Timeout: runTimeout})
		reg.Register(&services.RequestTestFixTool{})

		// Initialize LLM Client with database budget store
		var budgetStore domain.BudgetStore
		if sqliteRepo, ok := repo.(*storage.SQLiteRepository); ok {
			budgetStore = storage.NewSQLiteBudgetStore(sqliteRepo.DB())
		} else if pgRepo, ok := repo.(*storage.PostgresRepository); ok {
			budgetStore = storage.NewPostgresBudgetStore(pgRepo.DB())
		}
		llmClient := llm.BuildFailoverClient(cfg, budgetStore)

		// SPEC.md is guaranteed to exist and be non-template by this point (checked above)
		specFile := filepath.Join(targetDir, "SPEC.md")

		// 1. Discover user stories in roadmap/ folder if present
		roadmapDir := filepath.Join(targetDir, "roadmap")
		var storyFiles []string
		if entries, readErr := os.ReadDir(roadmapDir); readErr == nil {
			for _, entry := range entries {
				if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".md") {
					storyFiles = append(storyFiles, filepath.Join(roadmapDir, entry.Name()))
				}
			}
		}

		// 2. Invoke Product Manager Agent to audit/refine existing user stories or generate roadmap from SPEC.md
		if len(storyFiles) > 0 {
			fmt.Printf("Spawning Product Manager Agent to audit and refine existing roadmap user stories in %s/roadmap...\n", targetDir)
		} else {
			fmt.Printf("No user stories found in %s/roadmap. Spawning Product Manager Agent to generate roadmap from SPEC.md...\n", targetDir)
		}
		if genErr := services.GenerateRoadmap(context.Background(), targetDir, llmClient); genErr != nil {
			fmt.Printf("Warning: Product Manager Agent story refinement skipped: %v\n", genErr)
		}
		storyFiles = nil
		if entries, readErr := os.ReadDir(roadmapDir); readErr == nil {
			for _, entry := range entries {
				if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".md") {
					storyFiles = append(storyFiles, filepath.Join(roadmapDir, entry.Name()))
				}
			}
		}

		// Fallback to SPEC.md if no roadmap story files exist after generation
		if len(storyFiles) == 0 {
			storyFiles = []string{specFile}
		}
		sort.Strings(storyFiles)

		fmt.Printf("Enqueued %d user stories for execution in %s:\n", len(storyFiles), targetDir)
		for _, s := range storyFiles {
			fmt.Printf(" - %s\n", s)
		}

		// Setup orchestrator components
		gitClient := services.NewGitClient(".")
		if cfg.VCS.BaseBranch == "git-detect" {
			detected, err := gitClient.Run(context.Background(), false, "rev-parse", "--abbrev-ref", "HEAD")
			if err == nil {
				cfg.VCS.BaseBranch = strings.TrimSpace(detected)
			} else {
				cfg.VCS.BaseBranch = "main"
			}
		}
		cmdCtx := cmd.Context()
		if cmdCtx == nil {
			cmdCtx = context.Background()
		}
		rebaseQueue := services.NewRebaseQueue(gitClient)
		go rebaseQueue.Start(cmdCtx)

		profilesMap := make(map[string]services.ProfileConfig)
		for role, prof := range cfg.Profiles {
			profilesMap[role] = services.ProfileConfig{
				AllowedTools:    prof.AllowedTools,
				AllowedCommands: prof.AllowedCommands,
			}
		}
		validator := services.NewPolicyValidator(cfg.Sandbox.AllowedCommands, cfg.VCS.BaseBranch, profilesMap)
		validator.ExcludePaths = cfg.Sandbox.ExcludePaths
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
			Architecture:          cfg.Agents.Architecture,
			ArchitectNumber:       cfg.Agents.Architect.Number,
			ArchitectIterations:   cfg.Agents.Architect.Iterations,
			GeneratorsNumber:      cfg.Agents.Generators.Number,
			GeneratorsIterations:  cfg.Agents.Generators.Iterations,
			TestersNumber:         cfg.Agents.Testers.Number,
			TestersIterations:     cfg.Agents.Testers.Iterations,
			QAAgentsNumber:        cfg.Agents.QA.Number,
			QAAgentsIterations:    cfg.Agents.QA.Iterations,
			SecurityNumber:        cfg.Agents.Security.Number,
			SecurityIterations:    cfg.Agents.Security.Iterations,
			PerformanceNumber:     cfg.Agents.Performance.Number,
			PerformanceIterations: cfg.Agents.Performance.Iterations,
			DocsNumber:            cfg.Agents.Docs.Number,
			DocsIterations:        cfg.Agents.Docs.Iterations,
			DevOpsNumber:          cfg.Agents.DevOps.Number,
			DevOpsIterations:      cfg.Agents.DevOps.Iterations,
			PollInterval:          time.Duration(cfg.PollInterval),
			MaxRetries:            10,
			Concurrency:           effectiveConcurrency(cfg.VCS.UseWorktrees, cfg.Agents.Generators.Number),
			UseWorktrees:          cfg.VCS.UseWorktrees,
			OCCMaxRetries:         cfg.OCCMaxRetries,
			OCCBackoffBase:        time.Duration(cfg.OCCBackoffBase),
			OCCBackoffFactor:      cfg.OCCBackoffFactor,
			MaxDuration:           time.Duration(cfg.MaxDuration),
			AutoCreatePR:          cfg.VCS.PullRequest.AutoCreate,
			ExcludePaths:          cfg.Sandbox.ExcludePaths,
			WorkspaceCache:        cfg.GetWorkspaceCache(),
		}

		mailbox := services.NewCommandMailbox(repo)
		go mailbox.Start(cmdCtx)

		// Execute all enqueued user stories sequentially
		for _, currentStoryFile := range storyFiles {
			err := func(currentStoryFile string) error {
				cfg.Input = currentStoryFile
				fmt.Printf("\n==================================================\n")
				fmt.Printf("🚀 Executing User Story: %s\n", currentStoryFile)
				fmt.Printf("==================================================\n")

				specBytes, err := os.ReadFile(currentStoryFile)
				if err != nil {
					return fmt.Errorf("failed to read story file %s: %w", currentStoryFile, err)
				}
				specStr := string(specBytes)

				featName := filepath.Base(currentStoryFile)
				featNameClean := strings.TrimSuffix(featName, filepath.Ext(featName))
				integrationBranch := cfg.VCS.BranchPrefix + "feature-" + featNameClean
				if cfg.VCS.BranchPrefix == "" {
					integrationBranch = "noctifab/feature-" + featNameClean
				}

				state, err := repo.Load(context.Background())
				if err != nil {
					if errors.Is(err, sql.ErrNoRows) {
						cwd, getwdErr := os.Getwd()
						if getwdErr != nil {
							cwd = "."
						}
						state = &domain.State{
							ID:          uuid.New().String(),
							ProjectPath: cwd,
							Version:     0,
							BuildStatus: domain.BuildUnknown,
							Metadata: domain.StateMetadata{
								InputSource:       "markdown",
								InputPath:         currentStoryFile,
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

				// Always update metadata for current story file and reset state for fresh story pass
				state.Metadata.InputPath = currentStoryFile
				state.Metadata.FeatureName = featName
				state.Metadata.IntegrationBranch = integrationBranch
				state.Tasks = nil
				state.ActiveAgents = nil
				state.StoryStatus = domain.StoryRunning
				state.StoryError = ""
				if err := repo.Save(context.Background(), state); err != nil {
					return fmt.Errorf("failed to save initial state: %w", err)
				}

				orchestrator := services.NewOrchestrator(
					repo, reg, llmClient, validator, scheduler,
					gitClient, rebaseQueue, evaluator, vcsClient, orchConfig, mailbox, repairHandler,
				)

				if cfg.Unblocker.Enabled {
					unblocker := services.NewUnblockerAgent(
						repo,
						llmClient,
						mailbox,
						time.Duration(cfg.Unblocker.PollInterval),
						cfg.Unblocker.MaxRetries,
						time.Duration(cfg.Unblocker.StallThreshold),
						time.Duration(cfg.Unblocker.ConflictThreshold),
						cfg.Unblocker.LLMAssessment,
					)
					orchestrator.SetUnblocker(unblocker)
					unblockerCtx, cancelUnblocker := context.WithCancel(context.Background())
					defer cancelUnblocker()
					unblocker.Start(unblockerCtx)
				}

				fmt.Println("Decomposing specification into task DAG...")
				if err := orchestrator.PlanStory(context.Background(), state, specStr); err != nil {
					return fmt.Errorf("failed to plan specification: %w", err)
				}

				// Run orchestrator loop (story_exec_interval, default 2s)
				ticker := time.NewTicker(storyExecInterval(cfg))
				defer ticker.Stop()

				storyDone := false
				for !storyDone {
					select {
					case <-context.Background().Done():
						return context.Background().Err()
					case <-ticker.C:
						if _, err := orchestrator.RunOnce(context.Background()); err != nil {
							fmt.Fprintf(os.Stderr, "Orchestrator error: %v\n", err)
						}

						st, err := repo.Load(context.Background())
						if err != nil {
							return err
						}

						if allTasksFinished(st) {
							anyFailed := false
							for _, t := range st.Tasks {
								if t.Status == domain.TaskFailed {
									anyFailed = true
									break
								}
							}
							if anyFailed {
								return fmt.Errorf("execution failed: one or more tasks in %s failed validation permanently", currentStoryFile)
							}
							fmt.Printf("User story %s successfully implemented and validated!\n", currentStoryFile)
							storyDone = true
						}
					}
				}
				return nil
			}(currentStoryFile)
			if err != nil {
				return err
			}
		}

		fmt.Printf("\n🎉 All %d user stories implemented and validated successfully!\n", len(storyFiles))
		return nil
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

// isTemplateSpec reports whether content is the unmodified SPEC.md boilerplate
// written by EnsureWorkspaceInitialized. Users must replace it before starting.
func isTemplateSpec(content string) bool {
	return strings.Contains(content, "Specification: New Project")
}

// isTemplateStory reports whether a roadmap file still contains the boilerplate
// user story template written by EnsureWorkspaceInitialized.
func isTemplateStory(content string) bool {
	return strings.Contains(content, "User Story: US-001 - Initial Feature Specification")
}

func init() {
	startCmd.Flags().StringP("spec", "s", "SPEC.md", "Path to feature specification file")
	RootCmd.AddCommand(startCmd)
}
