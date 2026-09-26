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

type partitionTestMockLLMClient struct {
	Response *domain.LLMResponse
	Err      error
}

func (m *partitionTestMockLLMClient) Complete(ctx context.Context, prompt string) (*domain.LLMResponse, error) {
	return m.Response, m.Err
}

func TestGenerateRoadmap_PartitionsSpec(t *testing.T) {
	tempDir := t.TempDir()

	specContent := `# Multi-Section Spec

## 1. Overview
Core system invariants.

## 2. Directory Layout
- src/main.py

## 6. Commands

### 6.1 String Commands
| Command | Sig |
| :--- | :--- |
| ` + "`GET`" + ` | GET k |
| ` + "`SET`" + ` | SET k v |

### 6.2 List Commands
| Command | Sig |
| :--- | :--- |
| ` + "`LPUSH`" + ` | LPUSH k v |
| ` + "`LPOP`" + ` | LPOP k |

## 12. Definition of Done
All tests pass.
`
	require.NoError(t, os.WriteFile(filepath.Join(tempDir, "SPEC.md"), []byte(specContent), 0644))

	mockClient := &partitionTestMockLLMClient{
		Response: &domain.LLMResponse{
			Actions: []domain.LLMAction{
				{
					Tool: "create_story",
					Args: map[string]interface{}{
						"filename": "roadmap/user-stories/US-001-core.md",
						"content": `# US-001: Core
Goal: implement core and string commands.
`,
					},
				},
			},
		},
	}

	err := services.GenerateRoadmap(context.Background(), tempDir, mockClient, nil)
	require.NoError(t, err)

	// Verify that .noctifab/specs was populated with manifest and slices
	specsDir := filepath.Join(tempDir, ".noctifab", "specs")
	assert.FileExists(t, filepath.Join(specsDir, "manifest.json"))
	assert.FileExists(t, filepath.Join(specsDir, "00_core_invariants.md"))
	assert.FileExists(t, filepath.Join(specsDir, "01_string.md"))
	assert.FileExists(t, filepath.Join(specsDir, "02_list.md"))

	// Verify user story was created
	assert.FileExists(t, filepath.Join(tempDir, "roadmap", "user-stories", "US-001-core.md"))
}

func TestGenerateRoadmap_PartitionsSpecWithCompactionMode(t *testing.T) {
	tempDir := t.TempDir()

	specContent := `# Multi-Section Spec
<!-- HTML comment in spec -->

## 1. Overview
Please note that this is the core system invariants.
---

## 6. Commands

### 6.1 String Commands
<!-- Slice comment -->
| Command | Sig |
| :--- | :--- |
| ` + "`GET`" + ` | GET k |
| ` + "`SET`" + ` | SET k v |
`
	require.NoError(t, os.WriteFile(filepath.Join(tempDir, "SPEC.md"), []byte(specContent), 0644))

	mockClient := &partitionTestMockLLMClient{
		Response: &domain.LLMResponse{
			Actions: []domain.LLMAction{
				{
					Tool: "create_story",
					Args: map[string]interface{}{
						"filename": "roadmap/user-stories/US-001-core.md",
						"content":  "# US-001: Core\n",
					},
				},
			},
		},
	}

	ctx := services.WithCompactionMode(context.Background(), "caveman")
	err := services.GenerateRoadmap(ctx, tempDir, mockClient, nil)
	require.NoError(t, err)

	specsDir := filepath.Join(tempDir, ".noctifab", "specs")
	assert.FileExists(t, filepath.Join(specsDir, "01_string.md"))

	// Verify slice was compacted (comment stripped)
	sliceContent, err := os.ReadFile(filepath.Join(specsDir, "01_string.md"))
	require.NoError(t, err)
	assert.NotContains(t, string(sliceContent), "<!-- Slice comment -->")
	assert.Contains(t, string(sliceContent), "| `GET` | GET k |")

	// Verify core was compacted (comment and divider stripped)
	coreContent, err := os.ReadFile(filepath.Join(specsDir, "00_core_invariants.md"))
	require.NoError(t, err)
	assert.NotContains(t, string(coreContent), "<!-- HTML comment in spec -->")
	assert.NotContains(t, string(coreContent), "\n---\n")
}
