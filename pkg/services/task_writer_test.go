package services_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/diegojromerolopez/noctifab/pkg/domain"
	"github.com/diegojromerolopez/noctifab/pkg/services"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWriteTaskMarkdown(t *testing.T) {
	tempDir := t.TempDir()

	task := domain.Task{
		ID:          "task-035acd76",
		Title:       "Project Scaffolding and Environment Configuration",
		Description: "Set up project structure, pyproject.toml, Makefile, and RTD documentation.",
		Status:      domain.TaskPending,
		ChangeType:  domain.ChangeTypeFeature,
		DependsOn:   []string{"US-001"},
		TargetFiles: []string{"pyproject.toml", "Makefile"},
	}

	err := services.WriteTaskMarkdown(tempDir, "roadmap/user-stories/US-001-framing.md", task)
	require.NoError(t, err)

	expectedPath := filepath.Join(tempDir, "roadmap", "tasks", "US-001-task-035acd76-project-scaffolding-and-environment-configuration.md")
	assert.FileExists(t, expectedPath)

	content, err := os.ReadFile(expectedPath)
	require.NoError(t, err)

	strContent := string(content)
	assert.Contains(t, strContent, "# Task: Project Scaffolding and Environment Configuration")
	assert.Contains(t, strContent, "- **ID**: `task-035acd76`")
	assert.Contains(t, strContent, "- **Story ID**: `US-001`")
	assert.Contains(t, strContent, "Set up project structure")
}
