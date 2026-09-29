package services

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/diegojromerolopez/noctifab/pkg/domain"
	"github.com/diegojromerolopez/noctifab/pkg/infrastructure/prompts"
	"github.com/diegojromerolopez/noctifab/pkg/infrastructure/storage"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type planMockLLM struct {
	mu         sync.Mutex
	completeFn func(ctx context.Context, prompt string) (*domain.LLMResponse, error)
	calls      int
}

func (m *planMockLLM) Complete(ctx context.Context, prompt string) (*domain.LLMResponse, error) {
	m.mu.Lock()
	m.calls++
	fn := m.completeFn
	m.mu.Unlock()

	if fn != nil {
		return fn(ctx, prompt)
	}
	return &domain.LLMResponse{
		Actions: []domain.LLMAction{
			{
				Tool: "add_task",
				Args: map[string]any{
					"id":          "task-1",
					"title":       "Implement Feature",
					"description": "Write the core feature code",
				},
			},
		},
	}, nil
}

func (m *planMockLLM) Stream(ctx context.Context, prompt string, handler func(chunk string) error) (*domain.LLMResponse, error) {
	return m.Complete(ctx, prompt)
}

func (m *planMockLLM) EstimateTokens(text string) int {
	return len(text) / 4
}

func (m *planMockLLM) Ping(ctx context.Context) error {
	return nil
}

