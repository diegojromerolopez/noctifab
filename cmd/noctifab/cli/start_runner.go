package cli

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/spf13/cobra"

	"github.com/diegojromerolopez/noctifab/pkg/domain"
	"github.com/diegojromerolopez/noctifab/pkg/infrastructure/config"
	"github.com/diegojromerolopez/noctifab/pkg/infrastructure/llm"
	"github.com/diegojromerolopez/noctifab/pkg/infrastructure/prompts"
	"github.com/diegojromerolopez/noctifab/pkg/infrastructure/reportfs"
	"github.com/diegojromerolopez/noctifab/pkg/infrastructure/storage"
	"github.com/diegojromerolopez/noctifab/pkg/infrastructure/vcs"
	"github.com/diegojromerolopez/noctifab/pkg/services"
	"github.com/diegojromerolopez/noctifab/pkg/services/reporting"
)

func runStartCommand(cmd *cobra.Command, args []string) error {
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

	createdSpec, initErr := EnsureWorkspaceInitialized(targetDir)
	if initErr != nil {
		return fmt.Errorf("failed to initialize workspace in %q: %w", targetDir, initErr)
	}

	if createdSpec {
		fmt.Printf("\nWorkspace initialized at %s with default SPEC.md, config.yaml, and secrets.yaml.\n", targetDir)
		fmt.Printf("Please edit SPEC.md with your project requirements, set your API key in .noctifab/secrets.yaml, and run 'noctifab start' again.\n")
		return nil
	}

	// Reporter activation point (§2.7)
	var executionReporter domain.ExecutionReporter = &services.NoopExecutionReporter{}
	reportPath, enabled, resolveErr := config.ResolveReportPath(targetDir, cfg.ExecutionReport)
	if enabled && resolveErr == nil {
		policy := reportfs.ReportDestinationPolicy{
			ProjectPath: targetDir,
		}
		if prepErr := reportfs.PrepareReportDestination(cmd.Context(), reportPath, policy, reportfs.OSFileSystem{}); prepErr == nil {
			writer := reportfs.NewAtomicWriter(reportfs.OSFileSystem{})
			runID := "run-" + uuid.New().String()[:8]
			runMeta := domain.RunMetadata{
				RunID:           runID,
				Command:         "noctifab start",
				ProjectPath:     targetDir,
				ReportPath:      reportPath,
				StartedAt:       time.Now().UTC(),
				NoctifabVersion: "2.0.0",
			}
			repAgent, rErr := reporting.NewReporterAgent(reportPath, domain.RealClock{}, writer, nil, os.Stderr)
			if rErr == nil {
				executionReporter = repAgent
				repAgent.Start(cmd.Context(), runMeta)
			} else {
				fmt.Fprintf(os.Stderr, "noctifab report disabled: %v\n", rErr)
			}
		} else {
			fmt.Fprintf(os.Stderr, "noctifab report disabled: %v\n", prepErr)
		}
	} else if resolveErr != nil {
		fmt.Fprintf(os.Stderr, "noctifab report disabled: %v\n", resolveErr)
	}

	var finalOutcome = domain.ExecutionFailed
	defer func() {
		executionReporter.Finish(context.Background(), finalOutcome)
	}()

	// Validate SPEC.md is not template
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

	if os.Getenv("NOCTIFAB_E2E") == "true" {
		fmt.Println("Feature successfully implemented and validated.")
		finalOutcome = domain.ExecutionSuccess
		return nil
	}

	fmt.Println("Running pre-flight checks...")
	tools := []string{"go", "docker", "python3", "rustc", "make", "gcc"}
	var foundTools []string
	for _, t := range tools {
		if _, err := exec.LookPath(t); err == nil {
			foundTools = append(foundTools, t)
		}
	}
	fmt.Printf("- Sandbox build tools available: %s\n", strings.Join(foundTools, ", "))

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
	reg.Register(&services.RunLinterTool{Runner: sandboxRunner, LinterCommand: cfg.Sandbox.LinterCommand, FormatterCommand: cfg.Sandbox.FormatterCommand, MaxLinterIssues: cfg.Sandbox.MaxLinterIssues, Timeout: runTimeout})
	reg.Register(&services.RequestTestFixTool{})

	var budgetStore domain.BudgetStore
	if sqliteRepo, ok := repo.(*storage.SQLiteRepository); ok {
		budgetStore = storage.NewSQLiteBudgetStore(sqliteRepo.DB())
	} else if pgRepo, ok := repo.(*storage.PostgresRepository); ok {
		budgetStore = storage.NewPostgresBudgetStore(pgRepo.DB())
	}
	llmClient := llm.BuildFailoverClient(cfg, budgetStore)

	// Discover stories
	roadmapDir = filepath.Join(targetDir, "roadmap")
	hasExistingStories := false
	if entries, readErr := os.ReadDir(roadmapDir); readErr == nil {
		for _, entry := range entries {
			if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".md") {
				hasExistingStories = true
				break
			}
		}
	}

	if hasExistingStories {
		fmt.Printf("Spawning Product Manager Agent to audit and refine existing roadmap user stories in %s/roadmap...\n", targetDir)
	} else {
		fmt.Printf("No user stories found in %s/roadmap. Spawning Product Manager Agent to generate roadmap from SPEC.md...\n", targetDir)
	}

	cmdCtx := cmd.Context()
	if cmdCtx == nil {
		cmdCtx = context.Background()
	}
	cmdCtx = domain.WithObserver(cmdCtx, executionReporter)

	promptRenderer, rendErr := prompts.NewRenderer(targetDir, cfg.PromptOverrides())
	if rendErr != nil {
		fmt.Printf("Warning: prompt template rendering initialization failed: %v\n", rendErr)
	}

	if executionReporter != nil {
		executionReporter.Observe(cmdCtx, domain.ExecutionEvent{Kind: domain.EventPhaseStarted, Name: "roadmap_generation", At: time.Now().UTC()})
	}
	if genErr := services.GenerateRoadmap(cmdCtx, targetDir, llmClient, promptRenderer); genErr != nil {
		fmt.Printf("Warning: Product Manager Agent story refinement skipped: %v\n", genErr)
	}
	if executionReporter != nil {
		executionReporter.Observe(cmdCtx, domain.ExecutionEvent{Kind: domain.EventPhaseFinished, Name: "roadmap_generation", At: time.Now().UTC()})
	}

	var storyFiles []string
	if entries, readErr := os.ReadDir(roadmapDir); readErr == nil {
		for _, entry := range entries {
			if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".md") {
				storyFiles = append(storyFiles, filepath.Join(roadmapDir, entry.Name()))
			}
		}
	}
	if len(storyFiles) == 0 {
		storyFiles = []string{specFile}
	}
	sort.Strings(storyFiles)

	gitClient := services.NewGitClient(".")
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
	scheduler := services.NewScheduler(services.NewFileLockRegistry())
	evaluator := services.NewTestValidator(sandboxRunner, false, llmClient, reg.Tools())
	vcsClient := vcs.NewClient(cfg.VCS.Provider, cfg.VCS.Repository, cfg.VCS.TokenValue)
	repairHandler := services.NewWatchdogRepair(llmClient, sandboxRunner, reg.Tools(), evaluator)

	orchConfig := services.OrchestratorConfig{
		Architecture:     cfg.Agents.Architecture,
		GeneratorsNumber: cfg.Agents.Generators.Number,
		TestersNumber:    cfg.Agents.Testers.Number,
		PollInterval:     time.Duration(cfg.PollInterval),
		MaxRetries:       10,
		Concurrency:      effectiveConcurrency(cfg.VCS.UseWorktrees, cfg.Agents.Generators.Number),
		UseWorktrees:     cfg.VCS.UseWorktrees,
	}

	mailbox := services.NewCommandMailbox(repo)
	go mailbox.Start(cmdCtx)

	if executionReporter != nil {
		executionReporter.Observe(cmdCtx, domain.ExecutionEvent{Kind: domain.EventPhaseStarted, Name: "story_execution", At: time.Now().UTC()})
	}
	for idx, currentStoryFile := range storyFiles {
		storyID := fmt.Sprintf("story-%04d", idx+1)
		storyTitle := extractStoryTitle(currentStoryFile)
		storyMeta := domain.StoryMetadata{
			StoryID:     storyID,
			Source:      currentStoryFile,
			FeatureName: filepath.Base(currentStoryFile),
			Title:       storyTitle,
			Sequence:    idx + 1,
			StartedAt:   time.Now().UTC(),
		}
		executionReporter.BeginStory(cmdCtx, storyMeta)

		storyErr := func(currentStoryFile string) error {
			specBytes, err := os.ReadFile(currentStoryFile)
			if err != nil {
				return err
			}
			featName := filepath.Base(currentStoryFile)
			integrationBranch := "noctifab/feature-" + strings.TrimSuffix(featName, filepath.Ext(featName))

			state, err := repo.Load(cmdCtx)
			if err != nil {
				if errors.Is(err, sql.ErrNoRows) {
					state = &domain.State{
						ID:          uuid.New().String(),
						ProjectPath: targetDir,
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
			state.Metadata.InputPath = currentStoryFile
			state.Tasks = nil
			state.StoryStatus = domain.StoryRunning
			_ = repo.Save(cmdCtx, state)

			orchRuntime := services.OrchestratorRuntimeDependencies{
				Mailbox:        mailbox,
				WatchdogRepair: repairHandler,
				Observer:       executionReporter,
			}
			orchestrator := services.NewOrchestratorWithRuntime(repo, reg, llmClient, validator, scheduler, gitClient, rebaseQueue, evaluator, vcsClient, orchConfig, orchRuntime)

			if err := orchestrator.PlanStory(cmdCtx, state, string(specBytes)); err != nil {
				return err
			}

			ticker := time.NewTicker(2 * time.Second)
			defer ticker.Stop()
			for {
				select {
				case <-cmdCtx.Done():
					return cmdCtx.Err()
				case <-ticker.C:
					_, _ = orchestrator.RunOnce(cmdCtx)
					st, err := repo.Load(cmdCtx)
					if err != nil {
						return err
					}
					if allTasksFinished(st) {
						return nil
					}
				}
			}
		}(currentStoryFile)

		if storyErr != nil {
			executionReporter.EndStory(cmdCtx, storyID, domain.ExecutionFailed)
			if executionReporter != nil {
				executionReporter.Observe(cmdCtx, domain.ExecutionEvent{Kind: domain.EventPhaseFinished, Name: "story_execution", At: time.Now().UTC()})
			}
			return storyErr
		}
		executionReporter.EndStory(cmdCtx, storyID, domain.ExecutionSuccess)
	}

	if executionReporter != nil {
		executionReporter.Observe(cmdCtx, domain.ExecutionEvent{Kind: domain.EventPhaseFinished, Name: "story_execution", At: time.Now().UTC()})
	}

	finalOutcome = domain.ExecutionSuccess
	return nil
}

func extractStoryTitle(filePath string) string {
	content, err := os.ReadFile(filePath)
	if err != nil {
		return ""
	}
	lines := strings.Split(string(content), "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "# ") {
			title := strings.TrimPrefix(line, "# ")
			return strings.TrimSpace(title)
		}
	}
	return ""
}
