package services

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/diegojromerolopez/noctifab/pkg/domain"
)

// GenerateRoadmap reads SPEC.md from projectPath, invokes the Product Manager Agent to generate
// a roadmap of user stories, and writes the markdown files to projectPath/roadmap/.
func GenerateRoadmap(ctx context.Context, projectPath string, llmClient domain.LLMClient) error {
	specPath := filepath.Join(projectPath, "SPEC.md")
	specBytes, err := os.ReadFile(specPath)
	if err != nil {
		return fmt.Errorf("SPEC.md not found in project path %q: %w", projectPath, err)
	}

	prompt := fmt.Sprintf("Generate detailed user stories from specification:\n\n%s", string(specBytes))
	resp, err := llmClient.Complete(ctx, prompt)
	if err != nil {
		return fmt.Errorf("failed to complete LLM call for roadmap generation: %w", err)
	}

	roadmapDir := filepath.Join(projectPath, "roadmap")
	if err := os.MkdirAll(roadmapDir, 0755); err != nil {
		return fmt.Errorf("failed to create roadmap directory %q: %w", roadmapDir, err)
	}

	// Process the actions (which should be "create_story")
	storiesCount := 0
	for _, action := range resp.Actions {
		if action.Tool == "create_story" {
			filename, _ := action.Args["filename"].(string)
			content, _ := action.Args["content"].(string)
			if filename == "" || content == "" {
				continue
			}

			// Clean and resolve path
			cleaned := filepath.Clean(filename)
			var targetPath string
			if filepath.IsAbs(cleaned) {
				targetPath = cleaned
			} else {
				targetPath = filepath.Join(projectPath, cleaned)
			}

			// Ensure parent dir exists
			if err := os.MkdirAll(filepath.Dir(targetPath), 0755); err != nil {
				return fmt.Errorf("failed to create directory for story file %q: %w", targetPath, err)
			}

			if err := os.WriteFile(targetPath, []byte(content), 0644); err != nil {
				return fmt.Errorf("failed to write story file %q: %w", targetPath, err)
			}
			storiesCount++
		}
	}

	if storiesCount == 0 {
		return fmt.Errorf("LLM did not return any valid create_story actions")
	}

	return nil
}
