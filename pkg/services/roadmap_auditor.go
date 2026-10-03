package services

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
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
		fmt.Fprintf(&sb, "- **%s: %s** (File: %s | DependsOn: [%s])\n",
			s.ID, s.Title, s.Filename, deps)
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

// AuditRoadmapStoriesPerStory audits and refines each user story individually in isolated concurrent LLM requests.
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
	return AuditRoadmapStoriesParallel(ctx, projectPath, storyFiles, specContent, legacyBlock, llmClient, renderer, 4, onStoryReady)
}

// AuditRoadmapStoriesParallel audits and refines user stories concurrently using a bounded worker pool.
func AuditRoadmapStoriesParallel(
	ctx context.Context,
	projectPath string,
	storyFiles []string,
	specContent string,
	legacyBlock string,
	llmClient domain.LLMClient,
	renderer PromptRenderer,
	concurrency int,
	onStoryReady func(path, content string),
) (int, error) {
	ctx, span := telemetry.Tracer().Start(ctx, "AuditRoadmapStoriesPerStory",
		trace.WithAttributes(attribute.Int("total_stories", len(storyFiles))),
	)
	defer span.End()

	if concurrency <= 0 {
		concurrency = 4
	}

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
	specPath := ResolveSpecPath(projectPath)
	specBytes, _ := os.ReadFile(specPath)

	pmCtx := context.WithValue(ctx, "agent_role", "product_manager") //nolint:staticcheck

	var mu sync.Mutex
	refinedCount := 0
	sem := make(chan struct{}, concurrency)
	var wg sync.WaitGroup

	for i, item := range items {
		wg.Add(1)
		sem <- struct{}{}
		go func(idx int, stItem StoryAuditItem) {
			defer func() {
				<-sem
				wg.Done()
			}()

			// Use targeted domain slice + core invariants to avoid full-spec token bloat
			storySpec := BuildBasicAndFeatureSpec(projectPath, stItem.Title, specContent)
			if len(storySpec) > 20000 {
				storySpec = SliceSpecForRoadmap(storySpec)
			}

			targetStoryPayload := stItem.Content
			relCheck, rErr := filepath.Rel(projectPath, stItem.Path)
			if rErr != nil || strings.HasPrefix(relCheck, "..") {
				relCheck = filepath.Join("roadmap", "user-stories", stItem.Filename)
			}
			cErr := ValidateStoryContract(relCheck, stItem.Content)
			auditMode := domain.AuditModeFromContext(ctx)
			isSmart := auditMode == "" || strings.EqualFold(auditMode, "smart") || strings.EqualFold(auditMode, "adaptive")
			if cErr != nil {
				targetStoryPayload = stItem.Content + "\n\n[Contract Validation Error Requiring Repair: " + cErr.Error() + "]"
			} else if isSmart && strings.Contains(stItem.Content, "Definition of Done") && !IsSyntheticStoryContent(stItem.Content) {
				// Fast path: story already satisfies contract and DoD validation without placeholders.
				// Skip expensive LLM call and retain the verified story as-is.
				fmt.Printf("ℹ [Product Manager] Story %s already satisfies contract & DoD validation; skipping LLM audit (%d/%d)\n", stItem.ID, idx+1, len(items))
				if onStoryReady != nil {
					onStoryReady(stItem.Path, stItem.Content)
				}
				return
			}

			rendered, err := renderer.Render(prompts.AgentProductManager, "audit", prompts.ProductManagerPromptData{
				Spec:           storySpec,
				TargetStory:    targetStoryPayload,
				RoadmapCatalog: catalog,
				LegacyFiles:    legacyBlock,
			})
			if err != nil {
				fmt.Printf("⚠️  [Product Manager] Story %s prompt rendering failed: %v\n", stItem.ID, err)
				return
			}

			prompt := rendered.Full()
			itemCtx := domain.WithUncompactableTail(pmCtx, len(rendered.Contract))

			callCtx, cancel := context.WithTimeout(itemCtx, 120*time.Second)
			resp, err := llmClient.Complete(callCtx, prompt)
			cancel()

			audited := false
			if err == nil && resp != nil {
				for _, act := range resp.Actions {
					if act.Tool == "refine_spec" {
						content, _ := act.Args["content"].(string)
						if content == "" {
							content, _ = act.Args["spec"].(string)
						}
						mu.Lock()
						if strings.TrimSpace(content) != "" && strings.TrimSpace(content) != strings.TrimSpace(string(specBytes)) {
							stePath := filepath.Join(projectPath, "SPEC.ste.md")
							if wErr := os.WriteFile(stePath, []byte(content), 0644); wErr == nil {
								specBytes = []byte(content)
								fmt.Printf("ℹ [Product Manager] Refined and updated SPEC.ste.md with resolved inconsistencies/missing details\n")
							}
						}
						mu.Unlock()
					}
					if act.Tool == "create_story" {
						filename, _ := act.Args["filename"].(string)
						content, _ := act.Args["content"].(string)
						if filename == "" {
							filename = stItem.Filename
						}
						if content != "" {
							relPath, rErr := filepath.Rel(projectPath, stItem.Path)
							if rErr != nil || strings.HasPrefix(relPath, "..") {
								relPath = filepath.Join("roadmap", "user-stories", filename)
							}
							if cErr := ValidateStoryContract(relPath, content); cErr != nil {
								fmt.Printf("⚠️  [Product Manager] Story %s audited content failed contract validation: %v\n", stItem.ID, cErr)
							}
							targetPath := NormalizeStoryPath(projectPath, filename, content)
							mu.Lock()
							_ = os.MkdirAll(filepath.Dir(targetPath), 0755)
							_ = os.WriteFile(targetPath, []byte(content), 0644)
							mu.Unlock()
							if onStoryReady != nil {
								onStoryReady(targetPath, content)
							}
							audited = true
						}
					}
				}
			}

			mu.Lock()
			if audited {
				refinedCount++
				fmt.Printf("ℹ [Product Manager] Audited & refined story %s (%d/%d)\n", stItem.ID, idx+1, len(items))
			} else {
				fmt.Printf("ℹ [Product Manager] Story %s retained without modification (%d/%d)\n", stItem.ID, idx+1, len(items))
			}
			mu.Unlock()
		}(i, item)
	}

	wg.Wait()
	return refinedCount, nil
}

// IsSyntheticStoryContent checks whether a story contains synthetic fallback templates or dummy placeholders.
func IsSyntheticStoryContent(content string) bool {
	lower := strings.ToLower(content)
	if strings.Contains(lower, "cli or socket") {
		return true
	}
	if strings.Contains(lower, ".baseline") {
		return true
	}
	if strings.Contains(lower, "real working implementation with zero stubs or placeholders") {
		return true
	}
	return false
}
