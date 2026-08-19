package services

import (
	"context"
	"testing"

	"github.com/diegojromerolopez/noctifab/pkg/domain"
	"github.com/diegojromerolopez/noctifab/pkg/infrastructure/config"
	"github.com/diegojromerolopez/noctifab/pkg/infrastructure/llm"
	"github.com/diegojromerolopez/noctifab/pkg/infrastructure/prompts"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type mockSpecLLMClient struct {
	content string
}

func (m *mockSpecLLMClient) Complete(ctx context.Context, prompt string) (*domain.LLMResponse, error) {
	return &domain.LLMResponse{
		Reasoning: "Generated spec section",
		Actions: []domain.LLMAction{
			{
				Tool: "update_spec",
				Args: map[string]any{
					"section": "full_spec",
					"content": m.content,
				},
			},
		},
	}, nil
}

func TestSpecMultiAgentPipeline_ExecutePass(t *testing.T) {
	ctx := context.Background()

	mockClient := &mockSpecLLMClient{
		content: "# SPEC.md: Test App\n## 1. Overview\nOverview text\n## 2. Architecture\nArch text",
	}

	cfg := &config.Config{
		LLM: config.LLMConfig{
			Provider: "openai",
			Model:    "gpt-4o",
		},
	}

	router := llm.NewResilientLLMRouter(cfg, nil)
	// We can test stage execution with a custom client or router
	pipeline := NewSpecMultiAgentPipeline(cfg, router, nil)

	// Direct stage test with mock client
	draft, err := pipeline.executeStage(ctx, mockClient, "product_manager", "pm_draft", prompts.SpecPromptData{
		UserPrompt: "Build an API in Go",
	})
	require.NoError(t, err)
	assert.Contains(t, draft, "# SPEC.md: Test App")
}

func TestSpecMultiAgentPipeline_ExecuteRefinePass(t *testing.T) {
	ctx := context.Background()

	mockClient := &mockSpecLLMClient{
		content: "# SPEC.md: Refined Test App\n## 1. Overview\nRefined with PostgreSQL",
	}

	cfg := &config.Config{
		Spec: config.SpecConfig{
			LeadRole: "product_manager",
		},
	}
	pipeline := NewSpecMultiAgentPipeline(cfg, nil, nil)

	revisions := []domain.SpecRevision{
		{Version: 1, Prompt: "Initial build in SQLite"},
	}

	refined, err := pipeline.executeStage(ctx, mockClient, "product_manager", "refine", prompts.SpecPromptData{
		DraftSpec:    "# SPEC.md: Test App",
		Feedback:     "Switch to PostgreSQL",
		HumanHistory: "- Turn 1: Initial build in SQLite\n",
	})
	require.NoError(t, err)
	assert.Contains(t, refined, "Refined with PostgreSQL")

	// Missing client should error gracefully
	_, errMissing := pipeline.ExecuteRefinePass(ctx, "spec", "feedback", revisions)
	assert.Error(t, errMissing)
}
