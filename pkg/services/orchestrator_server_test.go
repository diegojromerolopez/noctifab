package services

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/diegojromerolopez/noctifab/pkg/domain"
	"github.com/diegojromerolopez/noctifab/pkg/infrastructure/prompts"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type planMockLLM struct {
	completeFn func(ctx context.Context, prompt string) (*domain.LLMResponse, error)
	calls      int
}

func (m *planMockLLM) Complete(ctx context.Context, prompt string) (*domain.LLMResponse, error) {
	m.calls++
	if m.completeFn != nil {
		return m.completeFn(ctx, prompt)
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
}
