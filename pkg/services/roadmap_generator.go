package services

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/diegojromerolopez/noctifab/pkg/domain"
	"github.com/diegojromerolopez/noctifab/pkg/infrastructure/prompts"
)

// GenerateRoadmap reads SPEC.md from projectPath and any existing user stories under roadmap/,
// invokes the Product Manager Agent to generate or audit/refine user stories with explicit Definitions of Done,
// and saves the updated markdown files to projectPath/roadmap/.
// renderer may be nil, in which case the embedded default templates are used.
func GenerateRoadmap(ctx context.Context, projectPath string, llmClient domain.LLMClient, renderer PromptRenderer) error {
	return GenerateRoadmapWithConfig(ctx, projectPath, llmClient, renderer, 1, 0)
}

// GenerateRoadmapWithPasses executes a multi-pass Product Manager roadmap generation & audit loop.
// Pass 1 generates initial user stories; Passes 2+ perform cross-story audits to refine contracts and dependencies.
func GenerateRoadmapWithPasses(ctx context.Context, projectPath string, llmClient domain.LLMClient, renderer PromptRenderer, passes int) error {
	return GenerateRoadmapWithConfig(ctx, projectPath, llmClient, renderer, passes, 0)
}

// GenerateRoadmapWithConfig executes a multi-pass Product Manager roadmap generation with an optional max user stories ceiling.
func GenerateRoadmapWithConfig(ctx context.Context, projectPath string, llmClient domain.LLMClient, renderer PromptRenderer, passes int, maxUserStories int) (lastErr error) {
	if passes <= 0 {
		passes = 1
	}

	pmStart := time.Now()
	if obs := domain.ObserverFromContext(ctx); obs != nil {
		obs.Observe(ctx, domain.ExecutionEvent{
			Kind:      domain.EventAgentStarted,
			AgentRole: "product_manager",
			At:        pmStart.UTC(),
		})
	}
	defer func() {
		durMS := time.Since(pmStart).Milliseconds()
		if obs := domain.ObserverFromContext(ctx); obs != nil {
			outcome := domain.OutcomeSuccess
			if lastErr != nil {
				outcome = domain.OutcomeFailed
			}
			obs.Observe(ctx, domain.ExecutionEvent{
				Kind:           domain.EventAgentFinished,
				AgentRole:      "product_manager",
				At:             time.Now().UTC(),
				DurationMillis: &durMS,
				Outcome:        outcome,
			})
		}
	}()
	if renderer == nil {
		renderer = prompts.NewDefaultRenderer()
	}
	specPath := filepath.Join(projectPath, "SPEC.md")
	specBytes, err := os.ReadFile(specPath)
	if err != nil {
		return fmt.Errorf("SPEC.md not found in project path %q: %w", projectPath, err)
	}

	legacyFiles, _ := scanLegacyFiles(projectPath)
	legacyBlock := ""
	if len(legacyFiles) > 0 {
		legacyBlock = fmt.Sprintf("\n\nExisting Legacy Code Files Detected in Workspace:\n- %s\n\nLEGACY STABILIZATION MANDATE: Code already exists in the project workspace. Assume it is legacy code with existing functionality. The primary initial goal is to stabilize it by creating unit and integration characterization tests for existing parts in US-001, and leveraging those tests as safety rails when refactoring the code to match future user story requirements.", strings.Join(legacyFiles, "\n- "))
	}

	for p := 1; p <= passes; p++ {
		storiesDir := filepath.Join(projectPath, "roadmap", "user-stories")
		var existingStories []string
		if matches, err := filepath.Glob(filepath.Join(storiesDir, "*.md")); err == nil {
			for _, match := range matches {
				rel, _ := filepath.Rel(projectPath, match)
				content, _ := os.ReadFile(match)
				existingStories = append(existingStories, fmt.Sprintf("=== File: %s ===\n%s\n", rel, string(content)))
			}
		}

		action := "generate"
		if len(existingStories) > 0 {
			action = "audit"
		}
		rendered, err := renderer.Render(prompts.AgentProductManager, action, prompts.ProductManagerPromptData{
			Spec:            string(specBytes),
			ExistingStories: strings.Join(existingStories, "\n"),
			LegacyFiles:     legacyBlock,
			MaxUserStories:  maxUserStories,
		})
		if err != nil {
			return fmt.Errorf("product manager prompt rendering failed (pass %d/%d): %w", p, passes, err)
		}
		prompt := rendered.Full()

		pmCtx := context.WithValue(ctx, "agent_role", "product_manager") //nolint:staticcheck
		pmCtx = domain.WithUncompactableTail(pmCtx, len(rendered.Contract))

		passSuccess := false
		for attempt := 0; attempt < 3; attempt++ {
			resp, err := llmClient.Complete(pmCtx, prompt)
			if err != nil {
				lastErr = err
				continue
			}

			if err := os.MkdirAll(storiesDir, 0755); err != nil {
				return fmt.Errorf("failed to create roadmap stories directory %q: %w", storiesDir, err)
			}

			storiesCount := 0
			specRefined := false
			for _, act := range resp.Actions {
				if act.Tool == "refine_spec" {
					content, _ := act.Args["content"].(string)
					if content == "" {
						content, _ = act.Args["spec"].(string)
					}
					if strings.TrimSpace(content) != "" && strings.TrimSpace(content) != strings.TrimSpace(string(specBytes)) {
						if err := os.WriteFile(specPath, []byte(content), 0644); err == nil {
							specBytes = []byte(content)
							specRefined = true
							fmt.Printf("ℹ [Product Manager] Refined and updated SPEC.md with resolved inconsistencies/missing details\n")
						}
					}
				}
				if act.Tool == "create_story" {
					filename, _ := act.Args["filename"].(string)
					content, _ := act.Args["content"].(string)
					if filename == "" || content == "" {
						continue
					}

					targetPath := NormalizeStoryPath(projectPath, filename, content)

					if err := os.MkdirAll(filepath.Dir(targetPath), 0755); err != nil {
						return fmt.Errorf("failed to create directory for story file %q: %w", targetPath, err)
					}

					if err := os.WriteFile(targetPath, []byte(content), 0644); err != nil {
						return fmt.Errorf("failed to write story file %q: %w", targetPath, err)
					}
					storiesCount++
				}
			}

			if storiesCount > 0 || specRefined {
				passSuccess = true
				if passes > 1 {
					fmt.Printf("ℹ [Product Manager] Completed pass %d/%d (wrote/refined %d user story files)\n", p, passes, storiesCount)
				}
				break
			}
			lastErr = fmt.Errorf("LLM did not return any valid create_story or refine_spec actions on pass %d/%d", p, passes)
		}

		if !passSuccess {
			return fmt.Errorf("roadmap generation failed on pass %d/%d: %w", p, passes, lastErr)
		}
	}

	return nil
}

