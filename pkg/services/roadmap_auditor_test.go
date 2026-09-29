package services

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/diegojromerolopez/noctifab/pkg/domain"
	"github.com/diegojromerolopez/noctifab/pkg/infrastructure/prompts"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type mockRoadmapAuditorLLM struct {
	mu        sync.Mutex
	responses map[string]*domain.LLMResponse
	errOnID   string
	calls     int
}

func (m *mockRoadmapAuditorLLM) Complete(ctx context.Context, prompt string) (*domain.LLMResponse, error) {
	m.mu.Lock()
	m.calls++
	errOnID := m.errOnID
	m.mu.Unlock()
	if errOnID != "" && strings.Contains(prompt, "Target User Story to Audit & Refine:\n# "+errOnID) {
		return nil, fmt.Errorf("context deadline exceeded")
	}
	for target, resp := range m.responses {
		if target != "" && strings.Contains(prompt, "Target User Story to Audit & Refine:\n# "+target) {
			return resp, nil
		}
	}
	if fallback, ok := m.responses[""]; ok {
		return fallback, nil
	}
	return &domain.LLMResponse{Reasoning: "noop"}, nil
}

func TestBuildRoadmapCatalogFromStories(t *testing.T) {
	items := []StoryAuditItem{
		{
			ID:       "US-001",
			Title:    "Foundational Scaffolding",
			Filename: "US-001-scaffolding.md",
			Content:  "# US-001: Foundational Scaffolding\ndepends_on: []\n",
		},
		{
			ID:       "US-002",
			Title:    "String Commands",
			Filename: "US-002-strings.md",
			Content:  "# US-002: String Commands\ndepends_on: [\"US-001\"]\n",
		},
	}

	catalog := BuildRoadmapCatalogFromStories(items)
	assert.Contains(t, catalog, "US-001: Foundational Scaffolding")
	assert.Contains(t, catalog, "DependsOn: [none]")
	assert.Contains(t, catalog, "US-002: String Commands")
	assert.Contains(t, catalog, "DependsOn: [US-001]")
}

func TestAuditRoadmapStoriesPerStory_Success(t *testing.T) {
	tempDir := t.TempDir()
	storiesDir := filepath.Join(tempDir, "roadmap", "user-stories")
	require.NoError(t, os.MkdirAll(storiesDir, 0755))

	s1 := filepath.Join(storiesDir, "US-001-initial.md")
	s1Content := "# US-001: Initial\n```noctifab-contract\n{\"story_id\":\"US-001\",\"public_contracts\":[{\"id\":\"c1\",\"interface\":\"cli\",\"allowed_executables\":[\"./app\"],\"exit_codes\":[0]}]}\n```\n"
	require.NoError(t, os.WriteFile(s1, []byte(s1Content), 0644))

	s2 := filepath.Join(storiesDir, "US-002-feature.md")
	s2Content := "# US-002: Feature\n```noctifab-contract\n{\"story_id\":\"US-002\",\"public_contracts\":[{\"id\":\"c2\",\"interface\":\"cli\",\"allowed_executables\":[\"./app\"],\"exit_codes\":[0]}]}\n```\n"
	require.NoError(t, os.WriteFile(s2, []byte(s2Content), 0644))

	refinedS1 := "# US-001: Refined\n```noctifab-contract\n{\"story_id\":\"US-001\",\"public_contracts\":[{\"id\":\"c1\",\"interface\":\"cli\",\"allowed_executables\":[\"./app\"],\"exit_codes\":[0]}]}\n```\nRefined DoD"
	refinedS2 := "# US-002: Refined\n```noctifab-contract\n{\"story_id\":\"US-002\",\"public_contracts\":[{\"id\":\"c2\",\"interface\":\"cli\",\"allowed_executables\":[\"./app\"],\"exit_codes\":[0]}]}\n```\nRefined DoD"

	mock := &mockRoadmapAuditorLLM{
		responses: map[string]*domain.LLMResponse{
			"US-001": {
				Actions: []domain.LLMAction{
					{
						Tool: "create_story",
						Args: map[string]any{
							"filename": "US-001-initial.md",
							"content":  refinedS1,
						},
					},
				},
			},
			"US-002": {
				Actions: []domain.LLMAction{
					{
						Tool: "create_story",
						Args: map[string]any{
							"filename": "US-002-feature.md",
							"content":  refinedS2,
						},
					},
				},
			},
		},
	}

	renderer, err := prompts.NewRenderer(tempDir, nil)
	require.NoError(t, err)
	refinedCount, err := AuditRoadmapStoriesPerStory(
		context.Background(),
		tempDir,
		[]string{s1, s2},
		"# Spec\nSample specification",
		"",
		mock,
		renderer,
		nil,
	)

	require.NoError(t, err)
	assert.Equal(t, 2, refinedCount)

	// Verify file was updated on disk
	data, err := os.ReadFile(s1)
	require.NoError(t, err)
	assert.Contains(t, string(data), "Refined DoD")
}

func TestAuditRoadmapStoriesPerStory_PartialFailure_GracefulRetain(t *testing.T) {
	tempDir := t.TempDir()
	storiesDir := filepath.Join(tempDir, "roadmap", "user-stories")
	require.NoError(t, os.MkdirAll(storiesDir, 0755))

	s1 := filepath.Join(storiesDir, "US-001-initial.md")
	s1Content := "# US-001: Initial\n```noctifab-contract\n{\"story_id\":\"US-001\",\"public_contracts\":[{\"id\":\"c1\",\"interface\":\"cli\",\"allowed_executables\":[\"./app\"],\"exit_codes\":[0]}]}\n```\n"
	require.NoError(t, os.WriteFile(s1, []byte(s1Content), 0644))

	s2 := filepath.Join(storiesDir, "US-002-feature.md")
	s2Content := "# US-002: Feature\n```noctifab-contract\n{\"story_id\":\"US-002\",\"public_contracts\":[{\"id\":\"c2\",\"interface\":\"cli\",\"allowed_executables\":[\"./app\"],\"exit_codes\":[0]}]}\n```\nOriginal S2"
	require.NoError(t, os.WriteFile(s2, []byte(s2Content), 0644))

	refinedS1 := "# US-001: Refined\n```noctifab-contract\n{\"story_id\":\"US-001\",\"public_contracts\":[{\"id\":\"c1\",\"interface\":\"cli\",\"allowed_executables\":[\"./app\"],\"exit_codes\":[0]}]}\n```\nRefined S1"

	// Mock fails on US-002
	mock := &mockRoadmapAuditorLLM{
		errOnID: "US-002",
		responses: map[string]*domain.LLMResponse{
			"US-001": {
				Actions: []domain.LLMAction{
					{
						Tool: "create_story",
						Args: map[string]any{
							"filename": "US-001-initial.md",
							"content":  refinedS1,
						},
					},
				},
			},
		},
	}

	renderer, err := prompts.NewRenderer(tempDir, nil)
	require.NoError(t, err)

	refinedCount, err := AuditRoadmapStoriesPerStory(
		context.Background(),
		tempDir,
		[]string{s1, s2},
		"# Spec",
		"",
		mock,
		renderer,
		nil,
	)

	require.NoError(t, err)
	assert.Equal(t, 1, refinedCount)

	// Verify s1 was updated
	d1, err := os.ReadFile(s1)
	require.NoError(t, err)
	assert.Contains(t, string(d1), "Refined S1")

	// Verify s2 was retained as-is
	d2, err := os.ReadFile(s2)
	require.NoError(t, err)
	assert.Contains(t, string(d2), "Original S2")
}
