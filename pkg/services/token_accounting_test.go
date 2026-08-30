package services_test

import (
	"context"
	"testing"

	"github.com/diegojromerolopez/noctifab/pkg/domain"
	"github.com/diegojromerolopez/noctifab/pkg/services"
)

func TestTokenAccountingService(t *testing.T) {
	ctx := context.Background()
	svc := services.NewTokenAccountingService()

	svc.RecordLLMUsage(ctx, "US-001", "task-1", "agent-1", domain.TokenUsage{
		InputTokens:  100,
		OutputTokens: 20,
		TotalTokens:  120,
	})
	svc.RecordLLMUsage(ctx, "US-001", "task-2", "agent-2", domain.TokenUsage{
		InputTokens:  200,
		OutputTokens: 30,
		TotalTokens:  230,
	})
	svc.RecordLLMUsage(ctx, "US-002", "task-3", "agent-3", domain.TokenUsage{
		InputTokens:  300,
		OutputTokens: 50,
		TotalTokens:  350,
	})

	in, out, tot := svc.GetTotalBreakdown()
	if in != 600 || out != 100 || tot != 700 {
		t.Errorf("unexpected total breakdown: in=%d out=%d tot=%d", in, out, tot)
	}

	sb1 := svc.GetStoryBreakdown("US-001")
	if sb1.InputTokens != 300 || sb1.OutputTokens != 50 || sb1.TotalTokens != 350 {
		t.Errorf("unexpected story breakdown US-001: %+v", sb1)
	}

	all := svc.GetAllStoryBreakdowns()
	if len(all) != 2 {
		t.Fatalf("expected 2 story breakdowns, got %d", len(all))
	}
	if all[0].StoryID != "US-001" || all[1].StoryID != "US-002" {
		t.Errorf("unexpected breakdown order: %+v", all)
	}

	st := &domain.State{
		Stories: []domain.Story{
			{ID: "US-001"},
			{ID: "US-002"},
		},
		Tasks: []domain.Task{
			{ID: "task-1"},
		},
		ActiveAgents: []domain.Agent{
			{ID: "agent-1"},
		},
	}

	svc.ApplyToState(st)

	if st.Metadata.TotalInputTokens != 600 || st.Metadata.TotalOutputTokens != 100 || st.Metadata.TotalTokensUsed != 700 {
		t.Errorf("unexpected state metadata tokens: %+v", st.Metadata)
	}
	if st.Stories[0].InputTokens != 300 || st.Stories[0].OutputTokens != 50 || st.Stories[0].TokensUsed != 350 {
		t.Errorf("unexpected story 0 tokens: %+v", st.Stories[0])
	}
	if st.Tasks[0].InputTokens != 100 || st.Tasks[0].OutputTokens != 20 || st.Tasks[0].TokensUsed != 120 {
		t.Errorf("unexpected task 0 tokens: %+v", st.Tasks[0])
	}
	if st.ActiveAgents[0].InputTokens != 100 || st.ActiveAgents[0].OutputTokens != 20 || st.ActiveAgents[0].TokensUsed != 120 {
		t.Errorf("unexpected agent 0 tokens: %+v", st.ActiveAgents[0])
	}
}
