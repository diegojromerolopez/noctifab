package services

import (
	"context"
	"os"
	"testing"

	"github.com/diegojromerolopez/noctifab/pkg/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type mockLLMClient struct {
	completeFn func(ctx context.Context, prompt string) (*domain.LLMResponse, error)
}

func (m *mockLLMClient) Complete(ctx context.Context, prompt string) (*domain.LLMResponse, error) {
	return m.completeFn(ctx, prompt)
}

func TestDisambiguate_ReturnsAnswer(t *testing.T) {
	git := NewGitClient("/tmp")
	llm := &mockLLMClient{
		completeFn: func(_ context.Context, _ string) (*domain.LLMResponse, error) {
			return &domain.LLMResponse{
				Actions: []domain.LLMAction{
					{Args: map[string]any{"answer": "use PostgreSQL"}},
				},
			}, nil
		},
	}
	d := NewIntentDisambiguator(git, llm)
	answer, err := d.Disambiguate(context.Background(), domain.Clarification{Question: "DB?"}, &domain.State{Metadata: domain.StateMetadata{BaseBranch: "main", FeatureName: "auth"}})
	require.NoError(t, err)
	assert.Equal(t, "use PostgreSQL", answer)
}

func TestDisambiguate_LLMFails(t *testing.T) {
	git := NewGitClient("/tmp")
	llm := &mockLLMClient{
		completeFn: func(_ context.Context, _ string) (*domain.LLMResponse, error) {
			return nil, assert.AnError
		},
	}
	d := NewIntentDisambiguator(git, llm)
	_, err := d.Disambiguate(context.Background(), domain.Clarification{Question: "DB?"}, &domain.State{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "disambiguation LLM call failed")
}

func TestDisambiguate_EmptyAnswer(t *testing.T) {
	git := NewGitClient("/tmp")
	llm := &mockLLMClient{
		completeFn: func(_ context.Context, _ string) (*domain.LLMResponse, error) {
			return &domain.LLMResponse{
				Actions: []domain.LLMAction{
					{Args: map[string]any{"answer": ""}},
				},
			}, nil
		},
	}
	d := NewIntentDisambiguator(git, llm)
	_, err := d.Disambiguate(context.Background(), domain.Clarification{Question: "DB?"}, &domain.State{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "missing 'answer' field")
}

func TestDisambiguate_NoActions(t *testing.T) {
	git := NewGitClient("/tmp")
	llm := &mockLLMClient{
		completeFn: func(_ context.Context, _ string) (*domain.LLMResponse, error) {
			return &domain.LLMResponse{}, nil
		},
	}
	d := NewIntentDisambiguator(git, llm)
	_, err := d.Disambiguate(context.Background(), domain.Clarification{Question: "DB?"}, &domain.State{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "returned no actions")
}

func TestDisambiguate_GitLogInContext(t *testing.T) {
	dir, err := os.MkdirTemp("", "noctifab-test-*")
	require.NoError(t, err)
	defer func() { _ = os.RemoveAll(dir) }()

	git := NewGitClient(dir)
	_, err = git.Run(context.Background(), true, "init")
	require.NoError(t, err)
	_, err = git.Run(context.Background(), true, "config", "user.email", "test@test.com")
	require.NoError(t, err)
	_, err = git.Run(context.Background(), true, "config", "user.name", "test")
	require.NoError(t, err)
	_, err = git.Run(context.Background(), true, "commit", "--allow-empty", "-m", "feat: initial commit")
	require.NoError(t, err)

	var capturedPrompt string
	llm := &mockLLMClient{
		completeFn: func(_ context.Context, prompt string) (*domain.LLMResponse, error) {
			capturedPrompt = prompt
			return &domain.LLMResponse{
				Actions: []domain.LLMAction{
					{Args: map[string]any{"answer": "yes"}},
				},
			}, nil
		},
	}
	d := NewIntentDisambiguator(git, llm)
	_, err = d.Disambiguate(context.Background(), domain.Clarification{Question: "DB?"}, &domain.State{
		Metadata: domain.StateMetadata{BaseBranch: "main", FeatureName: "auth"},
	})
	require.NoError(t, err)
	assert.Contains(t, capturedPrompt, "feat: initial commit")
	assert.Contains(t, capturedPrompt, "main")
	assert.Contains(t, capturedPrompt, "auth")
}

func TestDisambiguate_FileContext(t *testing.T) {
	git := NewGitClient("/tmp")
	var capturedPrompt string
	llm := &mockLLMClient{
		completeFn: func(_ context.Context, prompt string) (*domain.LLMResponse, error) {
			capturedPrompt = prompt
			return &domain.LLMResponse{
				Actions: []domain.LLMAction{
					{Args: map[string]any{"answer": "yes"}},
				},
			}, nil
		},
	}
	d := NewIntentDisambiguator(git, llm)
	_, err := d.Disambiguate(context.Background(), domain.Clarification{Question: "DB?"}, &domain.State{
		Metadata: domain.StateMetadata{BaseBranch: "main", FeatureName: "auth"},
		Files: []domain.FileInfo{
			{Path: "main.go"},
			{Path: "db/schema.sql"},
		},
	})
	require.NoError(t, err)
	assert.Contains(t, capturedPrompt, "main.go")
	assert.Contains(t, capturedPrompt, "db/schema.sql")
}

func TestDisambiguate_NoGitHistory(t *testing.T) {
	git := NewGitClient("/nonexistent")
	var capturedPrompt string
	llm := &mockLLMClient{
		completeFn: func(_ context.Context, prompt string) (*domain.LLMResponse, error) {
			capturedPrompt = prompt
			return &domain.LLMResponse{
				Actions: []domain.LLMAction{
					{Args: map[string]any{"answer": "yes"}},
				},
			}, nil
		},
	}
	d := NewIntentDisambiguator(git, llm)
	_, err := d.Disambiguate(context.Background(), domain.Clarification{Question: "DB?"}, &domain.State{
		Metadata: domain.StateMetadata{BaseBranch: "main", FeatureName: "auth"},
	})
	require.NoError(t, err)
	assert.Contains(t, capturedPrompt, "(no git history available)")
}
