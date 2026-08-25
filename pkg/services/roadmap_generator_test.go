package services_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/diegojromerolopez/noctifab/pkg/domain"
	"github.com/diegojromerolopez/noctifab/pkg/services"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockRoadmapLLMClient is a test double for domain.LLMClient.
type mockRoadmapLLMClient struct {
	Response *domain.LLMResponse
	Err      error
}

func (m *mockRoadmapLLMClient) Complete(ctx context.Context, prompt string) (*domain.LLMResponse, error) {
	return m.Response, m.Err
}

func TestGenerateRoadmap_Success(t *testing.T) {
	// Create a temp directory for the project root
	tempDir, err := os.MkdirTemp("", "noctifab-roadmap-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer func() { _ = os.RemoveAll(tempDir) }()

	// Create SPEC.md
	specPath := filepath.Join(tempDir, "SPEC.md")
	err = os.WriteFile(specPath, []byte("# My Project Spec"), 0644)
	assert.NoError(t, err)

	// Configure mock LLM client to return create_story actions
	mockLLM := &mockRoadmapLLMClient{
		Response: &domain.LLMResponse{
			Reasoning: "Generating roadmap",
			Actions: []domain.LLMAction{
				{
					Tool: "create_story",
					Args: map[string]any{
						"filename": "roadmap/user-stories/US-001-content.md",
						"content":  "# US-001 content",
					},
				},
				{
					Tool: "create_story",
					Args: map[string]any{
						"filename": "roadmap/user-stories/US-002-content.md",
						"content":  "# US-002 content",
					},
				},
			},
		},
	}

	err = services.GenerateRoadmap(context.Background(), tempDir, mockLLM, nil)
	assert.NoError(t, err)

	// Verify the roadmap files were written under roadmap/user-stories/
	us1Path := filepath.Join(tempDir, "roadmap", "user-stories", "US-001-content.md")
	us2Path := filepath.Join(tempDir, "roadmap", "user-stories", "US-002-content.md")

	assert.FileExists(t, us1Path)
	assert.FileExists(t, us2Path)

	us1Bytes, err := os.ReadFile(us1Path)
	assert.NoError(t, err)
	assert.Equal(t, "# US-001 content", string(us1Bytes))

	us2Bytes, err := os.ReadFile(us2Path)
	assert.NoError(t, err)
	assert.Equal(t, "# US-002 content", string(us2Bytes))
}

func TestGenerateRoadmap_MissingSpec(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "noctifab-roadmap-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer func() { _ = os.RemoveAll(tempDir) }()

	mockLL := &mockRoadmapLLMClient{}
	err = services.GenerateRoadmap(context.Background(), tempDir, mockLL, nil)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "SPEC.md not found")
}

func TestGenerateRoadmap_NoValidActions(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "noctifab-roadmap-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer func() { _ = os.RemoveAll(tempDir) }()

	specPath := filepath.Join(tempDir, "SPEC.md")
	err = os.WriteFile(specPath, []byte("# My Project Spec"), 0644)
	assert.NoError(t, err)

	mockLL := &mockRoadmapLLMClient{
		Response: &domain.LLMResponse{
			Reasoning: "No actions",
			Actions:   []domain.LLMAction{},
		},
	}
	err = services.GenerateRoadmap(context.Background(), tempDir, mockLL, nil)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "did not return any valid create_story or refine_spec actions")
}

func TestGenerateRoadmap_RefineExistingStories(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "noctifab-roadmap-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer func() { _ = os.RemoveAll(tempDir) }()

	specPath := filepath.Join(tempDir, "SPEC.md")
	err = os.WriteFile(specPath, []byte("# Calculator Spec\nBuild CLI calculator."), 0644)
	assert.NoError(t, err)

	storiesDir := filepath.Join(tempDir, "roadmap", "user-stories")
	err = os.MkdirAll(storiesDir, 0755)
	assert.NoError(t, err)

	existingStoryPath := filepath.Join(storiesDir, "US-001.md")
	err = os.WriteFile(existingStoryPath, []byte("# Existing Vague Story\nBuild calculator."), 0644)
	assert.NoError(t, err)

	var capturedPrompt string
	mockLLM := &mockRoadmapLLMClient{
		Response: &domain.LLMResponse{
			Reasoning: "Refining existing story with DoD and contracts",
			Actions: []domain.LLMAction{
				{
					Tool: "create_story",
					Args: map[string]any{
						"filename": "roadmap/user-stories/US-001.md",
						"content":  "# US-001: Refined Story with DoD\n## Definition of Done\n- Interface: Calculator::CLI\n",
					},
				},
			},
		},
	}

	// Capture prompt in mock
	mockLLMWithPromptCapture := &promptCapturingMockClient{
		mockRoadmapLLMClient: mockLLM,
		capturedPrompt:       &capturedPrompt,
	}

	err = services.GenerateRoadmap(context.Background(), tempDir, mockLLMWithPromptCapture, nil)
	assert.NoError(t, err)

	assert.Contains(t, capturedPrompt, "Audit and refine existing user stories")
	assert.Contains(t, capturedPrompt, "# Existing Vague Story")

	refinedBytes, err := os.ReadFile(existingStoryPath)
	assert.NoError(t, err)
	assert.Contains(t, string(refinedBytes), "Refined Story with DoD")
}

type promptCapturingMockClient struct {
	*mockRoadmapLLMClient
	capturedPrompt *string
}

