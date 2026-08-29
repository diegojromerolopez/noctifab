package services

import (
	"fmt"
	"strings"

	"github.com/diegojromerolopez/noctifab/pkg/domain"
)

// UpsertStoryContract updates an existing StoryContract or appends a new one to the state.
func UpsertStoryContract(state *domain.State, contract domain.StoryContract) {
	if state == nil || contract.StoryID == "" {
		return
	}
	for i := range state.StoryContracts {
		if state.StoryContracts[i].StoryID == contract.StoryID {
			state.StoryContracts[i] = contract
			return
		}
	}
	state.StoryContracts = append(state.StoryContracts, contract)
}

func upsertStoryContract(state *domain.State, contract domain.StoryContract) {
	UpsertStoryContract(state, contract)
}

// UpsertReviewPhase updates an existing ReviewPhase by ID or composite key, or appends a new one.
func UpsertReviewPhase(phases []domain.ReviewPhase, phase domain.ReviewPhase) []domain.ReviewPhase {
	for i := range phases {
		if (phase.ID != "" && phases[i].ID == phase.ID) ||
			(phase.StoryID != "" &&
				phases[i].StoryID == phase.StoryID &&
				phases[i].TaskID == phase.TaskID &&
				phases[i].Role == phase.Role &&
				phases[i].ArtifactID == phase.ArtifactID &&
				phases[i].Attempt == phase.Attempt) {
			phases[i] = phase
			return phases
		}
	}
	return append(phases, phase)
}

func upsertReviewPhase(phases []domain.ReviewPhase, phase domain.ReviewPhase) []domain.ReviewPhase {
	return UpsertReviewPhase(phases, phase)
}

func containsScenario(scenarios []domain.QAScenario, phaseID, fingerprint string) bool {
	for _, scenario := range scenarios {
		if scenario.ReviewPhaseID == phaseID && scenario.Fingerprint == fingerprint {
			return true
		}
	}
	return false
}

func containsFinding(findings []domain.QAFinding, artifactID, fingerprint string) bool {
	for _, finding := range findings {
		if finding.ArtifactID == artifactID && finding.ScenarioFingerprint == fingerprint {
			return true
		}
	}
	return false
}

func qaFindingsFeedback(findings []domain.QAFinding) string {
	var feedback strings.Builder
	feedback.WriteString("\n\nQA public-contract findings. Fix only the current task, then rebuild and rerun validation:\n")
	for _, finding := range findings {
		_, _ = fmt.Fprintf(&feedback, "contract=%s fingerprint=%s artifact=%s expected=%s actual=%s evidence=%s\n",
			finding.PublicContractID, finding.ScenarioFingerprint, finding.ArtifactID,
			sanitizeQAText(finding.Expected, 4096), sanitizeQAText(finding.Actual, 4096), sanitizeQAText(finding.Evidence, 4096))
	}
	return sanitizeQAText(feedback.String(), 16384)
}

func sanitizeQAResult(result QAReviewResult, limit int) QAReviewResult {
	result.Phase.TerminalReason = sanitizeQAText(result.Phase.TerminalReason, 1024)
	result.TesterPatch = capText(result.TesterPatch, limit)
	for i := range result.Scenarios {
		result.Scenarios[i].Name = sanitizeQAText(result.Scenarios[i].Name, 1024)
		result.Scenarios[i].Evidence = sanitizeQAText(result.Scenarios[i].Evidence, limit)
	}
	for i := range result.Findings {
		finding := &result.Findings[i]
		finding.Expected = sanitizeQAText(finding.Expected, limit)
		finding.Actual = sanitizeQAText(finding.Actual, limit)
		finding.Evidence = sanitizeQAText(finding.Evidence, limit)
	}
	return result
}
