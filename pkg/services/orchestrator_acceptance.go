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
	if len(auditResult.Fixes) > 0 {
		sb.WriteString("\nAUDITOR PROPOSED FIXES & REMEDIATION BLUEPRINT:\n")
		for i, fix := range auditResult.Fixes {
			fmt.Fprintf(&sb, "%d. [%s] %s: %s\n", i+1, fix.Action, fix.File, fix.Description)
		}
	}
	fmt.Fprintf(&sb, "\nSummary: %s\n\n", auditResult.Summary)
	sb.WriteString("MANDATE:\n")
	sb.WriteString("1. Apply all proposed fixes listed above, implementing missing commands, exports, and dispatcher bindings declared in SPEC.md.\n")
	sb.WriteString("2. Author non-tautological, behavioral black-box E2E tests covering each command's observable inputs and outputs.\n")
	sb.WriteString("3. Verify that all unit and E2E tests pass before completing your turn.\n")

	var targetFiles []string
	for _, fix := range auditResult.Fixes {
		if fix.File != "" {
			targetFiles = append(targetFiles, fix.File)
		}
	}

	isE2E := strings.Contains(auditResult.Summary, "E2E") || strings.Contains(strings.Join(auditResult.Gaps, " "), "E2E")
	if isE2E && state.ProjectPath != "" {
		containerFiles, containerContext := collectE2EContainerFiles(state.ProjectPath)
		targetFiles = append(targetFiles, containerFiles...)
		if containerContext != "" {
			sb.WriteString("\n### 📦 CONTAINER & E2E HARNESS CONFIGURATION FILES:\n")
			sb.WriteString(containerContext)
		}
		sb.WriteString("\n### 🐳 MANDATORY E2E VERIFICATION INSTRUCTION:\n")
		sb.WriteString("You have access to the 'run_e2e_tests' tool. You MUST run 'run_e2e_tests' to verify that the containerized E2E test suite passes before declaring your task complete.\n")
	}

	remediationTask := domain.Task{
		ID:          taskID,
		Title:       title,
		Description: sb.String(),
		StoryID:     currentStoryID,
		Status:      domain.TaskPending,
		DependsOn:   prevTaskIDs,
		TargetFiles: targetFiles,
		CreatedAt:   time.Now().UTC(),
		Retries:     0,
	}

	state.Tasks = append(state.Tasks, remediationTask)
	fmt.Printf("🔍 [Acceptance Gate] Missing specification features or E2E contracts detected (%d gap(s)). Triggering remediation cycle (Task: %s)...\n", len(auditResult.Gaps), taskID)
	return true
}

// SetAcceptanceAuditor overrides the acceptance auditor service (useful for tests).
func (o *Orchestrator) SetAcceptanceAuditor(auditor *AcceptanceAuditor) {
	o.acceptanceAuditor = auditor
}
