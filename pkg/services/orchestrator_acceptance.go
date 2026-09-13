package services

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/diegojromerolopez/noctifab/pkg/domain"
)

// shouldRemediateAcceptanceAudit checks whether the project is eligible for acceptance-level remediation.
// It limits remediation tasks to a safe maximum (2) to prevent infinite loops.
func (o *Orchestrator) shouldRemediateAcceptanceAudit(state *domain.State) bool {
	if o.acceptanceAuditor == nil || state == nil {
		return false
	}
	remediationCount := 0
	for _, t := range state.Tasks {
		if strings.HasPrefix(t.ID, "spec-remediation-") {
			remediationCount++
		}
	}
	return remediationCount < 2
}

// queueAcceptanceRemediationTask creates and queues a specification-level remediation task
// when the Whole-Project Acceptance Audit Gate detects missing features, broken contracts,
// or unverified commands.
func (o *Orchestrator) queueAcceptanceRemediationTask(ctx context.Context, state *domain.State, auditResult *AcceptanceAuditResult) bool {
	if state == nil || auditResult == nil || len(auditResult.Gaps) == 0 {
		return false
	}

	currentStoryID := ExtractStoryID(state.Metadata.InputPath)
	if currentStoryID == "" {
		currentStoryID = state.Metadata.FeatureName
	}
	if currentStoryID == "" {
		currentStoryID = "US-FINAL"
	}

	remediationCount := 0
	var prevTaskIDs []string
	for _, t := range state.Tasks {
		if strings.HasPrefix(t.ID, "spec-remediation-") {
			remediationCount++
		}
		prevTaskIDs = append(prevTaskIDs, t.ID)
	}

	taskID := fmt.Sprintf("spec-remediation-%d", remediationCount+1)
	title := fmt.Sprintf("Specification & Contract Remediation: Implement Missing Features & Real E2E Tests (%d)", remediationCount+1)

	var sb strings.Builder
	sb.WriteString("Whole-Project Acceptance Audit detected missing specification features, unfulfilled contracts, or unverified commands:\n")
	for _, gap := range auditResult.Gaps {
		fmt.Fprintf(&sb, "- %s\n", gap)
	}
	fmt.Fprintf(&sb, "\nSummary: %s\n\n", auditResult.Summary)
	sb.WriteString("MANDATE:\n")
	sb.WriteString("1. Implement all missing commands, protocols, and modules declared in SPEC.md.\n")
	sb.WriteString("2. Author non-tautological, behavioral black-box E2E tests covering each command's observable inputs and outputs.\n")
	sb.WriteString("3. Verify that all unit and E2E tests pass before completing your turn.\n")

	remediationTask := domain.Task{
		ID:          taskID,
		Title:       title,
		Description: sb.String(),
		StoryID:     currentStoryID,
		Status:      domain.TaskPending,
		DependsOn:   prevTaskIDs,
		CreatedAt:   time.Now().UTC(),
		Retries:     0,
	}

	state.Tasks = append(state.Tasks, remediationTask)
	fmt.Printf("🔍 [Acceptance Gate] Missing specification features or E2E contracts detected (%d gap(s)). Triggering remediation cycle (Task: %s)...\n", len(auditResult.Gaps), taskID)
	return true
}
