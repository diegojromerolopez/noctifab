package services

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/diegojromerolopez/noctifab/pkg/domain"
	"github.com/diegojromerolopez/noctifab/pkg/infrastructure/prompts"
	"github.com/diegojromerolopez/noctifab/pkg/infrastructure/telemetry"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

// StoryOutlineItem represents a planned user story emitted during Stage 1 roadmap planning.
type StoryOutlineItem struct {
	ID          string   `json:"id"`
	Title       string   `json:"title"`
	Slug        string   `json:"slug"`
	DomainSlice string   `json:"domain_slice"`
	DependsOn   []string `json:"depends_on"`
	Complexity  int      `json:"complexity"`
	Summary     string   `json:"summary"`
}

// ParseStoryOutlines extracts StoryOutlineItem entries from a plan_roadmap LLM action.
func ParseStoryOutlines(act domain.LLMAction) ([]StoryOutlineItem, error) {
	raw, ok := act.Args["stories"]
	if !ok || raw == nil {
		return nil, fmt.Errorf("plan_roadmap action missing 'stories' argument")
	}

	// Marshal and unmarshal to safely decode into structured StoryOutlineItem slice
	bytes, err := json.Marshal(raw)
	if err != nil {
		return nil, fmt.Errorf("failed to serialize stories argument: %w", err)
	}

	var outlines []StoryOutlineItem
	if err := json.Unmarshal(bytes, &outlines); err != nil {
		return nil, fmt.Errorf("failed to parse story outlines JSON: %w", err)
	}

	var valid []StoryOutlineItem
	for _, o := range outlines {
		if strings.TrimSpace(o.ID) == "" {
			continue
		}
		if o.Slug == "" {
			o.Slug = ToSlug(o.Title)
		}
		valid = append(valid, o)
	}

	if len(valid) == 0 {
		return nil, fmt.Errorf("no valid story outline items found in plan_roadmap action")
	}

	return valid, nil
}

// FormatRoadmapCatalog formats a list of StoryOutlineItem into human-readable catalog markdown
// suitable for injecting into Stage 2 expansion prompts.
func FormatRoadmapCatalog(outlines []StoryOutlineItem) string {
	var sb strings.Builder
	for _, o := range outlines {
		deps := "none"
		if len(o.DependsOn) > 0 {
			deps = strings.Join(o.DependsOn, ", ")
		}
		sb.WriteString(fmt.Sprintf("- **%s: %s** (Slug: %s | Complexity: %d CU | DependsOn: [%s])\n",
			o.ID, o.Title, o.Slug, o.Complexity, deps))
		if o.DomainSlice != "" {
			sb.WriteString(fmt.Sprintf("  - Domain Capability: %s\n", o.DomainSlice))
		}
		if o.Summary != "" {
			sb.WriteString(fmt.Sprintf("  - Scope Summary: %s\n", o.Summary))
		}
	}
	return sb.String()
}

// StoryReadyCallback is invoked immediately as each individual story completes Stage 2 expansion.
type StoryReadyCallback func(story RawStoryItem)

// ExpandRoadmapStoriesParallel concurrently expands multiple StoryOutlineItem outlines
// using a bounded worker pool. Results are returned in deterministic outline order.
func ExpandRoadmapStoriesParallel(
	ctx context.Context,
	projectPath string,
	outlines []StoryOutlineItem,
	existingIDs map[string]bool,
	specContent string,
	legacyBlock string,
	existingStories []string,
	llmClient domain.LLMClient,
	renderer PromptRenderer,
	concurrency int,
) []RawStoryItem {
	return ExpandRoadmapStoriesParallelStreaming(ctx, projectPath, outlines, existingIDs, specContent, legacyBlock, existingStories, llmClient, renderer, concurrency, nil)
}

// ExpandRoadmapStoriesParallelStreaming concurrently expands outlines and triggers onStoryReady as each story completes.
func ExpandRoadmapStoriesParallelStreaming(
	ctx context.Context,
	projectPath string,
	outlines []StoryOutlineItem,
	existingIDs map[string]bool,
	specContent string,
	legacyBlock string,
	existingStories []string,
	llmClient domain.LLMClient,
	renderer PromptRenderer,
	concurrency int,
	onStoryReady StoryReadyCallback,
) []RawStoryItem {
	if concurrency <= 0 {
		concurrency = 4
	}

	type toExpandItem struct {
		index int
		item  StoryOutlineItem
	}

	var toExpand []toExpandItem
	for i, out := range outlines {
		if !existingIDs[strings.ToUpper(out.ID)] {
			toExpand = append(toExpand, toExpandItem{index: i, item: out})
		}
	}

	if len(toExpand) == 0 {
		return nil
	}

	results := make([]*RawStoryItem, len(outlines))
	sem := make(chan struct{}, concurrency)
	var wg sync.WaitGroup

	for _, entry := range toExpand {
		wg.Add(1)
		sem <- struct{}{}
		go func(idx int, target StoryOutlineItem) {
			defer func() {
				<-sem
				wg.Done()
			}()

			fmt.Printf("ℹ [Product Manager] Expanding %s (%s) [parallel]...\n", target.ID, target.Title)
			expanded, expErr := ExpandRoadmapStory(ctx, projectPath, target, outlines, specContent, legacyBlock, nil, llmClient, renderer)
			if expErr != nil {
				fmt.Printf("⚠️  [Product Manager] Story expansion error for %s: %v (applying synthetic fallback)\n", target.ID, expErr)
				expanded = &RawStoryItem{
					Filename: fmt.Sprintf("roadmap/user-stories/%s-%s.md", strings.ToUpper(target.ID), target.Slug),
					Content:  buildSyntheticStoryFromOutline(target),
				}
			}
			if expanded != nil {
				results[idx] = expanded
				if onStoryReady != nil {
					onStoryReady(*expanded)
				}
			}
		}(entry.index, entry.item)
	}

	wg.Wait()

	var gathered []RawStoryItem
	for _, res := range results {
		if res != nil {
			gathered = append(gathered, *res)
		}
	}
	return gathered
}

