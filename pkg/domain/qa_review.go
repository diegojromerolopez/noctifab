package domain

import "time"

// ReviewStatus identifies the lifecycle state of a specialist review.
type ReviewStatus string

const (
	ReviewWorking         ReviewStatus = "WORKING"
	ReviewPass            ReviewStatus = "PASS"
	ReviewFindings        ReviewStatus = "FINDINGS"
	ReviewSkipped         ReviewStatus = "SKIPPED"
	ReviewInconclusive    ReviewStatus = "INCONCLUSIVE"
	ReviewBudgetExhausted ReviewStatus = "BUDGET_EXHAUSTED"
	ReviewError           ReviewStatus = "ERROR"
	ReviewInterrupted     ReviewStatus = "INTERRUPTED"
)

// ReviewPhase records one bounded review attempt against an immutable artifact.
type ReviewPhase struct {
	ID               string                  `json:"id"`
	StoryID          string                  `json:"story_id"`
	TaskID           string                  `json:"task_id"`
	Role             string                  `json:"role"`
	ArtifactID       string                  `json:"artifact_id"`
	ArtifactManifest []ArtifactManifestEntry `json:"artifact_manifest"`
	Attempt          int                     `json:"attempt"`
	Status           ReviewStatus            `json:"status"`
	TerminalReason   string                  `json:"terminal_reason,omitempty"`
	StartedAt        time.Time               `json:"started_at"`
	DeadlineAt       time.Time               `json:"deadline_at"`
	CompletedAt      time.Time               `json:"completed_at,omitempty"`
	InputTokens      int64                   `json:"input_tokens,omitempty"`
	OutputTokens     int64                   `json:"output_tokens,omitempty"`
	TokensUsed       int64                   `json:"tokens_used"`
}

// ArtifactManifestEntry identifies one persisted artifact file by path and content.
type ArtifactManifestEntry struct {
	Path   string `json:"path"`
	SHA256 string `json:"sha256"`
}

// QAStep describes one process invocation and its observable expectations.
type QAStep struct {
	Command          []string `json:"command"`
	Stdin            string   `json:"stdin,omitempty"`
	ExpectedExitCode int      `json:"expected_exit_code"`
	StdoutContains   []string `json:"stdout_contains,omitempty"`
	StderrPrefix     string   `json:"stderr_prefix,omitempty"`
}

// QAScenario records a deterministic black-box scenario and its evidence.
type QAScenario struct {
	ID               string       `json:"id"`
	ReviewPhaseID    string       `json:"review_phase_id"`
	PublicContractID string       `json:"public_contract_id"`
	Name             string       `json:"name"`
	Fingerprint      string       `json:"fingerprint"`
	Steps            []QAStep     `json:"steps"`
	Status           ReviewStatus `json:"status"`
	Evidence         string       `json:"evidence,omitempty"`
}

// QAFinding records one reproducible public-contract failure.
type QAFinding struct {
	ID                  string `json:"id"`
	ReviewPhaseID       string `json:"review_phase_id"`
	TaskID              string `json:"task_id"`
	ArtifactID          string `json:"artifact_id"`
	ScenarioFingerprint string `json:"scenario_fingerprint"`
	PublicContractID    string `json:"public_contract_id"`
	Severity            string `json:"severity"`
	Expected            string `json:"expected"`
	Actual              string `json:"actual"`
	Evidence            string `json:"evidence"`
	Disposition         string `json:"disposition"`
}
