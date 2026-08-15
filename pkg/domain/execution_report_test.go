package domain_test

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/diegojromerolopez/noctifab/pkg/domain"
)

func TestExecutionReportJSON(t *testing.T) {
	report := domain.ExecutionReport{
		Summary: "Run completed successfully",
		Priorities: []domain.AnalysisPriority{
			{IssueID: "ISSUE-01", Rank: 1, Reason: "High latency"},
		},
		Hypotheses: []domain.AnalysisHypothesis{
			{ID: "HYP-01", IssueID: "ISSUE-01", Statement: "LLM rate limit encountered", Confidence: "high"},
		},
		Proposals: []domain.ReportProposal{
			{ID: "PROP-01", IssueIDs: []string{"ISSUE-01"}, Scope: "configuration", Action: "Increase timeout", Verification: "Re-run benchmark"},
		},
	}

	bytes, err := json.Marshal(report)
	require.NoError(t, err)

	var unmarshaled domain.ExecutionReport
	err = json.Unmarshal(bytes, &unmarshaled)
	require.NoError(t, err)

	assert.Equal(t, report.Summary, unmarshaled.Summary)
	require.Len(t, unmarshaled.Priorities, 1)
	assert.Equal(t, "ISSUE-01", unmarshaled.Priorities[0].IssueID)
	require.Len(t, unmarshaled.Proposals, 1)
	assert.Equal(t, "PROP-01", unmarshaled.Proposals[0].ID)
}
