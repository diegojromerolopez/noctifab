package services

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/diegojromerolopez/noctifab/pkg/domain"
	"github.com/diegojromerolopez/noctifab/pkg/infrastructure/prompts"
)

// GenerateRoadmap reads SPEC.md from projectPath and any existing user stories under roadmap/,
// invokes the Product Manager Agent to generate or audit/refine user stories with explicit Definitions of Done,
// and saves the updated markdown files to projectPath/roadmap/.
// renderer may be nil, in which case the embedded default templates are used.
func GenerateRoadmap(ctx context.Context, projectPath string, llmClient domain.LLMClient, renderer PromptRenderer) error {
	if renderer == nil {
		renderer = prompts.NewDefaultRenderer()
	}
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

	legacyFiles, _ := scanLegacyFiles(projectPath)
	legacyBlock := ""
	if len(legacyFiles) > 0 {
		legacyBlock = fmt.Sprintf("\n\nExisting Legacy Code Files Detected in Workspace:\n- %s\n\nLEGACY STABILIZATION MANDATE: Code already exists in the project workspace. Assume it is legacy code with existing functionality. The primary initial goal is to stabilize it by creating unit and integration characterization tests for existing parts in US-001, and leveraging those tests as safety rails when refactoring the code to match future user story requirements.", strings.Join(legacyFiles, "\n- "))
	}

	action := "generate"
	if len(existingStories) > 0 {
		action = "audit"
	}
	prompt, err := renderer.Render(prompts.AgentProductManager, action, prompts.ProductManagerPromptData{
		Spec:            string(specBytes),
		ExistingStories: strings.Join(existingStories, "\n"),
		LegacyFiles:     legacyBlock,
	})
	if err != nil {
		return fmt.Errorf("product manager prompt rendering failed: %w", err)
	}
	var lastErr error

	// Propagate the product_manager role so the LLM router selects the
	// correct provider/model override (e.g. qwencloud) for this agent.
	pmCtx := context.WithValue(ctx, "agent_role", "product_manager") //nolint:staticcheck

	for attempt := 0; attempt < 3; attempt++ {
		resp, err := llmClient.Complete(pmCtx, prompt)
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

// scanLegacyFiles walks projectPath and returns relative paths of existing legacy source files,
// ignoring metadata directories, documentation, binary artifacts, and generated roadmap files.
func scanLegacyFiles(projectPath string) ([]string, error) {
	ignoredDirs := map[string]bool{
		".git": true, ".noctifab": true, ".github": true, ".idea": true,
		".vscode": true, ".gemini": true, ".antigravity": true, "roadmap": true,
		"output": true, "dist": true, "target": true, "node_modules": true,
		"vendor": true, "bin": true, "build": true,
	}

	ignoredFiles := map[string]bool{
		"spec.md": true, "readme.md": true, "changelog.md": true,
		"license": true, "version": true, ".gitignore": true,
	}

	ignoredExts := map[string]bool{
		".log": true, ".db": true, ".sqlite": true, ".out": true,
		".o": true, ".exe": true, ".so": true, ".dll": true,
		".dylib": true, ".tmp": true,
	}

	var legacyFiles []string
	err := filepath.WalkDir(projectPath, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}

		rel, err := filepath.Rel(projectPath, path)
		if err != nil {
			return nil
		}

		if d.IsDir() {
			base := filepath.Base(path)
			if ignoredDirs[strings.ToLower(base)] && rel != "." {
				return filepath.SkipDir
			}
			return nil
		}

		baseLower := strings.ToLower(filepath.Base(path))
		if ignoredFiles[baseLower] {
			return nil
		}

		extLower := strings.ToLower(filepath.Ext(path))
		if ignoredExts[extLower] {
			return nil
		}

		legacyFiles = append(legacyFiles, rel)
		return nil
	})

	sort.Strings(legacyFiles)
	return legacyFiles, err
}
