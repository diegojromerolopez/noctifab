package cli

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/diegojromerolopez/noctifab/pkg/domain"
	"github.com/diegojromerolopez/noctifab/pkg/infrastructure/config"
	"github.com/diegojromerolopez/noctifab/pkg/infrastructure/llm"
	"github.com/diegojromerolopez/noctifab/pkg/infrastructure/prompts"
	"github.com/diegojromerolopez/noctifab/pkg/infrastructure/storage"
	"github.com/diegojromerolopez/noctifab/pkg/infrastructure/telemetry"
	"github.com/diegojromerolopez/noctifab/pkg/infrastructure/vcs"
	"github.com/diegojromerolopez/noctifab/pkg/services"
	"github.com/spf13/cobra"
)

// serveCmd is a hidden subcommand that runs the headless orchestrator daemon.
// It is launched by the "start" command as a background child process and should
// not be invoked directly by users.
var serveCmd = &cobra.Command{
	Use:           "serve",
	Short:         "Run the noctifab daemon server (internal — use 'start' instead)",
	Hidden:        true,
	SilenceErrors: true,
	SilenceUsage:  true,
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := config.Load(cmd)
		if err != nil {
			return err
		}

		if cfg.Telemetry.Enabled {
			tp, tpErr := telemetry.InitTracer(cfg.Telemetry.ServiceName, cfg.Telemetry.Endpoint)
			if tpErr != nil {
				fmt.Fprintf(os.Stderr, "Warning: telemetry init failed: %v\n", tpErr)
			} else {
				defer func() { _ = tp.Shutdown(context.Background()) }()
			}
		}

		// Write PID file so "stop" and "start" can find this process.
		pidPath := ".noctifab/noctifab.pid"
		if err := services.WritePIDFile(pidPath); err != nil {
			return fmt.Errorf("failed to write PID file: %w", err)
		}
		defer func() { _ = services.RemovePIDFile(pidPath) }()

		// Initialize repository.
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
		if _, err := services.NewQARecoveryService(repo, services.SystemQAClock{}).Recover(cmd.Context()); err != nil {
			return fmt.Errorf("recover interrupted QA phases: %w", err)
		}

		// Storage retention: prune terminal story states beyond the
		// configured keep-last bound so the daemon's DB does not grow
		// monotonically across stories.
		pruneRetainedStates(context.Background(), repo, cfg.Storage.KeepFinishedStates)

		// Initialize sandbox runner.
		var sandboxRunner services.Sandbox
		var depMgr *services.DependencyManager
		if cfg.Sandbox.AutoInstallDeps || cfg.Sandbox.Mode != "docker" {
			depMgr = services.NewDependencyManager(cfg.Sandbox.PackageManagers)
		}
		if cfg.Sandbox.Mode == "docker" {
			sandboxRunner = services.NewDockerSandbox("noctifab-sandbox")
		} else {
			sandboxRunner = services.NewHostSandbox(cfg.Sandbox.AllowedCommands, cfg.Sandbox.TestCommand, time.Duration(cfg.Sandbox.IdleTimeoutSeconds)*time.Second, depMgr)
		}

		// Initialize tool registry.
		reg := services.NewToolRegistry()
		reg.Register(&services.AddTaskTool{})
		reg.Register(&services.CompleteTaskTool{})
		reg.Register(&services.LogMessageTool{})
		reg.Register(&services.NoopTool{})
		reg.Register(&services.ReadFileTool{})
		syntaxChecker := services.NewCommandSyntaxChecker(cfg.Sandbox.SyntaxCheckCommand)
		reg.Register(&services.WriteFileTool{SyntaxChecker: syntaxChecker})
		reg.Register(&services.WriteFilesTool{SyntaxChecker: syntaxChecker})
		reg.Register(&services.DeleteFileTool{})
		reg.Register(&services.EditFileTool{SyntaxChecker: syntaxChecker})
		reg.Register(&services.ApplyPatchTool{SyntaxChecker: syntaxChecker})
		reg.Register(&services.ListDirectoryTool{ExcludePaths: cfg.Sandbox.ExcludePaths})
		reg.Register(&services.FindFilesTool{ExcludePaths: cfg.Sandbox.ExcludePaths})
		reg.Register(&services.GrepSearchTool{ExcludePaths: cfg.Sandbox.ExcludePaths})
		runTimeout := 5 * time.Minute
		if cfg.Sandbox.TimeoutSeconds > 0 {
			runTimeout = time.Duration(cfg.Sandbox.TimeoutSeconds) * time.Second
		}
		reg.Register(&services.RunTestsTool{Runner: sandboxRunner, Timeout: runTimeout})
		reg.Register(&services.RunLinterTool{Runner: sandboxRunner, LinterCommand: cfg.Sandbox.GetLinterCommand(), FormatterCommand: cfg.Sandbox.FormatterCommand, MaxLinterIssues: cfg.Sandbox.GetMaxLinterIssues(), Timeout: runTimeout})
		reg.Register(&services.RequestTestFixTool{})
		reg.Register(&services.InstallPackageTool{DepMgr: depMgr, Runner: sandboxRunner})

		// Initialize LLM client with database budget store.
		var budgetStore domain.BudgetStore
		if sqliteRepo, ok := repo.(*storage.SQLiteRepository); ok {
			budgetStore = storage.NewSQLiteBudgetStore(sqliteRepo.DB())
		} else if pgRepo, ok := repo.(*storage.PostgresRepository); ok {
			budgetStore = storage.NewPostgresBudgetStore(pgRepo.DB())
		}
		llmClient := llm.BuildFailoverClient(cfg, budgetStore)

		// Initialize orchestrator components.
		gitClient := services.NewGitClient(".")
		if cfg.VCS.BaseBranch == "git-detect" {
			detected, err := gitClient.Run(context.Background(), false, "rev-parse", "--abbrev-ref", "HEAD")
			if err == nil {
				cfg.VCS.BaseBranch = strings.TrimSpace(detected)
			} else {
				cfg.VCS.BaseBranch = "main" // fallback
			}
		}
		rebaseQueue := services.NewRebaseQueue(gitClient)
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

		// Build the prompt renderer: config path overrides > workspace
		// convention files (.noctifab/prompts/) > embedded defaults. A broken
		// override aborts startup here with a file-named error (fail fast).
		promptRenderer, rendErr := prompts.NewRenderer(".", cfg.PromptOverrides())
		if rendErr != nil {
			return fmt.Errorf("invalid prompt templates: %w", rendErr)
		}

		orchConfig := services.OrchestratorConfig{
			Architecture:         cfg.Agents.Architecture,
			TaskExecutionOrder:   cfg.Agents.TaskExecutionOrder,
			GeneratorsNumber:     cfg.Agents.Generators.Number,
			GeneratorsIterations: cfg.Agents.Generators.Iterations,
			TestersNumber:        cfg.Agents.Testers.Number,
			TestersIterations:    cfg.Agents.Testers.Iterations,
			PollInterval:         time.Duration(cfg.PollInterval),
			MaxRetries:           10,
			Concurrency:          effectiveConcurrency(cfg.VCS.UseWorktrees, cfg.Agents.Generators.Number),
			UseWorktrees:         cfg.VCS.UseWorktrees,
			OCCMaxRetries:        cfg.Storage.OCC.MaxRetries,
			OCCBackoffBase:       time.Duration(cfg.Storage.OCC.BackoffBase),
			OCCBackoffFactor:     cfg.Storage.OCC.BackoffFactor,
			MaxDuration:          time.Duration(cfg.Runtime.MaxDuration),
			MaxActions:           cfg.Runtime.MaxActions,
			AutoCreatePR:         cfg.VCS.PullRequest.AutoCreate,
			ExcludePaths:         cfg.Sandbox.ExcludePaths,
			WorkspaceCache:       cfg.GetWorkspaceCache(),
			QA:                   cfg.Agents.QA,
			Fallback:             cfg.Agents.GetFallback(),
			LastResort:           cfg.Agents.LastResort,
		}

		// Story queue: the mailbox sends stories here; the server loop processes them.
		storyCh := make(chan services.StoryWorkItem, 32)
		mailbox := services.NewCommandMailbox(repo)

		orchestrator := services.NewOrchestrator(
			repo, reg, llmClient, validator, scheduler,
			gitClient, rebaseQueue, evaluator, vcsClient, orchConfig, mailbox, repairHandler,
			promptRenderer, qaDependencies(cfg)...,
		)

		ctx, cancel := context.WithCancel(context.Background())

		fallbackCfg := cfg.GetFallback()
		if fallbackCfg.Enabled {
			fallbackAgent := services.NewFallbackAgent(
				repo,
				llmClient,
				mailbox,
				time.Duration(fallbackCfg.PollInterval),
				fallbackCfg.MaxRetries,
				time.Duration(fallbackCfg.StallThreshold),
				time.Duration(fallbackCfg.ConflictThreshold),
				fallbackCfg.LLMAssessment,
			)
			fallbackAgent.SetBudgetCliff(fallbackCfg.BudgetCliffRatio, 0)
			fallbackAgent.SetStallCountThreshold(fallbackCfg.Triggers.StallCountThreshold)
			orchestrator.SetFallbackAgent(fallbackAgent)
			fallbackAgent.Start(ctx)
		}

		// Graceful shutdown on SIGTERM / SIGINT.
		sigCh := make(chan os.Signal, 1)
		signal.Notify(sigCh, syscall.SIGTERM, syscall.SIGINT)
		go func() {
			<-sigCh
			fmt.Fprintln(os.Stderr, "noctifab daemon: received shutdown signal, saving state...")
			cancel()
		}()

		go rebaseQueue.Start(ctx)
		go mailbox.Start(ctx)

		// Start REST HTTP server (loopback only, passes storyCh for /api/v1/stories).
		server := services.StartDaemonServer(repo, mailbox, storyCh, llmClient, promptRenderer)
		defer func() { _ = server.Close() }()

		fmt.Printf("noctifab daemon started (PID %d). Listening on 127.0.0.1:18080\n", os.Getpid())

		// Server mode: consume stories from the channel and execute each one.
		storyWorkers := cfg.Agents.Generators.Number
		if storyWorkers <= 0 {
			storyWorkers = 4
		}
		return runServerLoop(ctx, orchestrator, repo, storyCh, cfg.VCS.BaseBranch, cfg.VCS.BranchPrefix, storyExecInterval(cfg), storyWorkers)
	},
}