// NormalizeStoryPath normalizes user story paths to roadmap/user-stories/ and appends a title slug if missing.
func NormalizeStoryPath(projectPath, filename, content string) string {
	cleaned := filepath.Clean(filename)
	var targetPath string
	if filepath.IsAbs(cleaned) {
		targetPath = cleaned
	} else {
		targetPath = filepath.Join(projectPath, cleaned)
	}

	// If target file already exists on disk, update it in-place
	if info, err := os.Stat(targetPath); err == nil && !info.IsDir() {
		return targetPath
	}

	if !filepath.IsAbs(cleaned) {
		if strings.HasPrefix(cleaned, "roadmap/US-") || strings.HasPrefix(cleaned, "US-") {
			base := filepath.Base(cleaned)
			cleaned = filepath.Join("roadmap", "user-stories", base)
		} else if !strings.HasPrefix(cleaned, "roadmap/") {
			cleaned = filepath.Join("roadmap", "user-stories", cleaned)
		}
	}

	dir := filepath.Dir(cleaned)
	base := filepath.Base(cleaned)
	ext := filepath.Ext(base)
	nameNoExt := strings.TrimSuffix(base, ext)

	if isPureID(nameNoExt) && content != "" {
		slug := ExtractTitleSlug(content)
		if slug != "" {
			nameNoExt = nameNoExt + "-" + slug
		}
		cleaned = filepath.Join(dir, nameNoExt+ext)
	}

	if filepath.IsAbs(cleaned) {
		return cleaned
	}
	return filepath.Join(projectPath, cleaned)
}