func TestOrchestrator_PlanStory(t *testing.T) {
	t.Run("when tasks are already planned, it returns early without LLM calls", func(t *testing.T) {
		llm := &planMockLLM{}
		repo := &mockRepo{state: &domain.State{
			ID:    "story-1",
			Tasks: []domain.Task{{ID: "task-0", Title: "Existing"}},
		}}
		o := &Orchestrator{
			repo:           repo,
			llmClient:      llm,
			promptRenderer: prompts.NewDefaultRenderer(),
		}

		state := &domain.State{
			ID:    "story-1",
			Tasks: []domain.Task{{ID: "task-0", Title: "Existing"}},
		}
		err := o.PlanStory(context.Background(), state, "Feature Spec")
		require.NoError(t, err)
		assert.Equal(t, 0, llm.calls)
	})

	t.Run("when version conflict occurs on persistence, it retries with OCC and succeeds", func(t *testing.T) {
		llm := &planMockLLM{}
		conflictRepo := &mockConflictRepo{
			mockRepo:  mockRepo{state: &domain.State{ID: "story-1", Metadata: domain.StateMetadata{FeatureName: "Feature"}}},
			failSaves: 2, // Fail first 2 saves with domain.ErrVersionConflict
		}

		o := &Orchestrator{
			repo:           conflictRepo,
			llmClient:      llm,
			promptRenderer: prompts.NewDefaultRenderer(),
			cfg: OrchestratorConfig{
				OCCMaxRetries:  5,
				OCCBackoffBase: 1 * time.Millisecond,
			},
		}

		state := &domain.State{
			ID:       "story-1",
			Metadata: domain.StateMetadata{FeatureName: "Feature"},
		}

		err := o.PlanStory(context.Background(), state, "Feature Spec")
		require.NoError(t, err)
		assert.Equal(t, 1, llm.calls)
		assert.GreaterOrEqual(t, conflictRepo.saveCount, 3)
		assert.Len(t, state.Tasks, 1)
		assert.Equal(t, "task-1", state.Tasks[0].ID)
	})

	t.Run("when another worker already planned tasks concurrently, it succeeds cleanly", func(t *testing.T) {
		llm := &planMockLLM{}
		// Simulate state having tasks populated in repo while LLM was running
		repo := &mockRepo{
			state: &domain.State{
				ID: "story-1",
				Tasks: []domain.Task{
					{ID: "concurrent-task", Title: "Concurrent Plan"},
				},
			},
		}

		o := &Orchestrator{
			repo:           repo,
			llmClient:      llm,
			promptRenderer: prompts.NewDefaultRenderer(),
			cfg: OrchestratorConfig{
				OCCMaxRetries:  5,
				OCCBackoffBase: 1 * time.Millisecond,
			},
		}

		state := &domain.State{
			ID:       "story-1",
			Metadata: domain.StateMetadata{FeatureName: "Feature"},
		}

		err := o.PlanStory(context.Background(), state, "Feature Spec")
		require.NoError(t, err)
		assert.Equal(t, 1, llm.calls)
		// State in repo should retain the concurrently committed plan
		loaded, _ := repo.Load(context.Background())
		assert.Equal(t, "concurrent-task", loaded.Tasks[0].ID)
	})

	t.Run("when planning produces invalid tasks, it retries up to maxAttempts", func(t *testing.T) {
		llm := &planMockLLM{
			completeFn: func(ctx context.Context, prompt string) (*domain.LLMResponse, error) {
				return &domain.LLMResponse{
					Actions: []domain.LLMAction{
						{
							Tool: "unknown_tool",
						},
					},
				}, nil
			},
		}
		repo := &mockRepo{state: &domain.State{ID: "story-1"}}
		o := &Orchestrator{
			repo:           repo,
			llmClient:      llm,
			promptRenderer: prompts.NewDefaultRenderer(),
		}

		state := &domain.State{ID: "story-1"}
		err := o.PlanStory(context.Background(), state, "Feature Spec")
		assert.Error(t, err)
		assert.Equal(t, 3, llm.calls)
	})

	t.Run("when prompt rendering fails, it returns error immediately", func(t *testing.T) {
		repo := &mockRepo{state: &domain.State{ID: "story-1"}}
		o := &Orchestrator{
			repo: repo,
		}
		state := &domain.State{ID: "story-1"}
		assert.Panics(t, func() {
			_ = o.PlanStory(context.Background(), state, "Feature Spec")
		})
	})

	t.Run("when LLM completes with error, it retries up to maxAttempts", func(t *testing.T) {
		llm := &planMockLLM{
			completeFn: func(ctx context.Context, prompt string) (*domain.LLMResponse, error) {
				return nil, errors.New("API rate limit exceeded")
			},
		}
		repo := &mockRepo{state: &domain.State{ID: "story-1"}}
		o := &Orchestrator{
			repo:           repo,
			llmClient:      llm,
			promptRenderer: prompts.NewDefaultRenderer(),
		}

		state := &domain.State{ID: "story-1"}
		err := o.PlanStory(context.Background(), state, "Feature Spec")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "LLM planning failed")
		assert.Equal(t, 3, llm.calls)
	})

	t.Run("when mailbox is active, updateStateWithRetry routes through mailbox SendSync", func(t *testing.T) {
		repo := &mockRepo{state: &domain.State{ID: "story-1", Tasks: []domain.Task{}}}
		mailbox := NewCommandMailbox(repo)
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		go mailbox.Start(ctx)
		time.Sleep(5 * time.Millisecond)

		o := &Orchestrator{
			repo:    repo,
			mailbox: mailbox,
		}

		err := o.updateStateWithRetry(ctx, func(st *domain.State) error {
			st.Tasks = append(st.Tasks, domain.Task{ID: "routed-task", Title: "Mailbox Routed"})
			return nil
		})
		require.NoError(t, err)

		loaded, _ := repo.Load(ctx)
		require.Len(t, loaded.Tasks, 1)
		assert.Equal(t, "routed-task", loaded.Tasks[0].ID)
	})

	t.Run("when multiple stories are planned, their tasks are merged into global state.Tasks without overwriting", func(t *testing.T) {
		llm := &planMockLLM{}
		repo := &mockRepo{
			state: &domain.State{
				ID: "multi-story-state",
				Tasks: []domain.Task{
					{ID: "US-001-TASK-001", Title: "Story 1 Task", StoryID: "US-001"},
				},
				Stories: []domain.Story{
					{ID: "US-001", Title: "Story 1"},
					{ID: "US-002", Title: "Story 2"},
				},
			},
		}

		o := &Orchestrator{
			repo:           repo,
			llmClient:      llm,
			promptRenderer: prompts.NewDefaultRenderer(),
		}

		state := &domain.State{
			ID: "multi-story-state",
			Metadata: domain.StateMetadata{
				InputPath:   "roadmap/user-stories/US-002.md",
				FeatureName: "US-002",
			},
			Tasks: []domain.Task{
				{ID: "US-001-TASK-001", Title: "Story 1 Task", StoryID: "US-001"},
			},
			Stories: []domain.Story{
				{ID: "US-001", Title: "Story 1"},
				{ID: "US-002", Title: "Story 2"},
			},
		}

		err := o.PlanStory(context.Background(), state, "Story 2 Spec")
		require.NoError(t, err)
		assert.Equal(t, 1, llm.calls)

		// State must now contain both Story 1 Task and the newly planned Story 2 Task
		loaded, err := repo.Load(context.Background())
		require.NoError(t, err)
		assert.Len(t, loaded.Tasks, 2)
		assert.Equal(t, "US-001-TASK-001", loaded.Tasks[0].ID)
		assert.Equal(t, "task-1", loaded.Tasks[1].ID)
	})
}

