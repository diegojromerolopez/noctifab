package domain_test

import (
	"encoding/json"
	"testing"

	"github.com/diegojromerolopez/noctifab/pkg/domain"
)

func TestTokenUsageJSONSerialization(t *testing.T) {
	usage := domain.TokenUsage{
		InputTokens:     1000,
		OutputTokens:    200,
		ReasoningTokens: 50,
		CachedTokens:    300,
		TotalTokens:     1200,
	}

	data, err := json.Marshal(usage)
	if err != nil {
		t.Fatalf("unexpected error marshaling TokenUsage: %v", err)
	}

	var unmarshaled domain.TokenUsage
	if err := json.Unmarshal(data, &unmarshaled); err != nil {
		t.Fatalf("unexpected error unmarshaling TokenUsage: %v", err)
	}

	if unmarshaled.InputTokens != 1000 || unmarshaled.OutputTokens != 200 ||
		unmarshaled.ReasoningTokens != 50 || unmarshaled.CachedTokens != 300 ||
		unmarshaled.TotalTokens != 1200 {
		t.Errorf("unmarshaled TokenUsage mismatch: %+v", unmarshaled)
	}
}

func TestDomainStructsTokenFields(t *testing.T) {
	task := domain.Task{
		ID:           "US-001-T1",
		InputTokens:  500,
		OutputTokens: 100,
		TokensUsed:   600,
	}
	if task.InputTokens != 500 || task.OutputTokens != 100 || task.TokensUsed != 600 {
		t.Errorf("task token fields mismatch: %+v", task)
	}

	agent := domain.Agent{
		ID:           "agt-1",
		InputTokens:  1500,
		OutputTokens: 250,
		TokensUsed:   1750,
	}
	if agent.InputTokens != 1500 || agent.OutputTokens != 250 || agent.TokensUsed != 1750 {
		t.Errorf("agent token fields mismatch: %+v", agent)
	}

	story := domain.Story{
		ID:           "US-001",
		InputTokens:  2000,
		OutputTokens: 300,
		TokensUsed:   2300,
	}
	if story.InputTokens != 2000 || story.OutputTokens != 300 || story.TokensUsed != 2300 {
		t.Errorf("story token fields mismatch: %+v", story)
	}

	meta := domain.StateMetadata{
		TotalInputTokens:  10000,
		TotalOutputTokens: 1500,
		TotalTokensUsed:   11500,
	}
	if meta.TotalInputTokens != 10000 || meta.TotalOutputTokens != 1500 || meta.TotalTokensUsed != 11500 {
		t.Errorf("metadata token fields mismatch: %+v", meta)
	}

	specRev := domain.SpecRevision{
		Version:      1,
		InputTokens:  800,
		OutputTokens: 120,
		TokensUsed:   920,
	}
	if specRev.InputTokens != 800 || specRev.OutputTokens != 120 || specRev.TokensUsed != 920 {
		t.Errorf("specRevision token fields mismatch: %+v", specRev)
	}

	review := domain.ReviewPhase{
		ID:           "rev-1",
		InputTokens:  400,
		OutputTokens: 60,
		TokensUsed:   460,
	}
	if review.InputTokens != 400 || review.OutputTokens != 60 || review.TokensUsed != 460 {
		t.Errorf("reviewPhase token fields mismatch: %+v", review)
	}

	breakdown := domain.StoryTokenBreakdown{
		StoryID:      "US-001",
		InputTokens:  2000,
		OutputTokens: 300,
		TotalTokens:  2300,
	}
	if breakdown.StoryID != "US-001" || breakdown.TotalTokens != 2300 {
		t.Errorf("storyTokenBreakdown mismatch: %+v", breakdown)
	}
}