// runServerLoop processes user stories from the queue concurrently using StoryDAGScheduler.
// When stories are enqueued in rapid succession (e.g. from batch or directory submissions),
// a 100ms drain window groups burst-enqueued stories into a single batch for concurrent DAG execution
// across up to maxStoryWorkers parallel slots.
func runServerLoop(
	ctx context.Context,
	orchestrator *services.Orchestrator,
	repo domain.StateRepository,
	storyCh <-chan services.StoryWorkItem,
	baseBranch, branchPrefix string,
	execInterval time.Duration,
	maxStoryWorkers ...int,
) error {
	workers := 4
	if len(maxStoryWorkers) > 0 && maxStoryWorkers[0] > 0 {
		workers = maxStoryWorkers[0]
	}
	cwd, err := os.Getwd()
	if err != nil {
		cwd = "."
	}

	for {
		select {
		case <-ctx.Done():
			// Graceful shutdown: mark any in-progress tasks as INTERRUPTED.
			shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer shutdownCancel()
			interruptCmd := &services.MarkStoryInterruptedCmd{}
			_ = interruptCmd.Execute(shutdownCtx, repo)
			fmt.Fprintln(os.Stderr, "noctifab daemon: state saved. Goodbye.")
			return nil

		case item, ok := <-storyCh:
			if !ok {
				return nil
			}

			// Collect all story items currently buffered in storyCh (e.g. from directory command)
			items := []services.StoryWorkItem{item}
			drainTimer := time.NewTimer(100 * time.Millisecond)
			draining := true
			for draining {
				select {
				case nextItem, open := <-storyCh:
					if open {
						items = append(items, nextItem)
					} else {
						draining = false
					}
				case <-drainTimer.C:
					draining = false
				}
			}
			drainTimer.Stop()

			if len(items) == 1 {
				if err := processStory(ctx, orchestrator, repo, items[0], cwd, baseBranch, branchPrefix, execInterval); err != nil {
					fmt.Fprintf(os.Stderr, "noctifab daemon: story %s failed: %v\n", items[0].Path, err)
				}
			} else {
				scheduler := services.NewStoryDAGScheduler(workers)
				for _, it := range items {
					scheduler.AddStory(it)
				}

				if err := scheduler.Execute(ctx, func(storyCtx context.Context, it services.StoryWorkItem) error {
					return processStory(storyCtx, orchestrator, repo, it, cwd, baseBranch, branchPrefix, execInterval)
				}); err != nil {
					fmt.Fprintf(os.Stderr, "noctifab daemon: story DAG execution finished with error: %v\n", err)
				}
			}
		}
	}
}

