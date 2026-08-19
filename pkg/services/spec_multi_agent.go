package services

import (
	"context"
	"fmt"
	"strings"
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

	// Stage 2: Systems Architect / Generator (Architecture, Tech Stack, CLI/API Interfaces)
	archClient := p.getClientForRole("generator")
	draft, err = p.executeStage(ctx, archClient, "generator", "architect_enrich", prompts.SpecPromptData{
		UserPrompt: userPrompt,
		DraftSpec:  currentSpec,
	})
	if err != nil {
		return "", fmt.Errorf("stage 2 (systems_architect) failed: %w", err)
	}
	currentSpec = draft

	// Stage 3: Test Architect / Tester (Verification, Deterministic Clocks, Edge Cases)
	testerClient := p.getClientForRole("tester")
	draft, err = p.executeStage(ctx, testerClient, "tester", "tester_enrich", prompts.SpecPromptData{
		UserPrompt: userPrompt,
		DraftSpec:  currentSpec,
	})
	if err != nil {
		return "", fmt.Errorf("stage 3 (test_architect) failed: %w", err)
	}
	currentSpec = draft

	// Stage 4: QA Specialist (Definition of Done & Public Contracts)
	qaClient := p.getClientForRole("qa")
	draft, err = p.executeStage(ctx, qaClient, "qa", "qa_enrich", prompts.SpecPromptData{
		UserPrompt: userPrompt,
		DraftSpec:  currentSpec,
	})
	if err != nil {
		return "", fmt.Errorf("stage 4 (qa_specialist) failed: %w", err)
	}
	currentSpec = draft

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
