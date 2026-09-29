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
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type sequenceMockLLMClient struct {
	mu        sync.Mutex
	responses []*domain.LLMResponse
	calls     int
}

func (s *sequenceMockLLMClient) Complete(ctx context.Context, prompt string) (*domain.LLMResponse, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.calls < len(s.responses) {
		resp := s.responses[s.calls]
		s.calls++
		return resp, nil
	}
	return &domain.LLMResponse{
		Actions: []domain.LLMAction{},
	}, nil
}

type twoStageTestMockClient struct {
	stage1Resp *domain.LLMResponse
	storyResps map[string]*domain.LLMResponse
}

func (m *twoStageTestMockClient) Complete(ctx context.Context, prompt string) (*domain.LLMResponse, error) {
	if strings.Contains(prompt, "TARGET STORY TO EXPAND:") {
		targetSec := prompt[strings.Index(prompt, "TARGET STORY TO EXPAND:"):]
		if endIdx := strings.Index(targetSec, "ROADMAP CATALOG"); endIdx != -1 {
			targetSec = targetSec[:endIdx]
		}
		for id, resp := range m.storyResps {
			marker := fmt.Sprintf("ID: %s", id)
			if strings.Contains(targetSec, marker) {
				return resp, nil
			}
		}
	}
	return m.stage1Resp, nil
}

func TestParseStoryOutlines_Valid(t *testing.T) {
	act := domain.LLMAction{
		Tool: "plan_roadmap",
		Args: map[string]any{
			"stories": []any{
				map[string]any{
					"id":           "US-001",
					"title":        "Walking Skeleton Thin TCP PING Entrypoint",
					"slug":         "US-001-walking-skeleton-thin-tcp-ping-entrypoint",
					"domain_slice": "Core Networking & RESP Parser",
					"depends_on":   []any{},
					"complexity":   25,
					"summary":      "Implement thin entrypoint, TCP socket loop, and PING/ECHO/QUIT commands.",
				},
				map[string]any{
					"id":           "US-002",
					"title":        "String Key-Value Store & TTL",
					"slug":         "US-002-string-key-value-store-ttl",
					"domain_slice": "Key-Value Engine",
					"depends_on":   []any{"US-001"},
					"complexity":   30,
					"summary":      "Implement GET, SET, DEL, EXISTS, EXPIRE, and TTL with absolute epoch expiry.",
				},
			},
		},
	}

	outlines, err := ParseStoryOutlines(act)
	require.NoError(t, err)
	require.Len(t, outlines, 2)
	assert.Equal(t, "US-001", outlines[0].ID)
	assert.Equal(t, "Walking Skeleton Thin TCP PING Entrypoint", outlines[0].Title)
	assert.Equal(t, "Core Networking & RESP Parser", outlines[0].DomainSlice)
	assert.Equal(t, 25, outlines[0].Complexity)
	assert.Equal(t, "US-002", outlines[1].ID)
	assert.Equal(t, []string{"US-001"}, outlines[1].DependsOn)
}

func TestParseStoryOutlines_Invalid(t *testing.T) {
	_, err := ParseStoryOutlines(domain.LLMAction{Tool: "plan_roadmap", Args: map[string]any{}})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "missing 'stories' argument")

	_, err = ParseStoryOutlines(domain.LLMAction{Tool: "plan_roadmap", Args: map[string]any{"stories": []any{}}})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "no valid story outline items found")
}

func TestFormatRoadmapCatalog(t *testing.T) {
	outlines := []StoryOutlineItem{
		{
			ID:          "US-001",
			Title:       "Walking Skeleton",
			Slug:        "us-001-walking-skeleton",
			DomainSlice: "Core Networking",
			DependsOn:   []string{},
			Complexity:  25,
			Summary:     "Baseline networking.",
		},
		{
			ID:          "US-002",
			Title:       "Strings",
			Slug:        "us-002-strings",
			DomainSlice: "Storage",
			DependsOn:   []string{"US-001"},
			Complexity:  30,
			Summary:     "Key value storage.",
		},
	}

	catalog := FormatRoadmapCatalog(outlines)
	assert.Contains(t, catalog, "US-001: Walking Skeleton")
	assert.Contains(t, catalog, "Domain Capability: Core Networking")
	assert.Contains(t, catalog, "US-002: Strings")
	assert.Contains(t, catalog, "DependsOn: [US-001]")
}