// processStory runs the full plan→execute→finalize cycle for one user story.
// All orchestrator output is written to both stdout and the per-story log file.
func processStory(
	ctx context.Context,
	orchestrator *services.Orchestrator,
	repo domain.StateRepository,
	item services.StoryWorkItem,
	projectPath, baseBranch, branchPrefix string,
	execInterval time.Duration,
) error {
	// Open per-story log file (.noctifab/logs/roadmap/<story>.log).
	logFile, err := os.OpenFile(item.LogPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		fmt.Fprintf(os.Stderr, "⚠ Could not open log file %s: %v (continuing without file log)\n", item.LogPath, err)
	} else {
		defer func() { _ = logFile.Close() }()
		_, _ = fmt.Fprintf(logFile, "=== Story started: %s at %s ===\n", item.Path, time.Now().Format(time.RFC3339))
	}

	logf := func(format string, args ...any) {
		msg := fmt.Sprintf(format, args...)
		fmt.Print(msg)
		if logFile != nil {
			_, _ = fmt.Fprint(logFile, msg)
		}
	}

	logf("▶ Starting story: %s\n", item.Path)

	// Create a fresh State for this story.
	state := services.NewStateForStory(projectPath, item.Path, baseBranch, branchPrefix)
	state.StoryStatus = domain.StoryRunning
	if err := repo.Save(ctx, state); err != nil {
		return fmt.Errorf("failed to save initial state: %w", err)
	}

	// Planning phase: decompose the spec into tasks.
	logf("📋 Planning tasks from specification...\n")
	if err := orchestrator.PlanStory(ctx, state, item.Spec); err != nil {
		state.StoryStatus = domain.StoryFailed
		state.StoryError = fmt.Sprintf("planning failed: %v", err)
		_ = repo.Save(context.Background(), state)
		return fmt.Errorf("planning failed: %w", err)
	}

	// Execution loop: run until all tasks are done or context is cancelled.
	// Uses story_exec_interval (default 2s) for fast cycles during story
	// execution (the coarser poll_interval is used server-wide).
	ticker := time.NewTicker(execInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			current, loadErr := repo.Load(ctx)
			if loadErr != nil {
				logf("⚠ Load error: %v\n", loadErr)
				continue
			}

			if current.StoryStatus == domain.StoryPaused {
				continue
			}

			if current.StoryStatus == domain.StoryCancelled {
				logf("❌ Story %s: execution cancelled by user.\n", item.Path)
				// Revert running tasks to interrupted status
				for i := range current.Tasks {
					if current.Tasks[i].Status == domain.TaskInProgress || current.Tasks[i].Status == domain.TaskPending {
						current.Tasks[i].Status = domain.TaskInterrupted
						current.Tasks[i].UpdatedAt = time.Now()
					}
				}
				current.StoryError = "cancelled by user"
				_ = repo.Save(ctx, current)

				// Checkout back to base integration branch
				gitClient := services.NewGitClient(current.ProjectPath)
				_, _ = gitClient.Run(ctx, true, "checkout", baseBranch)

				if logFile != nil {
					_, _ = fmt.Fprintf(logFile, "=== Story CANCELLED: %s at %s ===\n", item.Path, time.Now().Format(time.RFC3339))
				}
				return fmt.Errorf("story %s: cancelled by user", item.Path)
			}

			if _, err := orchestrator.RunOnce(ctx); err != nil {
				logf("⚠ Orchestrator error: %v\n", err)
			}

			// Reload state to check completion status
			current, loadErr = repo.Load(ctx)
			if loadErr != nil {
				return loadErr
			}

			if allTasksDone(current) {
				logf("✅ All tasks finished for story: %s\n", item.Path)
				finalErr := orchestrator.FinalizeUserStory(ctx, current)
				current.StoryStatus = domain.StorySuccess
				if finalErr != nil {
					current.StoryStatus = domain.StoryFailed
					current.StoryError = finalErr.Error()
				}
				_ = repo.Save(ctx, current)
				if logFile != nil {
					_, _ = fmt.Fprintf(logFile, "=== Story finished: %s at %s ===\n", item.Path, time.Now().Format(time.RFC3339))
				}
				return finalErr
			}

			if anyTaskPermanentlyFailed(current) {
				current.StoryStatus = domain.StoryFailed
				current.StoryError = fmt.Sprintf("story %s: one or more tasks failed permanently", item.Path)
				_ = repo.Save(ctx, current)
				logf("❌ Story %s has permanently failed tasks.\n", item.Path)
				if logFile != nil {
					_, _ = fmt.Fprintf(logFile, "=== Story FAILED: %s at %s ===\n", item.Path, time.Now().Format(time.RFC3339))
				}
				return fmt.Errorf("story %s: one or more tasks failed permanently", item.Path)
			}
		}
	}
}

func allTasksDone(state *domain.State) bool {
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

func anyTaskPermanentlyFailed(state *domain.State) bool {
	for _, t := range state.Tasks {
		if t.Status == domain.TaskFailed {
			return true
		}
	}
	return false
}

func init() {
	RootCmd.AddCommand(serveCmd)
}
