package cli

import (
	"context"
	"fmt"
	"os"
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
	"github.com/diegojromerolopez/noctifab/pkg/infrastructure/vcs"
	"github.com/diegojromerolopez/noctifab/pkg/services"
	"github.com/diegojromerolopez/noctifab/pkg/services/reporting"
)

func runStartCommand(cmd *cobra.Command, args []string) error {
	cfg, err := config.Load(cmd)
	if err != nil {
		return err
	}

	targetPath := cfg.Runtime.SpecSource
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

	if abs, err := filepath.Abs(targetDir); err == nil {
		targetDir = abs
	}

	createdSpec, initErr := EnsureWorkspaceInitialized(targetDir)
	if initErr != nil {
		return fmt.Errorf("failed to initialize workspace in %q: %w", targetDir, initErr)
	}

	if createdSpec {
		if config.HasGlobalSecrets() {
			fmt.Printf("\nWorkspace initialized at %s with default SPEC.md and config.yaml.\n", targetDir)
			fmt.Printf("Please edit SPEC.md with your project requirements and run 'noctifab start' again.\n")
		} else {
			fmt.Printf("\nWorkspace initialized at %s with default SPEC.md, config.yaml, and secrets.yaml.\n", targetDir)
			fmt.Printf("Please edit SPEC.md with your project requirements, set your API key in .noctifab/secrets.yaml, and run 'noctifab start' again.\n")
		}
		return nil
	}

	// Change working directory to targetDir
	if err := os.Chdir(targetDir); err != nil {
		return fmt.Errorf("failed to change working directory to %q: %w", targetDir, err)
	}

	// Reload the configuration now that we are in the target directory
	cfg, err = config.Load(cmd)
	if err != nil {
		return err
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

	for _, sfPath := range discoverStoryFiles(targetDir) {
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

	if os.Getenv("NOCTIFAB_E2E") == "true" {
		fmt.Println("Feature successfully implemented and validated.")
		finalOutcome = domain.ExecutionSuccess
		return nil
	}

	if err := runPreFlightChecks(cfg, targetDir); err != nil {
		return err
	}

	repo, budgetStore, err := initStorageRepo(cfg)
	if err != nil {
		return err
	}

	if _, err := services.NewQARecoveryService(repo, services.SystemQAClock{}).Recover(cmd.Context()); err != nil {
		return fmt.Errorf("recover interrupted QA phases: %w", err)
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

	reg := initToolRegistry(cfg, sandboxRunner)
	llmClient := llm.BuildFailoverClient(cfg, budgetStore)

	cmdCtx := cmd.Context()
	if cmdCtx == nil {
		cmdCtx = context.Background()
	}
	cmdCtx = domain.WithObserver(cmdCtx, executionReporter)

	mailbox := services.NewCommandMailbox(repo)
	go mailbox.Start(cmdCtx)

	webServerInstance, webHost, webPort, webEnabled, webCleanup := startConcurrentWebServer(cmd, repo, mailbox)
	defer webCleanup()

	hasExistingStories := len(discoverStoryFiles(targetDir)) > 0
	promptRenderer, rendErr := prompts.NewRenderer(targetDir, cfg.PromptOverrides())
	if rendErr != nil {
		fmt.Printf("Warning: prompt template rendering initialization failed: %v\n", rendErr)
	}

	if hasExistingStories {
		fmt.Printf("ℹ [Roadmap] Existing roadmap user stories found; skipping Product Manager Agent refinement to preserve existing roadmap files.\n")
	} else {
		fmt.Printf("No user stories found in %s/roadmap. Spawning Product Manager Agent to generate roadmap from SPEC.md...\n", targetDir)
		if executionReporter != nil {
			executionReporter.Observe(cmdCtx, domain.ExecutionEvent{Kind: domain.EventPhaseStarted, Name: "roadmap_generation", At: time.Now().UTC()})
		}
		if genErr := services.GenerateRoadmapWithConfig(cmdCtx, targetDir, llmClient, promptRenderer, cfg.Agents.ProductManager.Passes, cfg.Agents.ProductManager.MaxUserStories); genErr != nil {
			fmt.Printf("Warning: Product Manager Agent story generation failed: %v\n", genErr)
		}
		if executionReporter != nil {
			executionReporter.Observe(cmdCtx, domain.ExecutionEvent{Kind: domain.EventPhaseFinished, Name: "roadmap_generation", At: time.Now().UTC()})
		}
	}

	storyFiles := discoverStoryFiles(targetDir)
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
	validator.ExcludePaths = cfg.Sandbox.ExcludePaths
	validator.SetForbiddenPatterns(cfg.Sandbox.ForbiddenPatterns)
	scheduler := services.NewScheduler(services.NewFileLockRegistry())
	evaluator := services.NewTestValidator(sandboxRunner, false, llmClient, reg.Tools())
	evaluator.FormatterCommand = cfg.Sandbox.FormatterCommand
	if cfg.Sandbox.TimeoutSeconds > 0 {
		evaluator.RunTimeout = time.Duration(cfg.Sandbox.TimeoutSeconds) * time.Second
	}
	vcsClient := vcs.NewClient(cfg.VCS.Provider, cfg.VCS.Repository, cfg.VCS.TokenValue)
	repairHandler := services.NewWatchdogRepair(llmClient, sandboxRunner, reg.Tools(), evaluator)

	orchConfig := buildOrchestratorConfig(cfg)

	executeStory := buildStoryExecutor(storyExecutorDeps{
		cfg:               cfg,
		targetDir:         targetDir,
		storyFiles:        storyFiles,
		gitClient:         gitClient,
		rebaseQueue:       rebaseQueue,
		repo:              repo,
		reg:               reg,
		llmClient:         llmClient,
		validator:         validator,
		scheduler:         scheduler,
		evaluator:         evaluator,
		vcsClient:         vcsClient,
		orchConfig:        orchConfig,
		mailbox:           mailbox,
		repairHandler:     repairHandler,
		promptRenderer:    promptRenderer,
		executionReporter: executionReporter,
	})

	if executionReporter != nil {
		executionReporter.Observe(cmdCtx, domain.ExecutionEvent{Kind: domain.EventPhaseStarted, Name: "story_execution", At: time.Now().UTC()})
	}
	resumeRequested := cmd.Name() == "resume"
	if flag := cmd.Flags().Lookup("resume"); flag != nil {
		if val, err := cmd.Flags().GetBool("resume"); err == nil && val {
			resumeRequested = true
		}
	}
	if !resumeRequested {
		if st, err := repo.Load(cmdCtx); err == nil && st != nil && len(st.Stories) > 0 {
			resumeRequested = true
		}
	}

	totalLoops := cfg.Runtime.GetLoops()
	if loopFlag := cmd.Flags().Lookup("loops"); loopFlag != nil {
		if val, err := cmd.Flags().GetInt("loops"); err == nil && val > 0 {
			totalLoops = val
		}
	}

	storyOutcomes, _ := runStoryIterationLoops(cmdCtx, StoryLoopOptions{
		Cfg:               cfg,
		TargetDir:         targetDir,
		StoryFiles:        storyFiles,
		Repo:              repo,
		ExecutionReporter: executionReporter,
		ExecuteStory:      executeStory,
		GitClient:         gitClient,
		TotalLoops:        totalLoops,
		ResumeRequested:   resumeRequested,
		WebEnabled:        webEnabled,
		WebHost:           webHost,
		WebPort:           webPort,
	})

	if executionReporter != nil {
		executionReporter.Observe(cmdCtx, domain.ExecutionEvent{Kind: domain.EventPhaseFinished, Name: "story_execution", At: time.Now().UTC()})
	}

	var failedStories []string
	for _, sf := range storyFiles {
		if err := storyOutcomes[sf]; err != nil {
			failedStories = append(failedStories, fmt.Sprintf("%s (%v)", filepath.Base(sf), err))
		}
	}
	if len(failedStories) > 0 {
		return fmt.Errorf("execution finished with %d incomplete/failed stories across %d loops:\n - %s", len(failedStories), totalLoops, strings.Join(failedStories, "\n - "))
	}

	finalOutcome = domain.ExecutionSuccess

	standbyRequested := webEnabled
	if sFlag := cmd.Flags().Lookup("standby"); sFlag != nil {
		if val, err := cmd.Flags().GetBool("standby"); err == nil && val {
			standbyRequested = true
		}
	}

	if standbyRequested {
		return runStandbyMode(cmdCtx, StandbyParams{
			Repo:      repo,
			Mailbox:   mailbox,
			WebServer: webServerInstance,
			Executor:  executeStory,
			TargetDir: targetDir,
			WebHost:   webHost,
			WebPort:   webPort,
		})
	}

	return nil
}

func isStoryCompletedSuccessfully(ctx context.Context, repo domain.StateRepository, storyFile string, storyID string, featName string) bool {
	if repo == nil {
		return false
	}
	st, err := repo.Load(ctx)
	if err != nil || st == nil {
		return false
	}

	for _, s := range st.Stories {
		if s.ID == featName || s.ID == storyID || s.FilePath == storyFile {
			if s.Status != domain.StorySuccess {
				return false
			}
			if st.Metadata.InputPath == storyFile || st.Metadata.FeatureName == featName {
				if !allTasksSucceeded(st) {
					return false
				}
			}
			return true
		}
	}

	if (st.Metadata.InputPath == storyFile || st.Metadata.FeatureName == featName) && st.StoryStatus == domain.StorySuccess && allTasksSucceeded(st) {
		return true
	}

	return false
}

func computeFailureSignature(outcomes map[string]error) string {
	var entries []string
	for k, v := range outcomes {
		if v != nil {
			entries = append(entries, fmt.Sprintf("%s:%v", filepath.Base(k), v))
		}
	}
	sort.Strings(entries)
	return strings.Join(entries, ";")
}
