package services

import (
	"context"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/diegojromerolopez/noctifab/pkg/domain"
	"github.com/diegojromerolopez/noctifab/pkg/infrastructure/prompts"
	"github.com/diegojromerolopez/noctifab/pkg/infrastructure/storage"
)

func TestOrchestratorServer_FallbackOCCRetryLoop(t *testing.T) {
	t.Run("when concurrent planners and writers mutate SQLite without mailbox, OCC reload-retry loop succeeds cleanly", func(t *testing.T) {
		ctx := context.Background()
		tmpDir, err := os.MkdirTemp("", "noctifab-sqlite-occ-test-*")
		require.NoError(t, err)
		defer func() { _ = os.RemoveAll(tmpDir) }()

		dbPath := filepath.Join(tmpDir, "test.db")
		repo, err := storage.NewSQLiteRepository(ctx, dbPath)
		require.NoError(t, err)
		defer func() { _ = repo.Close() }()

		initialState := &domain.State{
			ID:          "story-occ-retry-test",
			ProjectPath: tmpDir,
			Metadata: domain.StateMetadata{
				FeatureName: "Payment Gateway",
			},
			Tasks: []domain.Task{},
		}
		err = repo.Save(ctx, initialState)
		require.NoError(t, err)

		llm := &planMockLLM{
			completeFn: func(ctx context.Context, prompt string) (*domain.LLMResponse, error) {
				time.Sleep(20 * time.Millisecond)
				return &domain.LLMResponse{
					Actions: []domain.LLMAction{
						{
							Tool: "add_task",
							Args: map[string]any{
								"id":          "task-pay",
								"title":       "Implement Stripe Client",
								"description": "Stripe credit card processing client",
							},
						},
					},
				}, nil
			},
		}

		// Orchestrator without active mailbox (forces fallback updateStateWithRetry OCC loop)
		orchestrator := &Orchestrator{
			repo:           repo,
			llmClient:      llm,
			promptRenderer: prompts.NewDefaultRenderer(),
			cfg: OrchestratorConfig{
				OCCMaxRetries:  20,
				OCCBackoffBase: 50 * time.Millisecond,
			},
		}

		var wg sync.WaitGroup
		errs := make(chan error, 4)

		// 2 concurrent planners
		for i := 1; i <= 2; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				state, loadErr := repo.Load(ctx)
				if loadErr != nil {
					errs <- loadErr
					return
				}
				errs <- orchestrator.PlanStory(ctx, state, "Payment Spec")
			}()
		}

		// 2 concurrent writers causing intentional OCC version bumps
		for i := 1; i <= 2; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				for j := 0; j < 2; j++ {
					time.Sleep(10 * time.Millisecond)
					updateErr := orchestrator.updateStateWithRetry(ctx, func(st *domain.State) error {
						st.Metadata.TotalTokensUsed += 25
						return nil
					})
					if updateErr != nil {
						errs <- updateErr
						return
					}
				}
				errs <- nil
			}()
		}

		wg.Wait()
		close(errs)

		for err := range errs {
			assert.NoError(t, err, "fallback OCC retry loop must resolve all version conflicts without error")
		}

		finalState, err := repo.Load(ctx)
		require.NoError(t, err)
		assert.GreaterOrEqual(t, len(finalState.Tasks), 1, "planned tasks must be safely persisted")
	})
}