func isPureID(name string) bool {
	upper := strings.ToUpper(name)
	if strings.HasPrefix(upper, "US-") {
		rest := upper[3:]
		isNum := true
		for _, r := range rest {
			if r < '0' || r > '9' {
				isNum = false
				break
			}
		}
		return isNum
	}
	return false
}

// ExtractTitleSlug extracts the first markdown heading (# ...) title and converts it to a kebab-case slug.
func ExtractTitleSlug(content string) string {
	lines := strings.Split(content, "\n")
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "# ") {
			title := strings.TrimPrefix(trimmed, "# ")
			if idx := strings.Index(title, ":"); idx != -1 {
				title = title[idx+1:]
			}
			title = strings.TrimSpace(title)
			return ToSlug(title)
		}
	}
	return ""
}

// ToSlug converts a text string into a clean URL/filename-safe kebab-case slug (max 50 chars).
func ToSlug(text string) string {
	text = strings.ToLower(text)
	var sb strings.Builder
	inHyphen := false
	for _, r := range text {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			sb.WriteRune(r)
			inHyphen = false
		} else if !inHyphen && sb.Len() > 0 {
			sb.WriteRune('-')
			inHyphen = true
		}
	}
	res := strings.Trim(sb.String(), "-")
	if len(res) > 50 {
		res = strings.Trim(res[:50], "-")
	}
	return res
}

// scanLegacyFiles walks projectPath and returns relative paths of existing legacy source files,
// ignoring metadata directories, documentation, binary artifacts, and generated roadmap files.
func scanLegacyFiles(projectPath string) ([]string, error) {
	ignoredDirs := map[string]bool{
		".git": true, ".noctifab": true, ".github": true, ".idea": true,
		".vscode": true, ".gemini": true, ".antigravity": true, "roadmap": true,
		"user-stories": true, "tasks": true,
		"output": true, "dist": true, "target": true, "node_modules": true,
		"vendor": true, "bin": true, "build": true, "obj": true,
		".venv": true, "venv": true, "env": true, "__pycache__": true,
		".mypy_cache": true, ".ruff_cache": true, ".pytest_cache": true,
		".next": true, ".nuxt": true, ".turbo": true, ".cache": true,
		".gradle": true, ".mvn": true, "out": true, "_build": true, "deps": true,
		"cmake-build-debug": true, "cmake-build-release": true,
	}

	ignoredFiles := map[string]bool{
		"spec.md": true, "readme.md": true, "changelog.md": true,
		"license": true, "version": true, ".gitignore": true,
		"dockerfile": true, "docker-compose.yml": true, "docker-compose.yaml": true,
		"makefile": true, ".readthedocs.yaml": true, ".readthedocs.yml": true,
		"uv.lock": true, "poetry.lock": true, "package-lock.json": true,
		"pnpm-lock.yaml": true, "yarn.lock": true, "cargo.lock": true,
		"go.sum": true, "gemfile.lock": true, "composer.lock": true, "mix.lock": true,
		"noctifab_evaluation_report.md": true,
	}

	ignoredExts := map[string]bool{
		".log": true, ".db": true, ".sqlite": true, ".out": true,
		".o": true, ".exe": true, ".so": true, ".dll": true,
		".dylib": true, ".tmp": true, ".pyc": true, ".pyo": true,
		".a": true, ".rlib": true, ".class": true, ".jar": true,
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
