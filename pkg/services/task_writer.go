package services

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/diegojromerolopez/noctifab/pkg/domain"
)

// WriteTaskMarkdown serializes a domain.Task into a markdown file in projectPath/roadmap/tasks/.
// Filename format: <story_id>-<task_id>-<title_slug>.md
func WriteTaskMarkdown(projectPath, storyPath string, task domain.Task) error {
	if projectPath == "" {
		return nil
	}

	tasksDir := filepath.Join(projectPath, "roadmap", "tasks")
	if err := os.MkdirAll(tasksDir, 0755); err != nil {
		return fmt.Errorf("failed to create tasks directory %q: %w", tasksDir, err)
	}

	storyID := ExtractStoryID(storyPath)
	if storyID == "" {
		storyID = "US-001"
	}

	taskID := task.ID
	if taskID == "" {
		taskID = "task-001"
	}

	slug := ToSlug(task.Title)
	if slug == "" {
		slug = "task-item"
	}

	fileName := fmt.Sprintf("%s-%s-%s.md", storyID, taskID, slug)
	filePath := filepath.Join(tasksDir, fileName)

	var sb strings.Builder
	fmt.Fprintf(&sb, "# Task: %s\n\n", task.Title)
	fmt.Fprintf(&sb, "- **ID**: `%s`\n", task.ID)
	fmt.Fprintf(&sb, "- **Story ID**: `%s`\n", storyID)
	fmt.Fprintf(&sb, "- **Status**: `%s`\n", task.Status)
	fmt.Fprintf(&sb, "- **Change Type**: `%s`\n", task.ChangeType)
	if len(task.DependsOn) > 0 {
		fmt.Fprintf(&sb, "- **Depends On**: `%s`\n", strings.Join(task.DependsOn, "`, `"))
	} else {
		sb.WriteString("- **Depends On**: `[]` \n")
	}
	if len(task.TargetFiles) > 0 {
		fmt.Fprintf(&sb, "- **Target Files**: `%s`\n", strings.Join(task.TargetFiles, "`, `"))
	}
	sb.WriteString("\n## Description\n\n")
	sb.WriteString(task.Description)
	if !strings.HasSuffix(task.Description, "\n") {
		sb.WriteString("\n")
	}

	return os.WriteFile(filePath, []byte(sb.String()), 0644)
}
