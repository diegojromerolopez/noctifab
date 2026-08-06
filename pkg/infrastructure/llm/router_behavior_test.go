package llm

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/diegojromerolopez/noctifab/pkg/domain"
	"github.com/diegojromerolopez/noctifab/pkg/infrastructure/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// seedRouterCandidates injects a fixed candidate list into the router's
// per-role memoization cache so Complete uses mock clients.
func seedRouterCandidates(r *ResilientLLMRouter, roleName string, candidates []RouterCandidate) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.candidateCache == nil {
		r.candidateCache = make(map[string][]RouterCandidate)
	}
	r.candidateCache[roleName] = candidates
}

func TestRouterCandidateMemoization(t *testing.T) {
	cfg := &config.Config{
		LLM: config.LLMConfig{
			Priority: []string{"openai-primary"},
			Providers: []config.ProviderSpec{
				{Name: "openai-primary", Provider: "openai", Model: "gpt-4o"},
			},
		},
	}

	t.Run("when the same role is resolved twice it returns the memoized candidate list", func(t *testing.T) {
		router := NewResilientLLMRouter(cfg, nil)
		first := router.ResolveCandidatesForRole("generator")
		second := router.ResolveCandidatesForRole("generator")
		require.Len(t, first, 1)
		require.Len(t, second, 1)
		// Same underlying client instance proves the list was not rebuilt.
		assert.Same(t, first[0].Client, second[0].Client)
	})

	t.Run("when the cache is invalidated it rebuilds the candidate list", func(t *testing.T) {
		router := NewResilientLLMRouter(cfg, nil)
		first := router.ResolveCandidatesForRole("generator")
		router.InvalidateCandidateCache()
		second := router.ResolveCandidatesForRole("generator")
		require.Len(t, first, 1)
		require.Len(t, second, 1)
		assert.NotSame(t, first[0].Client, second[0].Client)
	})
}

func TestRouterCooldownOnlyOnTransientErrors(t *testing.T) {
	newRouter := func() *ResilientLLMRouter {
		return NewResilientLLMRouter(&config.Config{}, nil)
	}

	t.Run("when a candidate fails with a transient error it is placed on cooldown", func(t *testing.T) {
		router := newRouter()
		calls := 0
		failing := &mockLLMClient{completeFn: func(ctx context.Context, prompt string) (*domain.LLMResponse, error) {
			calls++
			return nil, errors.New("HTTP error 429: rate limited")
		}}
		seedRouterCandidates(router, "", []RouterCandidate{
			{Name: "p1", Provider: "openai", Model: "m", Client: failing},
		})

		_, err := router.Complete(context.Background(), "hi")
		require.Error(t, err)
		assert.Equal(t, 1, calls)

		router.mu.RLock()
		until, onCooldown := router.cooldowns["p1"]
		router.mu.RUnlock()
		assert.True(t, onCooldown)
		assert.True(t, until.After(time.Now()))
	})

	t.Run("when a candidate fails with a deterministic error it is not placed on cooldown", func(t *testing.T) {
		router := newRouter()
		failing := &mockLLMClient{completeFn: func(ctx context.Context, prompt string) (*domain.LLMResponse, error) {
			return nil, &httpError{StatusCode: 401, Body: "unauthorized"}
		}}
		seedRouterCandidates(router, "", []RouterCandidate{
			{Name: "p1", Provider: "openai", Model: "m", Client: failing},
		})

		_, err := router.Complete(context.Background(), "hi")
		require.Error(t, err)

		router.mu.RLock()
		_, onCooldown := router.cooldowns["p1"]
		router.mu.RUnlock()
		assert.False(t, onCooldown, "deterministic 401 must not bench the provider")
	})
}

func TestRouterTokenAccounting(t *testing.T) {
	t.Run("when a completion succeeds it records estimated tokens instead of a call count", func(t *testing.T) {
		store := newMockBudgetStore()
		router := NewResilientLLMRouter(&config.Config{TokenUsageLimit: 1_000_000}, store)
		ok := &mockLLMClient{completeFn: func(ctx context.Context, prompt string) (*domain.LLMResponse, error) {
			return &domain.LLMResponse{Reasoning: "some reasoning output from the model"}, nil
		}}
		seedRouterCandidates(router, "", []RouterCandidate{
			{Name: "p1", Provider: "openai", Model: "m", Client: ok},
		})

		prompt := "this is a prompt that is long enough to estimate more than one token"
		_, err := router.Complete(context.Background(), prompt)
		require.NoError(t, err)

		today := time.Now().UTC().Format("2006-01-02")
		total, err := store.GetDailyUsage(context.Background(), today, "total")
		require.NoError(t, err)
		expected := estimateUsageTokens(prompt, &domain.LLMResponse{Reasoning: "some reasoning output from the model"})
		assert.Equal(t, expected, total)
		assert.Greater(t, total, int64(1), "usage must reflect token estimates, not one per call")
	})
}

func TestFailoverPendingBudgetCheck(t *testing.T) {
	t.Run("when the pending prompt estimate would exceed the daily limit it rejects before sending", func(t *testing.T) {
		store := newMockBudgetStore()
		called := false
		backend := NamedClient{Name: "b1", Model: "m", Client: &mockLLMClient{
			completeFn: func(ctx context.Context, prompt string) (*domain.LLMResponse, error) {
				called = true
				return &domain.LLMResponse{Reasoning: "ok"}, nil
			},
		}}
		// Limit of 10 tokens; prompt of ~100 chars estimates ~25 tokens.
		client := NewFailoverClient([]NamedClient{backend}, time.Minute, 0, store, 10)

		longPrompt := make([]byte, 100)
		for i := range longPrompt {
			longPrompt[i] = 'a'
		}
		_, err := client.Complete(context.Background(), string(longPrompt))
		require.Error(t, err)
		assert.ErrorIs(t, err, domain.ErrBudgetExhausted)
		assert.False(t, called, "backend must not be called when the pending estimate busts the budget")
	})

	t.Run("when the pending prompt estimate fits the daily limit it completes", func(t *testing.T) {
		store := newMockBudgetStore()
		backend := NamedClient{Name: "b1", Model: "m", Client: &mockLLMClient{}}
		client := NewFailoverClient([]NamedClient{backend}, time.Minute, 0, store, 1000)

		_, err := client.Complete(context.Background(), "short prompt")
		require.NoError(t, err)
	})
}