func (p *promptCapturingMockClient) Complete(ctx context.Context, prompt string) (*domain.LLMResponse, error) {
	*p.capturedPrompt = prompt
	return p.mockRoadmapLLMClient.Complete(ctx, prompt)
}

func TestGenerateRoadmap_DetectsLegacyCode(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "noctifab-roadmap-legacy-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer func() { _ = os.RemoveAll(tempDir) }()

	specPath := filepath.Join(tempDir, "SPEC.md")
	err = os.WriteFile(specPath, []byte("# Project Spec"), 0644)
	assert.NoError(t, err)

	// Create a legacy source file
	legacyFilePath := filepath.Join(tempDir, "calculator.py")
	err = os.WriteFile(legacyFilePath, []byte("def add(a, b): return a + b"), 0644)
	assert.NoError(t, err)

	var capturedPrompt string
	mockLLM := &mockRoadmapLLMClient{
		Response: &domain.LLMResponse{
			Reasoning: "Generating story with legacy stabilization",
			Actions: []domain.LLMAction{
				{
					Tool: "create_story",
					Args: map[string]any{
						"filename": "roadmap/US-001.md",
						"content":  "# US-001: Legacy Codebase Characterization & Stabilization",
					},
				},
			},
		},
	}
	capturingMock := &promptCapturingMockClient{
		mockRoadmapLLMClient: mockLLM,
		capturedPrompt:       &capturedPrompt,
	}

	err = services.GenerateRoadmap(context.Background(), tempDir, capturingMock, nil)
	assert.NoError(t, err)

	assert.Contains(t, capturedPrompt, "Existing Legacy Code Files Detected in Workspace:")
	assert.Contains(t, capturedPrompt, "calculator.py")
	assert.Contains(t, capturedPrompt, "LEGACY STABILIZATION MANDATE")
}

func TestGenerateRoadmapWithPasses_MultiPass(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "noctifab-roadmap-multipass-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer func() { _ = os.RemoveAll(tempDir) }()

	specPath := filepath.Join(tempDir, "SPEC.md")
	err = os.WriteFile(specPath, []byte("# MultiPass Spec"), 0644)
	assert.NoError(t, err)

	mockLLM := &mockRoadmapLLMClient{
		Response: &domain.LLMResponse{
			Reasoning: "Generating multi-pass roadmap",
			Actions: []domain.LLMAction{
				{
					Tool: "create_story",
					Args: map[string]any{
						"filename": "roadmap/user-stories/US-001-initial-story.md",
						"content":  "# US-001: Initial Story\n",
					},
				},
			},
		},
	}

	err = services.GenerateRoadmapWithPasses(context.Background(), tempDir, mockLLM, nil, 3)
	assert.NoError(t, err)

	us1Path := filepath.Join(tempDir, "roadmap", "user-stories", "US-001-initial-story.md")
	_, err = os.Stat(us1Path)
	assert.NoError(t, err)
}

func TestNormalizeStoryPath_And_ToSlug(t *testing.T) {
	tempDir := t.TempDir()

	t.Run("converts pure US ID filename to roadmap/user-stories/ with title slug", func(t *testing.T) {
		got := services.NormalizeStoryPath(tempDir, "roadmap/US-001.md", "# US-001: Framing and Binary-Safe Streaming")
		expected := filepath.Join(tempDir, "roadmap", "user-stories", "US-001-framing-and-binary-safe-streaming.md")
		assert.Equal(t, expected, got)
	})

	t.Run("preserves path if target file already exists on disk", func(t *testing.T) {
		existing := filepath.Join(tempDir, "roadmap", "user-stories", "US-001.md")
		require.NoError(t, os.MkdirAll(filepath.Dir(existing), 0755))
		require.NoError(t, os.WriteFile(existing, []byte("# Existing"), 0644))

		got := services.NormalizeStoryPath(tempDir, "roadmap/user-stories/US-001.md", "# US-001: Refined Story")
		assert.Equal(t, existing, got)
	})

	t.Run("ToSlug formats title nicely", func(t *testing.T) {
		assert.Equal(t, "user-story-001-test", services.ToSlug("User Story 001: Test!"))
		assert.Equal(t, "framing-streaming", services.ToSlug("Framing & Streaming"))
	})
}

func TestGenerateRoadmap_RefineSpec(t *testing.T) {
	tempDir := t.TempDir()
	specPath := filepath.Join(tempDir, "SPEC.md")
	err := os.WriteFile(specPath, []byte("# Incomplete Spec\nMissing commands"), 0644)
	require.NoError(t, err)

	refinedSpec := "# Complete Spec\n## 1. Commands\nGET, SET, DEL, PING\n## 2. Wire Protocol\nRESP2"
	mockLLM := &mockRoadmapLLMClient{
		Response: &domain.LLMResponse{
			Reasoning: "Refining spec to add missing commands and wire protocol",
			Actions: []domain.LLMAction{
				{
					Tool: "refine_spec",
					Args: map[string]any{
						"content": refinedSpec,
					},
				},
				{
					Tool: "create_story",
					Args: map[string]any{
						"filename": "roadmap/user-stories/US-001-complete.md",
						"content":  "# US-001 Complete",
					},
				},
			},
		},
	}

	err = services.GenerateRoadmap(context.Background(), tempDir, mockLLM, nil)
	assert.NoError(t, err)

	// Verify SPEC.md on disk was updated with the refined content
	updatedBytes, err := os.ReadFile(specPath)
	assert.NoError(t, err)
	assert.Equal(t, refinedSpec, string(updatedBytes))
}