func TestExpandRoadmapStory_Success(t *testing.T) {
	tempDir := t.TempDir()
	target := StoryOutlineItem{
		ID:          "US-001",
		Title:       "Walking Skeleton",
		Slug:        "us-001-walking-skeleton",
		DomainSlice: "Core Networking",
		DependsOn:   []string{},
		Complexity:  25,
		Summary:     "Baseline networking.",
	}

	storyMarkdown := `# US-001: Walking Skeleton

## Overview
Baseline TCP entrypoint.

` + "```noctifab-contract\n" + `{
  "story_id": "US-001",
  "public_contracts": [{
    "id": "tcp.ping",
    "interface": "TCP :6379",
    "allowed_executables": ["python3"],
    "exit_codes": [0],
    "stdout_contains": [],
    "stderr_prefixes": []
  }]
}
` + "```\n"

	mockLLM := &sequenceMockLLMClient{
		responses: []*domain.LLMResponse{
			{
				Actions: []domain.LLMAction{
					{
						Tool: "create_story",
						Args: map[string]any{
							"filename": "roadmap/user-stories/US-001-walking-skeleton.md",
							"content":  storyMarkdown,
						},
					},
				},
			},
		},
	}

	rawStory, err := ExpandRoadmapStory(context.Background(), tempDir, target, []StoryOutlineItem{target}, "# Spec", "", nil, mockLLM, nil)
	require.NoError(t, err)
	require.NotNil(t, rawStory)
	assert.Equal(t, "roadmap/user-stories/US-001-walking-skeleton.md", rawStory.Filename)
	assert.Contains(t, rawStory.Content, "story_id")
}

func TestExpandRoadmapStory_SyntheticFallback(t *testing.T) {
	tempDir := t.TempDir()
	target := StoryOutlineItem{
		ID:          "US-003",
		Title:       "Bitmap Operations",
		Slug:        "us-003-bitmap-operations",
		DomainSlice: "Bitmaps",
		DependsOn:   []string{"US-001"},
		Complexity:  20,
		Summary:     "Bit operations.",
	}

	// LLM returns no actions or matching content
	mockLLM := &sequenceMockLLMClient{
		responses: []*domain.LLMResponse{
			{
				Reasoning: "I could not format the response properly.",
				Actions:   []domain.LLMAction{},
			},
		},
	}

	rawStory, err := ExpandRoadmapStory(context.Background(), tempDir, target, []StoryOutlineItem{target}, "# Spec", "", nil, mockLLM, nil)
	require.NoError(t, err)
	require.NotNil(t, rawStory)
	assert.Equal(t, "roadmap/user-stories/US-003-us-003-bitmap-operations.md", rawStory.Filename)
	assert.Contains(t, rawStory.Content, "# US-003: Bitmap Operations")
	assert.Contains(t, rawStory.Content, "noctifab-contract")
}

func TestGenerateRoadmapWithFullConfig_TwoStageExecution(t *testing.T) {
	tempDir := t.TempDir()
	specPath := filepath.Join(tempDir, "SPEC.md")
	require.NoError(t, os.WriteFile(specPath, []byte("# Comprehensive Redis Server Spec\n\nFull implementation required."), 0644))

	// Stage 1 LLM response: plan_roadmap action
	stage1Resp := &domain.LLMResponse{
		Reasoning: "Planning 2-stage roadmap",
		Actions: []domain.LLMAction{
			{
				Tool: "plan_roadmap",
				Args: map[string]any{
					"stories": []any{
						map[string]any{
							"id":           "US-001",
							"title":        "Walking Skeleton",
							"slug":         "us-001-walking-skeleton",
							"domain_slice": "Networking",
							"depends_on":   []any{},
							"complexity":   25,
							"summary":      "Thin shell entrypoint.",
						},
						map[string]any{
							"id":           "US-002",
							"title":        "Strings & TTL",
							"slug":         "us-002-strings-ttl",
							"domain_slice": "Strings",
							"depends_on":   []any{"US-001"},
							"complexity":   30,
							"summary":      "Strings and expiration.",
						},
					},
				},
			},
		},
	}

	// Stage 2 LLM responses for each story
	story1MD := "# US-001: Walking Skeleton\n\n```noctifab-contract\n{\"story_id\":\"US-001\",\"public_contracts\":[{\"id\":\"p1\",\"interface\":\"CLI\",\"allowed_executables\":[\"python3\"],\"exit_codes\":[0],\"stdout_contains\":[],\"stderr_prefixes\":[]}]}\n```\n"
	story2MD := "# US-002: Strings & TTL\n\n```noctifab-contract\n{\"story_id\":\"US-002\",\"public_contracts\":[{\"id\":\"p2\",\"interface\":\"CLI\",\"allowed_executables\":[\"python3\"],\"exit_codes\":[0],\"stdout_contains\":[],\"stderr_prefixes\":[]}]}\n```\n"

	stage2Story1Resp := &domain.LLMResponse{
		Actions: []domain.LLMAction{
			{
				Tool: "create_story",
				Args: map[string]any{
					"filename": "roadmap/user-stories/US-001-walking-skeleton.md",
					"content":  story1MD,
				},
			},
		},
	}

	stage2Story2Resp := &domain.LLMResponse{
		Actions: []domain.LLMAction{
			{
				Tool: "create_story",
				Args: map[string]any{
					"filename": "roadmap/user-stories/US-002-strings-ttl.md",
					"content":  story2MD,
				},
			},
		},
	}

	mockLLM := &twoStageTestMockClient{
		stage1Resp: stage1Resp,
		storyResps: map[string]*domain.LLMResponse{
			"US-001": stage2Story1Resp,
			"US-002": stage2Story2Resp,
		},
	}

	err := GenerateRoadmapWithFullConfig(context.Background(), tempDir, mockLLM, nil, 1, 5, 15, 35)
	require.NoError(t, err)

	// Verify both user story files were created
	story1Path := filepath.Join(tempDir, "roadmap", "user-stories", "US-001-walking-skeleton.md")
	story2Path := filepath.Join(tempDir, "roadmap", "user-stories", "US-002-strings-ttl.md")

	assert.FileExists(t, story1Path)
	assert.FileExists(t, story2Path)

	content1, err := os.ReadFile(story1Path)
	require.NoError(t, err)
	assert.Contains(t, string(content1), "Walking Skeleton")

	content2, err := os.ReadFile(story2Path)
	require.NoError(t, err)
	assert.Contains(t, string(content2), "Strings & TTL")
}

