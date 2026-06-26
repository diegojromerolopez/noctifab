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
	"github.com/diegojromerolopez/noctifab/pkg/infrastructure/storage"
	"github.com/diegojromerolopez/noctifab/pkg/infrastructure/vcs"
	"github.com/diegojromerolopez/noctifab/pkg/usecase"
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

		// Write PID file so "stop" and "start" can find this process.
		pidPath := ".noctifab/noctifab.pid"
		if err := usecase.WritePIDFile(pidPath); err != nil {
			return fmt.Errorf("failed to write PID file: %w", err)
		}
		defer func() { _ = usecase.RemovePIDFile(pidPath) }()

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

		// Initialize sandbox runner.
		var sandboxRunner usecase.Sandbox
		if cfg.Sandbox.Mode == "docker" {
			sandboxRunner = usecase.NewDockerSandbox("noctifab-sandbox")
		} else {
			sandboxRunner = usecase.NewHostSandbox(cfg.Sandbox.AllowedCommands, cfg.Sandbox.TestCommand)
		}

		// Initialize tool registry.
		reg := usecase.NewToolRegistry()
		reg.Register(&usecase.AddTaskTool{})
		reg.Register(&usecase.CompleteTaskTool{})
		reg.Register(&usecase.LogMessageTool{})
		reg.Register(&usecase.NoopTool{})
		reg.Register(&usecase.ReadFileTool{})
		reg.Register(&usecase.WriteFileTool{})
		reg.Register(&usecase.EditFileTool{})
		reg.Register(&usecase.ListDirectoryTool{})
		reg.Register(&usecase.FindFilesTool{})
		reg.Register(&usecase.GrepSearchTool{})
		reg.Register(&usecase.RunTestsTool{Runner: sandboxRunner})

		// Initialize LLM client.
		llmClient := llm.NewClient(
			cfg.LLM.Provider, cfg.LLM.Model, cfg.LLM.APIKeyValue,
			cfg.LLM.MaxRetries, time.Duration(cfg.LLM.RetryBackoff),
		)

		// Initialize orchestrator components.
		gitClient := usecase.NewGitClient(".")
		rebaseQueue := usecase.NewRebaseQueue(gitClient)
		validator := usecase.NewPolicyValidator(cfg.Sandbox.AllowedCommands, cfg.VCS.BaseBranch)
		scheduler := usecase.NewScheduler(usecase.NewFileLockRegistry())
		evaluator := usecase.NewTestValidator(sandboxRunner, false)
		vcsClient := vcs.NewClient(cfg.VCS.Provider, cfg.VCS.Repository, cfg.VCS.TokenValue)

		orchConfig := usecase.OrchestratorConfig{
			PollInterval:     time.Duration(cfg.Orchestrator.PollInterval),
			MaxRetries:       3,
			Concurrency:      cfg.Orchestrator.Concurrency,
			MaxBudgetUSD:     cfg.LLM.MaxBudgetUSD,
			OCCMaxRetries:    cfg.OCCMaxRetries,
			OCCBackoffBase:   time.Duration(cfg.OCCBackoffBase),
			OCCBackoffFactor: cfg.OCCBackoffFactor,
		}

		orchestrator := usecase.NewOrchestrator(
			repo, reg, llmClient, validator, scheduler,
			gitClient, rebaseQueue, evaluator, vcsClient, orchConfig,
		)

		// Story queue: the mailbox sends stories here; the server loop processes them.
		storyCh := make(chan usecase.StoryWorkItem, 32)
		mailbox := usecase.NewCommandMailbox(repo)

		ctx, cancel := context.WithCancel(context.Background())

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
		server := usecase.StartDaemonServer(repo, mailbox, storyCh)
		defer func() { _ = server.Close() }()

		fmt.Printf("noctifab daemon started (PID %d). Listening on 127.0.0.1:18080\n", os.Getpid())

		// Server mode: consume stories from the channel and execute each one.
		return runServerLoop(ctx, orchestrator, repo, storyCh, cfg.VCS.BaseBranch, cfg.VCS.BranchPrefix)
	},
}

// runServerLoop processes user stories from the queue one at a time.
// For each story it creates a fresh State, runs the full orchestration loop, finalizes with a PR,
// then writes a per-story completion entry and loops back.
func runServerLoop(
	ctx context.Context,
	orchestrator *usecase.Orchestrator,
	repo domain.StateRepository,
	storyCh <-chan usecase.StoryWorkItem,
	baseBranch, branchPrefix string,
) error {
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
			interruptCmd := &usecase.MarkStoryInterruptedCmd{}
			_ = interruptCmd.Execute(shutdownCtx, repo)
			fmt.Fprintln(os.Stderr, "noctifab daemon: state saved. Goodbye.")
			return nil

		case item := <-storyCh:
			if err := processStory(ctx, orchestrator, repo, item, cwd, baseBranch, branchPrefix); err != nil {
				fmt.Fprintf(os.Stderr, "noctifab daemon: story %s failed: %v\n", item.Path, err)
			}
		}
	}
}

// processStory runs the full plan→execute→finalize cycle for one user story.
// All orchestrator output is written to both stdout and the per-story log file.
func processStory(
	ctx context.Context,
	orchestrator *usecase.Orchestrator,
	repo domain.StateRepository,
	item usecase.StoryWorkItem,
	projectPath, baseBranch, branchPrefix string,
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
	state := usecase.NewStateForStory(projectPath, item.Path, baseBranch, branchPrefix)
	if err := repo.Save(ctx, state); err != nil {
		return fmt.Errorf("failed to save initial state: %w", err)
	}

	// Planning phase: decompose the spec into tasks.
	logf("📋 Planning tasks from specification...\n")
	if err := orchestrator.PlanStory(ctx, state, item.Spec); err != nil {
		return fmt.Errorf("planning failed: %w", err)
	}

	// Execution loop: run until all tasks are done or context is cancelled.
	ticker := time.NewTicker(orchestrator.PollInterval())
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			if err := orchestrator.RunOnce(ctx); err != nil {
				logf("⚠ Orchestrator error: %v\n", err)
			}

			current, loadErr := repo.Load(ctx)
			if loadErr != nil {
				return loadErr
			}

			if allTasksDone(current) {
				logf("✅ All tasks finished for story: %s\n", item.Path)
				finalErr := orchestrator.FinalizeUserStory(ctx, current)
				if logFile != nil {
					_, _ = fmt.Fprintf(logFile, "=== Story finished: %s at %s ===\n", item.Path, time.Now().Format(time.RFC3339))
				}
				return finalErr
			}

			if anyTaskPermanentlyFailed(current) {
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
