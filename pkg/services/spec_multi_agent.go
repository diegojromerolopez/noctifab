package services

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/diegojromerolopez/noctifab/pkg/domain"
	"github.com/diegojromerolopez/noctifab/pkg/infrastructure/config"
	"github.com/diegojromerolopez/noctifab/pkg/infrastructure/llm"
	"github.com/diegojromerolopez/noctifab/pkg/infrastructure/prompts"
)

// SpecMultiAgentPipeline coordinates multi-role specification generation and refinement.
type SpecMultiAgentPipeline struct {
	cfg      *config.Config
	router   *llm.ResilientLLMRouter
	renderer PromptRenderer
}

// NewSpecMultiAgentPipeline creates a new multi-agent spec drafting pipeline.
func NewSpecMultiAgentPipeline(cfg *config.Config, router *llm.ResilientLLMRouter, renderer PromptRenderer) *SpecMultiAgentPipeline {
	if renderer == nil {
		renderer = prompts.NewDefaultRenderer()
	}
	return &SpecMultiAgentPipeline{
		cfg:      cfg,
		router:   router,
		renderer: renderer,
	}
}

func (p *SpecMultiAgentPipeline) getClientForRole(roleName string) domain.LLMClient {
	if p.router != nil {
		candidates := p.router.ResolveCandidatesForRole(roleName)
		if len(candidates) > 0 && candidates[0].Client != nil {
			return candidates[0].Client
		}
	}
	return nil
}

// ExecutePass runs the 4-stage sequential spec drafting pipeline.
func (p *SpecMultiAgentPipeline) ExecutePass(ctx context.Context, userPrompt string, existingSpec string) (string, error) {
	currentSpec := existingSpec

	// Stage 1: Product Manager (Overview & Domain Models)
	pmClient := p.getClientForRole("product_manager")
	draft, err := p.executeStage(ctx, pmClient, "product_manager", "pm_draft", prompts.SpecPromptData{
		UserPrompt:   userPrompt,
		ExistingSpec: currentSpec,
		DraftSpec:    currentSpec,
	})
	if err != nil {
		return "", fmt.Errorf("stage 1 (product_manager) failed: %w", err)
	}
	currentSpec = draft

	// Stages 2, 3, 4: Concurrent Multi-Agent Enrichment Burst
	// Systems Architect (Arch & Interfaces), Test Architect (Verification & Testing),
	// and QA Specialist (Definition of Done & Public Contracts) run concurrently.
	var (
		archDraft, testerDraft, qaDraft string
		archErr, testerErr, qaErr       error
		wg                              sync.WaitGroup
	)

	wg.Add(3)
	go func() {
		defer wg.Done()
		archClient := p.getClientForRole("generator")
		archDraft, archErr = p.executeStage(ctx, archClient, "generator", "architect_enrich", prompts.SpecPromptData{
			UserPrompt: userPrompt,
			DraftSpec:  currentSpec,
		})
	}()

	go func() {
		defer wg.Done()
		testerClient := p.getClientForRole("tester")
		testerDraft, testerErr = p.executeStage(ctx, testerClient, "tester", "tester_enrich", prompts.SpecPromptData{
			UserPrompt: userPrompt,
			DraftSpec:  currentSpec,
		})
	}()

	go func() {
		defer wg.Done()
		qaClient := p.getClientForRole("qa")
		qaDraft, qaErr = p.executeStage(ctx, qaClient, "qa", "qa_enrich", prompts.SpecPromptData{
			UserPrompt: userPrompt,
			DraftSpec:  currentSpec,
		})
	}()

	wg.Wait()

	if archErr != nil && testerErr != nil && qaErr != nil {
		return currentSpec, fmt.Errorf("all parallel enrichment stages failed: arch=%v, tester=%v, qa=%v", archErr, testerErr, qaErr)
	}

	currentSpec = mergeSpecEnrichments(currentSpec, archDraft, testerDraft, qaDraft)
	return currentSpec, nil
}

// ExecuteRefinePass refines an existing specification with human feedback and revision history.
func (p *SpecMultiAgentPipeline) ExecuteRefinePass(ctx context.Context, currentSpec string, feedback string, revisions []domain.SpecRevision) (string, error) {
	var historyBuilder strings.Builder
	for _, rev := range revisions {
		if rev.Prompt != "" {
			fmt.Fprintf(&historyBuilder, "- Turn %d: %s\n", rev.Version, rev.Prompt)
		}
	}

	leadRole := "product_manager"
	if p.cfg != nil && p.cfg.Spec.LeadRole != "" {
		leadRole = p.cfg.Spec.LeadRole
	}

	client := p.getClientForRole(leadRole)
	refined, err := p.executeStage(ctx, client, leadRole, "refine", prompts.SpecPromptData{
		DraftSpec:    currentSpec,
		Feedback:     feedback,
		HumanHistory: historyBuilder.String(),
	})
	if err != nil {
		return "", fmt.Errorf("refine stage failed: %w", err)
	}
	return refined, nil
}

