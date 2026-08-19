package services

import (
	"context"
	"fmt"
	"strings"

	"github.com/diegojromerolopez/noctifab/pkg/domain"
	"github.com/diegojromerolopez/noctifab/pkg/infrastructure/config"
	"github.com/diegojromerolopez/noctifab/pkg/infrastructure/llm"
	"github.com/diegojromerolopez/noctifab/pkg/infrastructure/prompts"
)

// SpecConsensusAuditor performs cross-model consistency validation and contradiction resolution.
type SpecConsensusAuditor struct {
	cfg      *config.Config
	router   *llm.ResilientLLMRouter
	renderer PromptRenderer
}

// NewSpecConsensusAuditor creates a new consensus auditor service.
func NewSpecConsensusAuditor(cfg *config.Config, router *llm.ResilientLLMRouter, renderer PromptRenderer) *SpecConsensusAuditor {
	if renderer == nil {
		renderer = prompts.NewDefaultRenderer()
	}
	return &SpecConsensusAuditor{
		cfg:      cfg,
		router:   router,
		renderer: renderer,
	}
}

// AuditAndReconcile performs a multi-model consistency audit pass over the assembled draft specification.
func (a *SpecConsensusAuditor) AuditAndReconcile(ctx context.Context, draftSpec string, humanHistory string) (string, error) {
	if strings.TrimSpace(draftSpec) == "" {
		return draftSpec, nil
	}

	var client domain.LLMClient
	if a.router != nil {
		// Prefer QA or Product Manager for auditing
		candidates := a.router.ResolveCandidatesForRole("qa")
		if len(candidates) == 0 {
			candidates = a.router.ResolveCandidatesForRole("product_manager")
		}
		if len(candidates) > 0 {
			client = candidates[0].Client
		}
	}

	if client == nil {
		// If no client available, return draft unchanged
		return draftSpec, nil
	}

	rendered, err := a.renderer.Render(prompts.AgentSpec, "consensus_audit", prompts.SpecPromptData{
		DraftSpec:    draftSpec,
		HumanHistory: humanHistory,
	})
	if err != nil {
		return draftSpec, fmt.Errorf("failed to render consensus audit prompt: %w", err)
	}

	auditCtx := context.WithValue(ctx, "agent_role", "auditor") //nolint:staticcheck
	auditCtx = domain.WithUncompactableTail(auditCtx, len(rendered.Contract))

	resp, err := client.Complete(auditCtx, rendered.Full())
	if err != nil {
		// Non-fatal: if audit call fails, preserve draftSpec rather than breaking the loop
		return draftSpec, nil
	}

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

	return draftSpec, nil
}
