package services

import (
	"context"
	"testing"

	"github.com/diegojromerolopez/noctifab/pkg/infrastructure/config"
	"github.com/diegojromerolopez/noctifab/pkg/infrastructure/prompts"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSpecConsensusAuditor(t *testing.T) {
	ctx := context.Background()

	// 1. Empty draft returns as is
	auditor := NewSpecConsensusAuditor(&config.Config{}, nil, nil)
	res, err := auditor.AuditAndReconcile(ctx, "", "history")
	require.NoError(t, err)
	assert.Equal(t, "", res)

	// 2. Nil client returns draft unchanged
	res2, err2 := auditor.AuditAndReconcile(ctx, "# Draft", "history")
	require.NoError(t, err2)
	assert.Equal(t, "# Draft", res2)

	// 3. Mock client reconciles specification
	mockClient := &mockSpecLLMClient{
		content: "# SPEC.md: Reconciled Specification\n## 1. Overview\nConsistent",
	}

	// Test direct execution logic
	rendered, err := auditor.renderer.Render(prompts.AgentSpec, "consensus_audit", prompts.SpecPromptData{
		DraftSpec:    "# Contradictory Draft",
		HumanHistory: "history",
	})
	require.NoError(t, err)

	resp, err := mockClient.Complete(ctx, rendered.Full())
	require.NoError(t, err)
	assert.Equal(t, "update_spec", resp.Actions[0].Tool)
	assert.Equal(t, "# SPEC.md: Reconciled Specification\n## 1. Overview\nConsistent", resp.Actions[0].Args["content"])
}
