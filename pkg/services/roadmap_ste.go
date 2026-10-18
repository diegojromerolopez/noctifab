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
)

// ResolveSpecPath returns projectPath/SPEC.ste.md if it exists, otherwise projectPath/SPEC.md.
// This ensures runtime components transparently read the ASD-STE100 specification
// once the Product Manager has generated it, while falling back to the human-authored SPEC.md.
func ResolveSpecPath(projectPath string) string {
	stePath := filepath.Join(projectPath, "SPEC.ste.md")
	if _, err := os.Stat(stePath); err == nil {
		return stePath
	}
	return filepath.Join(projectPath, "SPEC.md")
}

// EnsureSTESpecification executes the Product Manager Agent's first task:
// translating the human-authored SPEC.md into ASD-STE100 Simplified Technical English
// and persisting it as SPEC.ste.md.
//
// Rules enforced:
//  1. SPEC.md is the human ground truth and is NEVER overwritten or modified.
//  2. If SPEC.ste.md already exists, it is loaded directly without invoking the LLM.
//  3. If SPEC.ste.md is absent, the Product Manager translates SPEC.md into ASD-STE100
//     and writes SPEC.ste.md in the project directory.
//  4. Downstream operations then consume SPEC.ste.md for roadmap generation, user stories, and tasks.
func EnsureSTESpecification(
	ctx context.Context,
	projectPath string,
	llmClient domain.LLMClient,
	renderer PromptRenderer,
) (string, error) {
	stePath := filepath.Join(projectPath, "SPEC.ste.md")
	if steBytes, err := os.ReadFile(stePath); err == nil && len(strings.TrimSpace(string(steBytes))) > 0 {
		return string(steBytes), nil
	}

	specPath := filepath.Join(projectPath, "SPEC.md")
	specBytes, err := os.ReadFile(specPath)
	if err != nil {
		if os.IsNotExist(err) {
			return "", fmt.Errorf("SPEC.md not found in project path %q: %w", projectPath, err)
		}
		return "", fmt.Errorf("reading SPEC.md: %w", err)
	}
	specContent := string(specBytes)

	fmt.Printf("ℹ [Product Manager] First Task: Translating SPEC.md to ASD-STE100 Simplified Technical English (SPEC.ste.md)...\n")

	if obs := domain.ObserverFromContext(ctx); obs != nil {
		obs.Observe(ctx, domain.ExecutionEvent{
			Kind:      domain.EventPhaseStarted,
			AgentRole: "product_manager",
			Name:      "translate_spec_to_ste",
			At:        time.Now().UTC(),
		})
	}

	if renderer == nil {
		renderer = prompts.NewDefaultRenderer()
	}

	translatedSpec := specContent
	if llmClient != nil {
		rendered, rErr := renderer.Render(prompts.AgentProductManager, "translate_ste", prompts.ProductManagerPromptData{
			Spec: specContent,
		})
		if rErr == nil {
			callCtx, cancel := context.WithTimeout(ctx, 180*time.Second)
			defer cancel()

			pmCtx := context.WithValue(callCtx, "agent_role", "product_manager") //nolint:staticcheck
			pmCtx = domain.WithUncompactableTail(pmCtx, len(rendered.Contract))

			resp, cErr := llmClient.Complete(pmCtx, rendered.Full())
			if cErr == nil && resp != nil {
				extracted := extractSTEContentFromResponse(resp)
				if strings.TrimSpace(extracted) != "" {
					translatedSpec = extracted
				}
			}
		}
	}

	if wErr := os.WriteFile(stePath, []byte(translatedSpec), 0644); wErr != nil {
		return "", fmt.Errorf("failed to write SPEC.ste.md: %w", wErr)
	}

	fmt.Printf("ℹ [Product Manager] Created SPEC.ste.md (%d bytes) in ASD-STE100 Simplified Technical English\n", len(translatedSpec))

	if obs := domain.ObserverFromContext(ctx); obs != nil {
		obs.Observe(ctx, domain.ExecutionEvent{
			Kind:      domain.EventPhaseFinished,
			AgentRole: "product_manager",
			Name:      "translate_spec_to_ste",
			At:        time.Now().UTC(),
			Outcome:   domain.OutcomeSuccess,
		})
	}

	return translatedSpec, nil
}

// extractSTEContentFromResponse resiliently retrieves the translated markdown specification
// from LLM tool actions or reasoning text.
func extractSTEContentFromResponse(resp *domain.LLMResponse) string {
	if resp == nil {
		return ""
	}
	for _, act := range resp.Actions {
		if act.Tool == "translate_ste" || act.Tool == "refine_spec" {
			for _, key := range []string{"content", "spec", "markdown", "text"} {
				if val, ok := act.Args[key].(string); ok && strings.TrimSpace(val) != "" {
					return val
				}
			}
		}
	}

	// Resilient fallback: inspect reasoning if wrapped in markdown
	reasoning := strings.TrimSpace(resp.Reasoning)
	if strings.Contains(reasoning, "# ") {
		return extractFencedOrRawMarkdown(reasoning)
	}

	return ""
}

// extractFencedOrRawMarkdown extracts markdown from code fences if present, or returns raw string.
func extractFencedOrRawMarkdown(raw string) string {
	trimmed := strings.TrimSpace(raw)
	if strings.HasPrefix(trimmed, "```markdown") || strings.HasPrefix(trimmed, "```md") || strings.HasPrefix(trimmed, "```") {
		firstLineEnd := strings.Index(trimmed, "\n")
		lastFence := strings.LastIndex(trimmed, "```")
		if firstLineEnd != -1 && lastFence > firstLineEnd {
			return strings.TrimSpace(trimmed[firstLineEnd+1 : lastFence])
		}
	}
	return trimmed
}