func (p *SpecMultiAgentPipeline) executeStage(ctx context.Context, client domain.LLMClient, roleName, actionName string, data prompts.SpecPromptData) (string, error) {
	if client == nil {
		return "", fmt.Errorf("no LLM client resolved for role %q", roleName)
	}

	rendered, err := p.renderer.Render(prompts.AgentSpec, actionName, data)
	if err != nil {
		return "", fmt.Errorf("failed to render prompt %s/%s: %w", prompts.AgentSpec, actionName, err)
	}

	stageCtx := context.WithValue(ctx, "agent_role", roleName) //nolint:staticcheck
	stageCtx = domain.WithUncompactableTail(stageCtx, len(rendered.Contract))

	var lastErr error
	for attempt := 0; attempt < 3; attempt++ {
		resp, err := client.Complete(stageCtx, rendered.Full())
		if err != nil {
			lastErr = err
			time.Sleep(100 * time.Millisecond)
			continue
		}

		// Extract specification text from tool actions or reasoning
		for _, act := range resp.Actions {
			if act.Tool == "update_spec" {
				if content, ok := act.Args["content"].(string); ok && strings.TrimSpace(content) != "" {
					return strings.TrimSpace(content), nil
				}
			}
		}

		if strings.Contains(resp.Reasoning, "# ") {
			return strings.TrimSpace(resp.Reasoning), nil
		}

		lastErr = fmt.Errorf("model response did not contain update_spec tool action with content")
	}

	return "", fmt.Errorf("stage execution failed after retries: %w", lastErr)
}

func mergeSpecEnrichments(baseSpec, archSpec, testerSpec, qaSpec string) string {
	res := baseSpec
	if archSpec != "" {
		res = mergeSections(res, archSpec, []string{"2.", "4.", "Architecture", "Interfaces", "Command Contracts"})
	}
	if testerSpec != "" {
		res = mergeSections(res, testerSpec, []string{"5.", "Verification", "Test Architecture"})
	}
	if qaSpec != "" {
		res = mergeSections(res, qaSpec, []string{"6.", "Definition of Done", "Public Contracts"})
	}
	if res == baseSpec {
		for _, draft := range []string{qaSpec, testerSpec, archSpec} {
			if len(draft) > len(res) {
				return draft
			}
		}
	}
	return res
}

func mergeSections(targetDoc, sourceDoc string, sectionKeywords []string) string {
	for _, kw := range sectionKeywords {
		secContent, secHeading := extractSection(sourceDoc, kw)
		if secContent != "" {
			targetDoc = replaceOrAppendSection(targetDoc, secHeading, secContent)
		}
	}
	return targetDoc
}

func extractSection(doc, keyword string) (string, string) {
	lines := strings.Split(doc, "\n")
	startIdx := -1
	headingLine := ""

	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "## ") && strings.Contains(strings.ToLower(trimmed), strings.ToLower(keyword)) {
			startIdx = i
			headingLine = trimmed
			break
		}
	}

	if startIdx == -1 {
		return "", ""
	}

	endIdx := len(lines)
	for i := startIdx + 1; i < len(lines); i++ {
		trimmed := strings.TrimSpace(lines[i])
		if strings.HasPrefix(trimmed, "## ") || (strings.HasPrefix(trimmed, "# ") && !strings.HasPrefix(trimmed, "### ")) {
			endIdx = i
			break
		}
	}

	return strings.TrimSpace(strings.Join(lines[startIdx:endIdx], "\n")), headingLine
}

func replaceOrAppendSection(doc, heading, content string) string {
	if strings.TrimSpace(content) == "" {
		return doc
	}
	lines := strings.Split(doc, "\n")
	startIdx := -1
	kw := heading
	if strings.HasPrefix(kw, "## ") {
		kw = strings.TrimPrefix(kw, "## ")
	}
	parts := strings.Fields(kw)
	matchKw := kw
	if len(parts) > 0 {
		matchKw = parts[0]
	}

	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "## ") && strings.Contains(strings.ToLower(trimmed), strings.ToLower(matchKw)) {
			startIdx = i
			break
		}
	}

	if startIdx == -1 {
		return strings.TrimSpace(doc) + "\n\n" + content
	}

	endIdx := len(lines)
	for i := startIdx + 1; i < len(lines); i++ {
		trimmed := strings.TrimSpace(lines[i])
		if strings.HasPrefix(trimmed, "## ") || (strings.HasPrefix(trimmed, "# ") && !strings.HasPrefix(trimmed, "### ")) {
			endIdx = i
			break
		}
	}

	var newLines []string
	newLines = append(newLines, lines[:startIdx]...)
	newLines = append(newLines, content)
	newLines = append(newLines, lines[endIdx:]...)
	return strings.TrimSpace(strings.Join(newLines, "\n"))
}