func TestPlanStory_SQLite_ConcurrentPlanningAndMutationsNoOCCError(t *testing.T) {
	t.Run("when concurrent planners and background workers mutate real SQLite state via mailbox, zero OCC errors occur", func(t *testing.T) {
		ctx := context.Background()
		tmpDir, err := os.MkdirTemp("", "noctifab-sqlite-plan-test-*")
		require.NoError(t, err)
		defer func() { _ = os.RemoveAll(tmpDir) }()

		dbPath := filepath.Join(tmpDir, "test.db")
		repo, err := storage.NewSQLiteRepository(ctx, dbPath)
		require.NoError(t, err)
		defer func() { _ = repo.Close() }()

		initialState := &domain.State{
			ID:          "story-occ-test",
			ProjectPath: tmpDir,
			Metadata: domain.StateMetadata{
				FeatureName: "User Authentication",
			},
			Tasks: []domain.Task{},
		}
		err = repo.Save(ctx, initialState)
		require.NoError(t, err)

		mailbox := NewCommandMailbox(repo)
		mailboxCtx, cancelMailbox := context.WithCancel(ctx)
		defer cancelMailbox()
		go mailbox.Start(mailboxCtx)
		time.Sleep(5 * time.Millisecond)

		llm := &planMockLLM{
			completeFn: func(ctx context.Context, prompt string) (*domain.LLMResponse, error) {
				time.Sleep(30 * time.Millisecond) // Simulate slow LLM execution
				return &domain.LLMResponse{
					Actions: []domain.LLMAction{
						{
							Tool: "add_task",
							Args: map[string]any{
								"id":          "task-auth",
								"title":       "Implement Auth Core",
								"description": "Auth token generator",
							},
						},
					},
				}, nil
			},
		}

		orchestrator := &Orchestrator{
			repo:           repo,
			mailbox:        mailbox,
			llmClient:      llm,
			promptRenderer: prompts.NewDefaultRenderer(),
			cfg: OrchestratorConfig{
				OCCMaxRetries:  20,
				OCCBackoffBase: 50 * time.Millisecond,
			},
		}

		var wg sync.WaitGroup
		errs := make(chan error, 6)

		// 2 concurrent planners
		for i := 1; i <= 2; i++ {
			wg.Add(1)
			go func(workerID int) {
				defer wg.Done()
				state, loadErr := repo.Load(ctx)
				if loadErr != nil {
					errs <- loadErr
					return
				}
				errs <- orchestrator.PlanStory(ctx, state, "Auth Spec")
			}(i)
		}

		// 4 concurrent background workers mutating tokens and file indexes
		for i := 1; i <= 4; i++ {
			wg.Add(1)
			go func(workerID int) {
				defer wg.Done()
				for j := 0; j < 3; j++ {
					time.Sleep(10 * time.Millisecond)
					updateErr := orchestrator.updateStateWithRetry(ctx, func(st *domain.State) error {
						st.Metadata.TotalTokensUsed += 50
						return nil
					})
					if updateErr != nil {
						errs <- updateErr
						return
					}
				}
				errs <- nil
			}(i)
		}

		wg.Wait()
		close(errs)

		for err := range errs {
			assert.NoError(t, err, "no worker should fail with optimistic concurrency version conflict")
		}

		finalState, err := repo.Load(ctx)
		require.NoError(t, err)
		assert.GreaterOrEqual(t, len(finalState.Tasks), 1, "planned tasks must be safely persisted")
		assert.Greater(t, finalState.Metadata.TotalTokensUsed, int64(0), "token usage metrics must be safely persisted")
	})

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
