package services

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/diegojromerolopez/noctifab/pkg/domain"
)

// GenerateRoadmap reads SPEC.md from projectPath and any existing user stories under roadmap/,
// invokes the Product Manager Agent to generate or audit/refine user stories with explicit Definitions of Done,
// and saves the updated markdown files to projectPath/roadmap/.
func GenerateRoadmap(ctx context.Context, projectPath string, llmClient domain.LLMClient) error {
	specPath := filepath.Join(projectPath, "SPEC.md")
	specBytes, err := os.ReadFile(specPath)
	if err != nil {
		return fmt.Errorf("SPEC.md not found in project path %q: %w", projectPath, err)
	}

	roadmapDir := filepath.Join(projectPath, "roadmap")
	var existingStories []string
	if matches, err := filepath.Glob(filepath.Join(roadmapDir, "*.md")); err == nil && len(matches) > 0 {
		for _, match := range matches {
			rel, _ := filepath.Rel(projectPath, match)
			content, _ := os.ReadFile(match)
			existingStories = append(existingStories, fmt.Sprintf("=== File: %s ===\n%s\n", rel, string(content)))
		}
	}

	var prompt string
	if len(existingStories) > 0 {
		prompt = fmt.Sprintf("Audit and refine existing user stories to ensure complete Definition of Done (DoD), edge cases, and interface contracts:\n\nSpecification:\n%s\n\nExisting User Stories:\n%s", string(specBytes), strings.Join(existingStories, "\n"))
	} else {
		prompt = fmt.Sprintf("Generate detailed user stories from specification:\n\n%s", string(specBytes))
	}
	var lastErr error

	for attempt := 0; attempt < 3; attempt++ {
		resp, err := llmClient.Complete(ctx, prompt)
		if err != nil {
			lastErr = err
			continue
		}

		roadmapDir := filepath.Join(projectPath, "roadmap")
		if err := os.MkdirAll(roadmapDir, 0755); err != nil {
			return fmt.Errorf("failed to create roadmap directory %q: %w", roadmapDir, err)
		}

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

		if storiesCount > 0 {
			return nil
		}
		lastErr = fmt.Errorf("LLM did not return any valid create_story actions")
	}

	return fmt.Errorf("roadmap generation failed: %w", lastErr)
}
