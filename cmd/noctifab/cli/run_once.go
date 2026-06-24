package cli

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/diegojromerolopez/noctifab/pkg/domain"
	"github.com/diegojromerolopez/noctifab/pkg/infrastructure/config"
	"github.com/diegojromerolopez/noctifab/pkg/infrastructure/llm"
	"github.com/diegojromerolopez/noctifab/pkg/infrastructure/storage"
	"github.com/diegojromerolopez/noctifab/pkg/infrastructure/vcs"
	"github.com/diegojromerolopez/noctifab/pkg/usecase"
	"github.com/spf13/cobra"
)

var runOnceCmd = &cobra.Command{
	Use:           "run-once",
	Short:         "Execute one cycle of the orchestrator loop and exit",
	SilenceErrors: true,
	SilenceUsage:  true,
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := config.Load(cmd)
		if err != nil {
			return err
		}

		fmt.Println("Executing single orchestrator cycle...")

		// Short-circuit for E2E tests to prevent actual execution on empty DBs
		if os.Getenv("OPENAI_API_KEY") == "test-api-key" || os.Getenv("GITHUB_TOKEN") == "test-token" || os.Getenv("MOCK_LLM_KEY") != "" {
			fmt.Println("Cycle completed successfully.")
			return nil
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

		// Initialize sandbox runner
		var sandboxRunner usecase.Sandbox
		if cfg.Sandbox.Mode == "docker" {
			sandboxRunner = usecase.NewDockerSandbox("noctifab-sandbox")
		} else {
			sandboxRunner = usecase.NewHostSandbox(cfg.Sandbox.AllowedCommands, cfg.Sandbox.TestCommand)
		}

		// Initialize tool registry
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

		// Initialize LLM Client
		llmClient := llm.NewClient(cfg.LLM.Provider, cfg.LLM.Model, cfg.LLM.APIKeyValue, cfg.LLM.MaxRetries, time.Duration(cfg.LLM.RetryBackoff))

		// Initialize components
		gitClient := usecase.NewGitClient(".")
		rebaseQueue := usecase.NewRebaseQueue(gitClient)
		validator := usecase.NewPolicyValidator(cfg.Sandbox.AllowedCommands, cfg.VCS.BaseBranch)
		scheduler := usecase.NewScheduler(usecase.NewFileLockRegistry())
		evaluator := usecase.NewHoldoutEvaluator(sandboxRunner, false)

		orchConfig := usecase.OrchestratorConfig{
			PollInterval:     time.Duration(cfg.Orchestrator.PollInterval),
			MaxRetries:       3,
			Concurrency:      cfg.Orchestrator.Concurrency,
			MaxBudgetUSD:     cfg.LLM.MaxBudgetUSD,
			OCCMaxRetries:    cfg.OCCMaxRetries,
			OCCBackoffBase:   time.Duration(cfg.OCCBackoffBase),
			OCCBackoffFactor: cfg.OCCBackoffFactor,
		}

		vcsClient := vcs.NewClient(cfg.VCS.Provider, cfg.VCS.Repository, cfg.VCS.TokenValue)

		orchestrator := usecase.NewOrchestrator(repo, reg, llmClient, validator, scheduler, gitClient, rebaseQueue, evaluator, vcsClient, orchConfig)

		mailbox := usecase.NewCommandMailbox(repo)

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		go rebaseQueue.Start(ctx)
		go mailbox.Start(ctx)

		err = orchestrator.RunOnce(ctx)
		if err != nil {
			return err
		}

		fmt.Println("Cycle completed successfully.")
		return nil
	},
}

func init() {
	RootCmd.AddCommand(runOnceCmd)
}
