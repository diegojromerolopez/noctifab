package services

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/diegojromerolopez/noctifab/pkg/domain"
	"github.com/diegojromerolopez/noctifab/pkg/infrastructure/prompts"
	"github.com/diegojromerolopez/noctifab/pkg/infrastructure/telemetry"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

// StoryAuditItem represents a user story identified for per-story auditing.
type StoryAuditItem struct {
	Path     string
	Filename string
	Content  string
	ID       string
	Title    string
}

// BuildRoadmapCatalogFromStories builds a concise summary catalog from existing story files.
func BuildRoadmapCatalogFromStories(stories []StoryAuditItem) string {
	var sb strings.Builder
	for _, s := range stories {
		depsList := ParseStoryDependencies(s.Content)
		deps := "none"
		if len(depsList) > 0 {
			deps = strings.Join(depsList, ", ")
		}
		sb.WriteString(fmt.Sprintf("- **%s: %s** (File: %s | DependsOn: [%s])\n",
			s.ID, s.Title, s.Filename, deps))
	}
	return sb.String()
}

// ParseStoryAuditItem extracts ID, title, and file content from a story file.
func ParseStoryAuditItem(path string) (StoryAuditItem, error) {
	bytes, err := os.ReadFile(path)
	if err != nil {
		return StoryAuditItem{}, err
	}
	content := string(bytes)
	filename := filepath.Base(path)

	id := ""
	title := filename
	parts := strings.Split(filename, "-")
	if len(parts) >= 2 {
		id = strings.ToUpper(parts[0] + "-" + parts[1])
	}

	lines := strings.Split(content, "\n")
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "# ") {
			title = strings.TrimPrefix(trimmed, "# ")
			break
		}
	}

	return StoryAuditItem{
		Path:     path,
		Filename: filename,
		Content:  content,
		ID:       id,
		Title:    title,
	}, nil
}

// AuditRoadmapStoriesPerStory audits and refines each user story individually in isolated LLM requests.
func AuditRoadmapStoriesPerStory(
	ctx context.Context,
	projectPath string,
	storyFiles []string,
	specContent string,
	legacyBlock string,
	llmClient domain.LLMClient,
	renderer PromptRenderer,
	onStoryReady func(path, content string),
) (int, error) {
	ctx, span := telemetry.Tracer().Start(ctx, "AuditRoadmapStoriesPerStory",
		trace.WithAttributes(attribute.Int("total_stories", len(storyFiles))),
	)
	defer span.End()

	var items []StoryAuditItem
	for _, f := range storyFiles {
		item, err := ParseStoryAuditItem(f)
		if err == nil && item.Content != "" {
			items = append(items, item)
		}
	}

	if len(items) == 0 {
		return 0, nil
	}

	catalog := BuildRoadmapCatalogFromStories(items)
	specPath := filepath.Join(projectPath, "SPEC.md")
	specBytes, _ := os.ReadFile(specPath)

	pmCtx := context.WithValue(ctx, "agent_role", "product_manager") //nolint:staticcheck
	refinedCount := 0

	for i, item := range items {
		// Use sliced spec or fallback to outline spec
		storySpec := specContent
		if len(specContent) > 25000 {
			storySpec = BuildRoadmapOutlineSpec(projectPath, specContent)
		}

		rendered, err := renderer.Render(prompts.AgentProductManager, "audit", prompts.ProductManagerPromptData{
			Spec:           storySpec,
			TargetStory:    item.Content,
			RoadmapCatalog: catalog,
			LegacyFiles:    legacyBlock,
		})
		if err != nil {
			fmt.Printf("⚠️  [Product Manager] Story %s prompt rendering failed: %v\n", item.ID, err)
			continue
		}

		prompt := rendered.Full()
		itemCtx := domain.WithUncompactableTail(pmCtx, len(rendered.Contract))

		audited := false
		for attempt := 0; attempt < 2; attempt++ {
			callCtx, cancel := context.WithTimeout(itemCtx, 120*time.Second)
			resp, err := llmClient.Complete(callCtx, prompt)
			cancel()
			if err != nil {
				continue
			}

			for _, act := range resp.Actions {
				if act.Tool == "refine_spec" {
					content, _ := act.Args["content"].(string)
					if content == "" {
						content, _ = act.Args["spec"].(string)
					}
					if strings.TrimSpace(content) != "" && strings.TrimSpace(content) != strings.TrimSpace(string(specBytes)) {
						if err := os.WriteFile(specPath, []byte(content), 0644); err == nil {
							specBytes = []byte(content)
							fmt.Printf("ℹ [Product Manager] Refined and updated SPEC.md with resolved inconsistencies/missing details\n")
						}
					}
				}
				if act.Tool == "create_story" {
					filename, _ := act.Args["filename"].(string)
					content, _ := act.Args["content"].(string)
					if filename == "" {
						filename = item.Filename
					}
					if content != "" {
						relPath, rErr := filepath.Rel(projectPath, item.Path)
						if rErr != nil || strings.HasPrefix(relPath, "..") {
							relPath = filepath.Join("roadmap", "user-stories", filename)
						}
						if cErr := ValidateStoryContract(relPath, content); cErr != nil {
							fmt.Printf("⚠️  [Product Manager] Story %s audited content failed contract validation: %v\n", item.ID, cErr)
						}
						targetPath := NormalizeStoryPath(projectPath, filename, content)
						_ = os.MkdirAll(filepath.Dir(targetPath), 0755)
						_ = os.WriteFile(targetPath, []byte(content), 0644)
						if onStoryReady != nil {
							onStoryReady(targetPath, content)
						}
						audited = true
					}
				}
			}

			if audited {
				refinedCount++
				fmt.Printf("ℹ [Product Manager] Audited & refined story %s (%d/%d)\n", item.ID, i+1, len(items))
				break
			}
		}

		if !audited {
			fmt.Printf("ℹ [Product Manager] Story %s retained without modification (%d/%d)\n", item.ID, i+1, len(items))
		}
	}

	return refinedCount, nil
}
