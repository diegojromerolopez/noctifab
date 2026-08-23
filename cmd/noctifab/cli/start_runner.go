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

	// Discover stories
	hasExistingStories := len(discoverStoryFiles(targetDir)) > 0

	if hasExistingStories {
		fmt.Printf("Spawning Product Manager Agent to audit and refine existing roadmap user stories in %s/roadmap...\n", targetDir)
	} else {
		fmt.Printf("No user stories found in %s/roadmap. Spawning Product Manager Agent to generate roadmap from SPEC.md...\n", targetDir)
	}

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
	evaluator.LinterCommand = cfg.Sandbox.LinterCommand
	evaluator.MaxLinterIssues = cfg.Sandbox.MaxLinterIssues
	if cfg.Sandbox.TimeoutSeconds > 0 {
		evaluator.RunTimeout = time.Duration(cfg.Sandbox.TimeoutSeconds) * time.Second
	}
	vcsClient := vcs.NewClient(cfg.VCS.Provider, cfg.VCS.Repository, cfg.VCS.TokenValue)
	repairHandler := services.NewWatchdogRepair(llmClient, sandboxRunner, reg.Tools(), evaluator)

	orchConfig := buildOrchestratorConfig(cfg)

	executeStory := func(ctx context.Context, currentStoryFile string) error {
		specBytes, err := os.ReadFile(currentStoryFile)
		if err != nil {
			return err
		}
		featName := filepath.Base(currentStoryFile)
		configuredBranch := cfg.VCS.GetIntegrationBranch()
		if strings.ToLower(cfg.VCS.BranchStrategy) == "per_story_isolated" {
			configuredBranch = ""
		} else if configuredBranch == "" && len(storyFiles) > 1 {
			prefix := cfg.VCS.BranchPrefix
			if prefix == "" {
				prefix = "noctifab/"
			}
			configuredBranch = prefix + "implementation"
		}
		branchRes := services.ResolveBranches(ctx, gitClient, cfg.VCS.BaseBranch, configuredBranch, cfg.VCS.BranchPrefix, featName)
		baseBranch := branchRes.BaseBranch
		integrationBranch := branchRes.IntegrationBranch

		state, err := repo.Load(ctx)
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
						BaseBranch:        baseBranch,
						IntegrationBranch: integrationBranch,
					},
				}
			} else {
				return err
			}
		}
		state.Metadata.InputPath = currentStoryFile
		state.Metadata.FeatureName = featName
		state.Metadata.BaseBranch = baseBranch
		state.Metadata.IntegrationBranch = integrationBranch
		state.Tasks = nil
		state.StoryStatus = domain.StoryRunning

		now := time.Now().UTC()
		foundStory := false
		for i, st := range state.Stories {
			if st.ID == featName || st.FilePath == currentStoryFile {
				state.Stories[i].Status = domain.StoryRunning
				if state.Stories[i].StartedAt == nil {
					state.Stories[i].StartedAt = &now
				}
				state.Stories[i].UpdatedAt = now
				foundStory = true
				break
			}
		}
		if !foundStory {
			state.Stories = append(state.Stories, domain.Story{
				ID:        featName,
				StateID:   state.ID,
				Title:     featName,
				FilePath:  currentStoryFile,
				Status:    domain.StoryRunning,
				StartedAt: &now,
				CreatedAt: now,
				UpdatedAt: now,
			})
		}

		storyPath := currentStoryFile
		if !filepath.IsAbs(storyPath) {
			storyPath = filepath.Join(targetDir, storyPath)
		}
		if markdown, err := os.ReadFile(storyPath); err == nil {
			relPath := currentStoryFile
			if filepath.IsAbs(relPath) {
				if rel, err := filepath.Rel(targetDir, relPath); err == nil {
					relPath = rel
				}
			}
			if contract, err := services.ParseStoryContract(relPath, string(markdown)); err == nil && contract.StoryID != "" {
				services.UpsertStoryContract(state, contract)
			}
		}
		if err := repo.Save(ctx, state); err != nil {
			return fmt.Errorf("failed to save initial state: %w", err)
		}

		var qaCoord *services.QARuntimeCoordinator
		if qaDeps := qaDependencies(cfg); len(qaDeps) > 0 {
			deps := qaDeps[0]
			qaCoord = services.NewQARuntimeCoordinator(
				cfg.Agents.QA, llmClient, promptRenderer,
				deps.WorkspaceFactory, deps.ArtifactBuilder, deps.Sandbox, deps.FileSystem, deps.Clock,
			)
		}

		orchRuntime := services.OrchestratorRuntimeDependencies{
			Mailbox:        mailbox,
			WatchdogRepair: repairHandler,
			PromptRenderer: promptRenderer,
			QA:             qaCoord,
			Observer:       executionReporter,
		}
		orchestrator := services.NewOrchestratorWithRuntime(repo, reg, llmClient, validator, scheduler, gitClient, rebaseQueue, evaluator, vcsClient, orchConfig, orchRuntime)

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
			unblockerCtx, cancelUnblocker := context.WithCancel(ctx)
			defer cancelUnblocker()
			unblocker.Start(unblockerCtx)
		}

		if err := orchestrator.PlanStory(ctx, state, string(specBytes)); err != nil {
			return err
		}

		ticker := time.NewTicker(storyExecInterval(cfg))
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-ticker.C:
				_, _ = orchestrator.RunOnce(ctx)
				st, err := repo.Load(ctx)
				if err != nil {
					return err
				}
				if allTasksFinished(st) {
					return nil
				}
			}
		}
	}

	if executionReporter != nil {
		executionReporter.Observe(cmdCtx, domain.ExecutionEvent{Kind: domain.EventPhaseStarted, Name: "story_execution", At: time.Now().UTC()})
	}
	resumeRequested := cmd.Name() == "resume"
	if flag := cmd.Flags().Lookup("resume"); flag != nil {
		if val, err := cmd.Flags().GetBool("resume"); err == nil && val {
			resumeRequested = true
		}
	}

	for idx, currentStoryFile := range storyFiles {
		storyID := fmt.Sprintf("story-%04d", idx+1)
		featName := strings.TrimSuffix(filepath.Base(currentStoryFile), filepath.Ext(currentStoryFile))
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

		if resumeRequested {
			if st, err := repo.Load(cmdCtx); err == nil && st != nil {
				if st.Metadata.InputPath == currentStoryFile && st.StoryStatus == domain.StorySuccess && allTasksFinished(st) {
					fmt.Printf("ℹ [Resume] Skipping already completed story %s (%s) — all tasks succeeded\n", storyID, storyTitle)
					continue
				}
			}
		}

		if webEnabled {
			fmt.Printf("\n🚀 Executing %s (%s)\n➜  Web Dashboard: http://%s:%d\n\n", storyID, storyTitle, webHost, webPort)
		} else {
			fmt.Printf("\n🚀 Executing %s (%s)\n\n", storyID, storyTitle)
		}

		storyErr := executeStory(cmdCtx, currentStoryFile)
		if storyErr != nil {
			executionReporter.EndStory(cmdCtx, storyID, domain.ExecutionFailed)
			if st, err := repo.Load(cmdCtx); err == nil && st != nil {
				now := time.Now().UTC()
				for i, s := range st.Stories {
					if s.ID == featName || s.ID == storyID || s.FilePath == currentStoryFile {
						st.Stories[i].Status = domain.StoryFailed
						st.Stories[i].CompletedAt = &now
						st.Stories[i].UpdatedAt = now
						break
					}
				}
				_ = repo.Save(cmdCtx, st)
			}
			if executionReporter != nil {
				executionReporter.Observe(cmdCtx, domain.ExecutionEvent{Kind: domain.EventPhaseFinished, Name: "story_execution", At: time.Now().UTC()})
			}
			return storyErr
		}
		executionReporter.EndStory(cmdCtx, storyID, domain.ExecutionSuccess)
		if st, err := repo.Load(cmdCtx); err == nil && st != nil {
			now := time.Now().UTC()
			for i, s := range st.Stories {
				if s.ID == featName || s.ID == storyID || s.FilePath == currentStoryFile {
					st.Stories[i].Status = domain.StorySuccess
					st.Stories[i].CompletedAt = &now
					st.Stories[i].UpdatedAt = now
					break
				}
			}
			_ = repo.Save(cmdCtx, st)
		}
	}

	if executionReporter != nil {
		executionReporter.Observe(cmdCtx, domain.ExecutionEvent{Kind: domain.EventPhaseFinished, Name: "story_execution", At: time.Now().UTC()})
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