// ExpandRoadmapStory executes Stage 2 story expansion for a single StoryOutlineItem.
// It invokes the Product Manager Agent with the targeted expand prompt and returns the generated RawStoryItem.
func ExpandRoadmapStory(
	ctx context.Context,
	projectPath string,
	target StoryOutlineItem,
	allOutlines []StoryOutlineItem,
	specContent string,
	legacyBlock string,
	existingStories []string,
	llmClient domain.LLMClient,
	renderer PromptRenderer,
) (*RawStoryItem, error) {
	ctx, span := telemetry.Tracer().Start(ctx, "ExpandRoadmapStory",
		trace.WithAttributes(
			attribute.String("story_id", target.ID),
			attribute.String("story_title", target.Title),
		))
	defer span.End()

	if renderer == nil {
		renderer = prompts.NewDefaultRenderer()
	}

	deps := "none"
	if len(target.DependsOn) > 0 {
		deps = strings.Join(target.DependsOn, ", ")
	}
	targetStoryStr := fmt.Sprintf("ID: %s\nTitle: %s\nSlug: %s\nDomain Capability: %s\nDepends On: [%s]\nComplexity: %d CU\nScope Summary: %s",
		target.ID, target.Title, target.Slug, target.DomainSlice, deps, target.Complexity, target.Summary)

	domainQuery := target.DomainSlice
	if domainQuery == "" {
		domainQuery = target.Title
	}
	featureSpec := BuildBasicAndFeatureSpec(projectPath, domainQuery, specContent)

	rendered, err := renderer.Render(prompts.AgentProductManager, "expand", prompts.ProductManagerPromptData{
		Spec:            featureSpec,
		TargetStory:     targetStoryStr,
		RoadmapCatalog:  FormatRoadmapCatalog(allOutlines),
		ExistingStories: strings.Join(existingStories, "\n"),
		LegacyFiles:     legacyBlock,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to render product_manager/expand prompt for story %s: %w", target.ID, err)
	}

	pmCtx := context.WithValue(ctx, "agent_role", "product_manager") //nolint:staticcheck
	pmCtx = domain.WithUncompactableTail(pmCtx, len(rendered.Contract))

	// Enforce 180s timeout per story expansion
	callCtx, cancel := context.WithTimeout(pmCtx, 180*time.Second)
	defer cancel()

	resp, err := llmClient.Complete(callCtx, rendered.Full())
	if err != nil {
		return nil, fmt.Errorf("LLM completion failed expanding story %s: %w", target.ID, err)
	}

	for _, act := range resp.Actions {
		if act.Tool == "create_story" {
			filename, _ := act.Args["filename"].(string)
			content, _ := act.Args["content"].(string)
			if content != "" {
				if filename == "" {
					slug := target.Slug
					if slug == "" {
						slug = ToSlug(target.Title)
					}
					filename = fmt.Sprintf("roadmap/user-stories/%s-%s.md", strings.ToUpper(target.ID), slug)
				}
				return &RawStoryItem{
					Filename: filename,
					Content:  content,
				}, nil
			}
		}
	}

	// Fallback: if response reasoning contains markdown with noctifab-contract
	if strings.Contains(resp.Reasoning, "noctifab-contract") {
		slug := target.Slug
		if slug == "" {
			slug = ToSlug(target.Title)
		}
		filename := fmt.Sprintf("roadmap/user-stories/%s-%s.md", strings.ToUpper(target.ID), slug)
		return &RawStoryItem{
			Filename: filename,
			Content:  resp.Reasoning,
		}, nil
	}

	// Clean synthetic fallback based on outline data if LLM returned no action
	syntheticContent := buildSyntheticStoryFromOutline(target)
	filename := fmt.Sprintf("roadmap/user-stories/%s-%s.md", strings.ToUpper(target.ID), target.Slug)
	return &RawStoryItem{
		Filename: filename,
		Content:  syntheticContent,
	}, nil
}

func buildSyntheticStoryFromOutline(target StoryOutlineItem) string {
	depsJSON, _ := json.Marshal(target.DependsOn)
	if len(target.DependsOn) == 0 {
		depsJSON = []byte("[]")
	}

	relPath := strings.ToLower(strings.ReplaceAll(target.ID, "-", "_"))
	return fmt.Sprintf(`# %s: %s

## Overview
%s

## Domain Capability
%s

## Dependencies
depends_on: %s
change_type: "new"

## Definition of Done (DoD)
1. Observable Entry Point & Black-Box Protocol Contracts: Real working implementation with zero stubs or placeholders.
2. Standard I/O & Formatting: Real domain responses, exit code 0 for success, non-zero on error.
3. Verification Criteria: 100%% passing unit, integration, and black-box E2E test suites with zero failures.
4. Modular File Paths:
   - Implementation: src/%s/
   - Co-located Tests: tests/%s/

`+"```noctifab-contract\n"+`{
  "story_id": "%s",
  "public_contracts": [{
    "id": "%s.baseline",
    "interface": "CLI or Socket",
    "applicable_path_prefixes": ["src/"],
    "allowed_executables": ["make"],
    "exit_codes": [0],
    "stdout_contains": [],
    "stderr_prefixes": []
  }]
}
`+"```\n", strings.ToUpper(target.ID), target.Title, target.Summary, target.DomainSlice, string(depsJSON), relPath, relPath, strings.ToUpper(target.ID), strings.ToLower(target.ID))
}