func TestExpandRoadmapStoriesParallel_SuccessAndOrdering(t *testing.T) {
	tempDir := t.TempDir()
	outlines := []StoryOutlineItem{
		{ID: "US-001", Title: "Story 1", Slug: "us-001-story-1"},
		{ID: "US-002", Title: "Story 2", Slug: "us-002-story-2"},
		{ID: "US-003", Title: "Story 3", Slug: "us-003-story-3"},
		{ID: "US-004", Title: "Story 4", Slug: "us-004-story-4"},
	}

	// Mock LLM that generates responses based on story ID found in prompt
	promptMatchMock := &promptMatchingMockLLMClient{
		responses: map[string]*domain.LLMResponse{
			"US-001": {
				Actions: []domain.LLMAction{
					{
						Tool: "create_story",
						Args: map[string]any{
							"filename": "roadmap/user-stories/US-001-story-1.md",
							"content":  "# US-001: Story 1\n```noctifab-contract\n{\"story_id\":\"US-001\"}\n```",
						},
					},
				},
			},
			"US-002": {
				Actions: []domain.LLMAction{
					{
						Tool: "create_story",
						Args: map[string]any{
							"filename": "roadmap/user-stories/US-002-story-2.md",
							"content":  "# US-002: Story 2\n```noctifab-contract\n{\"story_id\":\"US-002\"}\n```",
						},
					},
				},
			},
			// US-003 returns no actions to test synthetic fallback in parallel
			"US-003": {
				Reasoning: "Fallback triggered",
				Actions:   []domain.LLMAction{},
			},
			"US-004": {
				Actions: []domain.LLMAction{
					{
						Tool: "create_story",
						Args: map[string]any{
							"filename": "roadmap/user-stories/US-004-story-4.md",
							"content":  "# US-004: Story 4\n```noctifab-contract\n{\"story_id\":\"US-004\"}\n```",
						},
					},
				},
			},
		},
	}

	stories := ExpandRoadmapStoriesParallel(context.Background(), tempDir, outlines, nil, "# Spec", "", nil, promptMatchMock, nil, 2)
	require.Len(t, stories, 4)

	// Invariant: Stories must strictly preserve outline ordering
	assert.Contains(t, stories[0].Filename, "US-001")
	assert.Contains(t, stories[1].Filename, "US-002")
	assert.Contains(t, stories[2].Filename, "US-003")
	assert.Contains(t, stories[3].Filename, "US-004")

	// US-003 should have synthetic fallback content
	assert.Contains(t, stories[2].Content, "# US-003: Story 3")
	assert.Contains(t, stories[2].Content, "noctifab-contract")
}

type promptMatchingMockLLMClient struct {
	responses map[string]*domain.LLMResponse
}

func (p *promptMatchingMockLLMClient) Complete(ctx context.Context, prompt string) (*domain.LLMResponse, error) {
	// Look specifically at the TARGET STORY TO EXPAND section
	targetSection := prompt
	if idx := strings.Index(prompt, "TARGET STORY TO EXPAND:"); idx != -1 {
		targetSection = prompt[idx:]
		if endIdx := strings.Index(targetSection, "ROADMAP CATALOG"); endIdx != -1 {
			targetSection = targetSection[:endIdx]
		}
	}

	for id, resp := range p.responses {
		marker := fmt.Sprintf("ID: %s", id)
		if strings.Contains(targetSection, marker) {
			return resp, nil
		}
	}
	return &domain.LLMResponse{Actions: []domain.LLMAction{}}, nil
}
