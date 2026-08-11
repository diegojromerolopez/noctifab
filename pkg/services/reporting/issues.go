package reporting

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"

	"github.com/diegojromerolopez/noctifab/pkg/domain"
)

type IssueEngine struct{}

func NewIssueEngine() *IssueEngine {
	return &IssueEngine{}
}

func (e *IssueEngine) GenerateIssueID(category, kind, storyID, taskID, phase, title string) string {
	rawKey := fmt.Sprintf("%s|%s|%s|%s|%s|%s",
		strings.ToLower(category),
		strings.ToLower(kind),
		strings.ToLower(storyID),
		strings.ToLower(taskID),
		strings.ToLower(phase),
		strings.ToLower(title),
	)
	hash := sha256.Sum256([]byte(rawKey))
	hexStr := strings.ToUpper(hex.EncodeToString(hash[:]))
	return "ISSUE-" + hexStr[:10]
}

func (e *IssueEngine) GenerateProposalID(issueIDs []string, action string) string {
	sortKey := strings.Join(issueIDs, ",") + "|" + strings.ToLower(action)
	hash := sha256.Sum256([]byte(sortKey))
	hexStr := strings.ToUpper(hex.EncodeToString(hash[:]))
	return "PROP-" + hexStr[:10]
}

func (e *IssueEngine) DeriveProposals(issues []domain.ReportIssue) []domain.ReportProposal {
	if len(issues) == 0 {
		return nil
	}

	var proposals []domain.ReportProposal
	for _, issue := range issues {
		var action, verification string
		switch issue.Category {
		case "functional":
			action = "Inspect failed task output, error category, and unit test assertions"
			verification = "Re-run target story task locally to confirm reproducibility"
		case "performance":
			action = "Review agent role latency distribution and token usage concentration"
			verification = "Benchmark task execution time after prompt optimization"
		case "operational":
			action = "Inspect sandbox max/idle timeout, network connectivity, and provider health"
			verification = "Verify provider API ping and execution environment resource limits"
		case "configuration":
			action = "Validate .noctifab/config.yaml setting schema and permissions"
			verification = "Run noctifab start with validated config"
		default:
			action = "Inspect execution log tail and structured event evidence"
			verification = "Re-evaluate issue disposition with updated telemetry"
		}

		if issue.ProposedAction != "" {
			action = issue.ProposedAction
		}

		propID := e.GenerateProposalID([]string{issue.ID}, action)
		proposals = append(proposals, domain.ReportProposal{
			ID:           propID,
			IssueIDs:     []string{issue.ID},
			Scope:        issue.Scope,
			Action:       action,
			Components:   []string{"noctifab"},
			Verification: verification,
		})
	}
	return proposals
}
