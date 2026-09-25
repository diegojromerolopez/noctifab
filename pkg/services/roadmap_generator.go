package services

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/diegojromerolopez/noctifab/pkg/domain"
	"github.com/diegojromerolopez/noctifab/pkg/infrastructure/llm"
	"github.com/diegojromerolopez/noctifab/pkg/infrastructure/prompts"
	"github.com/diegojromerolopez/noctifab/pkg/infrastructure/telemetry"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

type compactionModeKey struct{}

// WithCompactionMode sets the prompt compaction mode into the context.
func WithCompactionMode(ctx context.Context, mode string) context.Context {
	return context.WithValue(ctx, compactionModeKey{}, mode)
}

// CompactionModeFromContext retrieves the prompt compaction mode from context, or empty string.
func CompactionModeFromContext(ctx context.Context) string {
	if v, ok := ctx.Value(compactionModeKey{}).(string); ok {
		return v
	}
	return ""
}

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
	return GenerateRoadmapWithFullConfig(ctx, projectPath, llmClient, renderer, passes, maxUserStories, 0, 0)
}

// GenerateRoadmapWithFullConfig executes a multi-pass Product Manager roadmap generation with user story limits and complexity bounds.
func GenerateRoadmapWithFullConfig(ctx context.Context, projectPath string, llmClient domain.LLMClient, renderer PromptRenderer, passes int, maxUserStories int, minComplexity int, maxComplexity int) (lastErr error) {
	ctx, span := telemetry.Tracer().Start(ctx, "GenerateRoadmap",
		trace.WithAttributes(
			attribute.String("project_path", projectPath),
			attribute.Int("passes", passes),
			attribute.Int("max_stories", maxUserStories),
		))
	defer span.End()

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
	specContent := string(specBytes)

	// If specification has domain sections/tables, deterministically partition into .noctifab/specs/
	if manifest, pErr := PartitionSpecIfNeeded(projectPath); pErr == nil && manifest != nil && len(manifest.Sections) > 0 {
		fmt.Printf("ℹ [Product Manager] Deterministically partitioned SPEC.md into .noctifab/specs/ (%d domain slices, %s)\n", len(manifest.Sections), manifest.CoreFile)
		corePath := filepath.Join(projectPath, ".noctifab", "specs", manifest.CoreFile)
		if coreBytes, rErr := os.ReadFile(corePath); rErr == nil && len(coreBytes) > 0 {
			specContent = string(coreBytes)
		}
	}

	if mode := CompactionModeFromContext(ctx); mode != "" && mode != "none" {
		specContent = llm.CompactMarkdownSpecWithMode(specContent, mode)
	}
	specContent = SliceSpecForRoadmap(specContent)

	legacyFiles, _ := scanLegacyFiles(projectPath)
	legacyBlock := ""
	if len(legacyFiles) > 0 {
		fmt.Printf("ℹ [Product Manager] Legacy codebase detected (%d files). Applying Legacy Stabilization Mandate.\n", len(legacyFiles))
		legacyBlock = fmt.Sprintf("\n\nExisting Legacy Code Files Detected in Workspace:\n- %s\n\nLEGACY STABILIZATION MANDATE: Code already exists in the project workspace. Assume it is legacy code with existing functionality. The primary initial goal is to stabilize it by creating unit and integration characterization tests for existing parts in US-001, and leveraging those tests as safety rails when refactoring the code to match future user story requirements.", strings.Join(legacyFiles, "\n- "))
	}

	effectiveMaxStories := ResolveUserStoryCeiling(specContent, maxUserStories)
	if effectiveMaxStories != maxUserStories && maxUserStories > 0 {
		fmt.Printf("ℹ [Product Manager] Large specification detected (%d bytes). Dynamically scaled user story ceiling from %d to %d stories.\n", len(specContent), maxUserStories, effectiveMaxStories)
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
			Spec:            specContent,
			ExistingStories: strings.Join(existingStories, "\n"),
			LegacyFiles:     legacyBlock,
			MaxUserStories:  effectiveMaxStories,
			MinComplexity:   minComplexity,
			MaxComplexity:   maxComplexity,
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
			var rawStories []RawStoryItem
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
					if filename != "" && content != "" {
						rawStories = append(rawStories, RawStoryItem{
							Filename: filename,
							Content:  content,
						})
					}
				}
			}

			sanitizedStories := SanitizeAndCapStories(projectPath, rawStories, specContent, maxUserStories)
			writtenPaths := make(map[string]bool)
			for _, st := range sanitizedStories {
				targetPath := NormalizeStoryPath(projectPath, st.Filename, st.Content)
				relStory := targetPath
				if r, rErr := filepath.Rel(projectPath, targetPath); rErr == nil {
					relStory = r
				}
				if cErr := ValidateStoryContract(relStory, st.Content); cErr != nil {
					fmt.Printf("⚠️  [Product Manager] Story %s failed contract validation: %v\n", st.Filename, cErr)
				}
				writtenPaths[filepath.Clean(targetPath)] = true
				if err := os.MkdirAll(filepath.Dir(targetPath), 0755); err != nil {
					return fmt.Errorf("failed to create directory for story file %q: %w", targetPath, err)
				}
				if err := os.WriteFile(targetPath, []byte(st.Content), 0644); err != nil {
					return fmt.Errorf("failed to write story file %q: %w", targetPath, err)
				}
				storiesCount++
			}

			// On the final pass, purge any obsolete user story files from prior passes
			if p == passes && len(writtenPaths) > 0 {
				if existingFiles, err := filepath.Glob(filepath.Join(storiesDir, "*.md")); err == nil {
					for _, ef := range existingFiles {
						if !writtenPaths[filepath.Clean(ef)] {
							_ = os.Remove(ef)
						}
					}
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
// delegating to the centralized ScanLegacyFiles scanner.
func scanLegacyFiles(projectPath string) ([]string, error) {
	return ScanLegacyFiles(projectPath)
}

// ResolveUserStoryCeiling dynamically scales the maximum number of user stories based on
// specification length and complexity, ensuring large multi-subsystem specifications (e.g. > 35KB)
// are not artificially capped at an inadequate number of stories.
func ResolveUserStoryCeiling(specContent string, configuredMax int) int {
	if configuredMax > 0 && configuredMax != 5 {
		return configuredMax
	}
	specLen := len(specContent)
	if specLen >= 60000 {
		return 12
	}
	if specLen >= 35000 {
		return 8
	}
	if configuredMax > 0 {
		return configuredMax
	}
	return 5
}
