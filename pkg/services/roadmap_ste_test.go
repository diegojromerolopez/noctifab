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

type capturingMockObserver struct {
	events []domain.ExecutionEvent
}

func (c *capturingMockObserver) Observe(ctx context.Context, ev domain.ExecutionEvent) {
	c.events = append(c.events, ev)
}

func (c *capturingMockObserver) Finish(ctx context.Context, outcome domain.ExecutionOutcome) {}

func TestResolveSpecPath(t *testing.T) {
	t.Run("when SPEC.ste.md does not exist, it resolves to SPEC.md", func(t *testing.T) {
		tmpDir := t.TempDir()
		specPath := filepath.Join(tmpDir, "SPEC.md")
		require.NoError(t, os.WriteFile(specPath, []byte("# Test Spec"), 0644))

		resolved := services.ResolveSpecPath(tmpDir)
		assert.Equal(t, specPath, resolved)
	})

	t.Run("when SPEC.ste.md exists, it resolves to SPEC.ste.md", func(t *testing.T) {
		tmpDir := t.TempDir()
		specPath := filepath.Join(tmpDir, "SPEC.md")
		stePath := filepath.Join(tmpDir, "SPEC.ste.md")
		require.NoError(t, os.WriteFile(specPath, []byte("# Test Spec"), 0644))
		require.NoError(t, os.WriteFile(stePath, []byte("# STE Spec"), 0644))

		resolved := services.ResolveSpecPath(tmpDir)
		assert.Equal(t, stePath, resolved)
	})
}

func TestEnsureSTESpecification(t *testing.T) {
	t.Run("when SPEC.md is missing, it returns an error", func(t *testing.T) {
		tmpDir := t.TempDir()
		_, err := services.EnsureSTESpecification(context.Background(), tmpDir, nil, nil)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "SPEC.md not found")
	})

	t.Run("when SPEC.ste.md already exists, it reads directly without invoking LLM", func(t *testing.T) {
		tmpDir := t.TempDir()
		stePath := filepath.Join(tmpDir, "SPEC.ste.md")
		existingContent := "# Existing STE Specification\n\n- Short sentences only."
		require.NoError(t, os.WriteFile(stePath, []byte(existingContent), 0644))

		callingMock := &mockRoadmapLLMClient{
			Err: assert.AnError, // Should not be called
		}

		content, err := services.EnsureSTESpecification(context.Background(), tmpDir, callingMock, nil)
		require.NoError(t, err)
		assert.Equal(t, existingContent, content)
	})

	t.Run("when SPEC.ste.md is absent, it translates SPEC.md via LLM and preserves SPEC.md", func(t *testing.T) {
		tmpDir := t.TempDir()
		specPath := filepath.Join(tmpDir, "SPEC.md")
		humanSpec := "# Human Spec\n\nThis is a rather long descriptive paragraph that explains how the system works."
		require.NoError(t, os.WriteFile(specPath, []byte(humanSpec), 0644))

		steSpec := "# Project Specification in ASD-STE100\n\n## 1. Overview\nThis section explains the system."
		mockLLM := &mockRoadmapLLMClient{
			Response: &domain.LLMResponse{
				Actions: []domain.LLMAction{
					{
						Tool: "translate_ste",
						Args: map[string]any{
							"content": steSpec,
						},
					},
				},
			},
		}

		obs := &capturingMockObserver{}
		ctx := domain.WithObserver(context.Background(), obs)

		content, err := services.EnsureSTESpecification(ctx, tmpDir, mockLLM, nil)
		require.NoError(t, err)
		assert.Equal(t, steSpec, content)

		// Verify SPEC.ste.md was written
		stePath := filepath.Join(tmpDir, "SPEC.ste.md")
		steData, err := os.ReadFile(stePath)
		require.NoError(t, err)
		assert.Equal(t, steSpec, string(steData))

		// Verify human ground truth SPEC.md was NOT overwritten
		specData, err := os.ReadFile(specPath)
		require.NoError(t, err)
		assert.Equal(t, humanSpec, string(specData))

		// Verify observer events
		assert.True(t, len(obs.events) >= 2)
		assert.Equal(t, "translate_spec_to_ste", obs.events[0].Name)
	})

	t.Run("when LLM returns refine_spec tool action, it extracts content and preserves SPEC.md", func(t *testing.T) {
		tmpDir := t.TempDir()
		specPath := filepath.Join(tmpDir, "SPEC.md")
		humanSpec := "# Human Spec Original"
		require.NoError(t, os.WriteFile(specPath, []byte(humanSpec), 0644))

		steSpec := "# Refined STE Spec"
		mockLLM := &mockRoadmapLLMClient{
			Response: &domain.LLMResponse{
				Actions: []domain.LLMAction{
					{
						Tool: "refine_spec",
						Args: map[string]any{
							"content": steSpec,
						},
					},
				},
			},
		}

		content, err := services.EnsureSTESpecification(context.Background(), tmpDir, mockLLM, nil)
		require.NoError(t, err)
		assert.Equal(t, steSpec, content)

		// Verify SPEC.md untouched
		specData, _ := os.ReadFile(specPath)
		assert.Equal(t, humanSpec, string(specData))
	})

	t.Run("when LLM returns markdown in reasoning, it extracts markdown", func(t *testing.T) {
		tmpDir := t.TempDir()
		specPath := filepath.Join(tmpDir, "SPEC.md")
		humanSpec := "# Human Spec"
		require.NoError(t, os.WriteFile(specPath, []byte(humanSpec), 0644))

		fencedMarkdown := "```markdown\n# STE Specification\n\nDo task 1.\n```"
		mockLLM := &mockRoadmapLLMClient{
			Response: &domain.LLMResponse{
				Reasoning: fencedMarkdown,
				Actions:   []domain.LLMAction{},
			},
		}

		content, err := services.EnsureSTESpecification(context.Background(), tmpDir, mockLLM, nil)
		require.NoError(t, err)
		assert.Equal(t, "# STE Specification\n\nDo task 1.", content)
	})

	t.Run("when LLM fails or is nil, it falls back to specContent and creates SPEC.ste.md", func(t *testing.T) {
		tmpDir := t.TempDir()
		specPath := filepath.Join(tmpDir, "SPEC.md")
		humanSpec := "# Human Spec"
		require.NoError(t, os.WriteFile(specPath, []byte(humanSpec), 0644))

		content, err := services.EnsureSTESpecification(context.Background(), tmpDir, nil, nil)
		require.NoError(t, err)
		assert.Equal(t, humanSpec, content)

		stePath := filepath.Join(tmpDir, "SPEC.ste.md")
		steData, err := os.ReadFile(stePath)
		require.NoError(t, err)
		assert.Equal(t, humanSpec, string(steData))
	})
}
