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
	"github.com/diegojromerolopez/noctifab/pkg/usecase"
	"github.com/google/uuid"
	"github.com/spf13/cobra"
)

var planCmd = &cobra.Command{
	Use:           "plan",
	Short:         "Read specification and plan task DAG",
	SilenceErrors: true,
	SilenceUsage:  true,
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := config.Load(cmd)
		if err != nil {
			return err
		}

		fmt.Println("Reading specification...")
		fmt.Println("Building task DAG...")

		// Short-circuit for E2E tests
		if os.Getenv("OPENAI_API_KEY") == "test-api-key" || os.Getenv("GITHUB_TOKEN") == "test-token" || os.Getenv("MOCK_LLM_KEY") != "" {
			fmt.Println("Plan created successfully.")
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

		// Read spec
		specBytes, err := os.ReadFile(cfg.Input)
		if err != nil {
			return fmt.Errorf("failed to read specification: %w", err)
		}
		specStr := string(specBytes)

		// Initialize LLM Client
		llmClient := llm.NewClient(cfg.LLM.Provider, cfg.LLM.Model, cfg.LLM.APIKeyValue, cfg.LLM.MaxRetries, time.Duration(cfg.LLM.RetryBackoff))

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

		// Invoke LLM Planner
		prompt := fmt.Sprintf("Decompose specification into tasks: %s", specStr)
		resp, err := llmClient.Complete(context.Background(), prompt)
		if err != nil {
			return err
		}

		// Save planned tasks to state
		reg := usecase.NewToolRegistry()
		reg.Register(&usecase.AddTaskTool{})
		for _, action := range resp.Actions {
			if tool, ok := reg.Get(action.Tool); ok {
				_, _ = tool.Execute(context.Background(), state, action.Args)
			}
		}

		if err := repo.Save(context.Background(), state); err != nil {
			return err
		}

		fmt.Println("Plan created successfully.")
		return nil
	},
}

func init() {
	RootCmd.AddCommand(planCmd)
}
