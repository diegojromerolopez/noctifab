package services_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/diegojromerolopez/noctifab/pkg/domain"
	"github.com/diegojromerolopez/noctifab/pkg/services"
	"github.com/stretchr/testify/assert"
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
						"filename": "roadmap/US-001.md",
						"content":  "# US-001 content",
					},
				},
				{
					Tool: "create_story",
					Args: map[string]any{
						"filename": "roadmap/US-002.md",
						"content":  "# US-002 content",
					},
				},
			},
		},
	}

	err = services.GenerateRoadmap(context.Background(), tempDir, mockLLM)
	assert.NoError(t, err)

	// Verify the roadmap files were written
	us1Path := filepath.Join(tempDir, "roadmap", "US-001.md")
	us2Path := filepath.Join(tempDir, "roadmap", "US-002.md")

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
	err = services.GenerateRoadmap(context.Background(), tempDir, mockLL)
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
	err = services.GenerateRoadmap(context.Background(), tempDir, mockLL)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "did not return any valid create_story actions")
}
