package domain

import (
	"context"
	"time"
)

type RunMetadata struct {
	RunID           string    `json:"run_id"`
	Command         string    `json:"command"`
	ProjectPath     string    `json:"project_path"`
	ReportPath      string    `json:"report_path"`
	StartedAt       time.Time `json:"started_at"`
	NoctifabVersion string    `json:"noctifab_version"`
}

type StoryMetadata struct {
	StoryID     string    `json:"story_id"`
	Source      string    `json:"source"`
	FeatureName string    `json:"feature_name"`
	Title       string    `json:"title,omitempty"`
	StateID     string    `json:"state_id,omitempty"`
	Sequence    int       `json:"sequence"`
	StartedAt   time.Time `json:"started_at"`
}

type ExecutionOutcome string

const (
	ExecutionRunning     ExecutionOutcome = "RUNNING"
	ExecutionSuccess     ExecutionOutcome = "SUCCESS"
	ExecutionFailed      ExecutionOutcome = "FAILED"
	ExecutionCancelled   ExecutionOutcome = "CANCELLED"
	ExecutionInterrupted ExecutionOutcome = "INTERRUPTED"
)

type StoryTokenBreakdown struct {
	StoryID      string `json:"story_id"`
	InputTokens  int64  `json:"input_tokens"`
	OutputTokens int64  `json:"output_tokens"`
	TotalTokens  int64  `json:"total_tokens"`
}

type EvidenceRef struct {
	EventID string `json:"event_id"`
	Excerpt string `json:"excerpt,omitempty"`
}

type ReportIssue struct {
	ID                string        `json:"id"`
	Category          string        `json:"category"`
	Severity          string        `json:"severity"`
	Kind              string        `json:"kind"` // confirmed | observation | hypothesis
	Title             string        `json:"title"`
	Behavior          string        `json:"behavior"`
	Impact            string        `json:"impact"`
	Evidence          []EvidenceRef `json:"evidence"`
	StoryID           string        `json:"story_id,omitempty"`
	TaskID            string        `json:"task_id,omitempty"`
	AgentInvocationID string        `json:"agent_invocation_id,omitempty"`
	Phase             string        `json:"phase,omitempty"`
	Scope             string        `json:"scope"` // noctifab | configuration | generated_project | environment | unknown
	AffectedComponent string        `json:"affected_component,omitempty"`
	Blocked           bool          `json:"blocked"`
	Confidence        string        `json:"confidence"`
	ProposedAction    string        `json:"proposed_action"`
}

type ReportBottleneck struct {
	ID          string        `json:"id"`
	Rank        int           `json:"rank"`
	RuleID      string        `json:"rule_id"`
	Scope       string        `json:"scope"`
	Measurement string        `json:"measurement"`
	Impact      string        `json:"impact"`
	Evidence    []EvidenceRef `json:"evidence"`
}

type ReportProposal struct {
	ID           string   `json:"id"`
	IssueIDs     []string `json:"issue_ids"`
	Scope        string   `json:"scope"`
	Action       string   `json:"action"`
	Components   []string `json:"components,omitempty"`
	Verification string   `json:"verification"`
}

type AnalysisPriority struct {
	IssueID string `json:"issue_id"`
	Rank    int    `json:"rank"`
	Reason  string `json:"reason"`
}

type AnalysisHypothesis struct {
	ID         string `json:"id"`
	IssueID    string `json:"issue_id"`
	Statement  string `json:"statement"`
	Confidence string `json:"confidence"`
}

type ExecutionReportInput struct {
	RunID               string             `json:"run_id"`
	Outcome             ExecutionOutcome   `json:"outcome"`
	ExecutionWallMS     *int64             `json:"execution_wall_ms,omitempty"`
	DeterministicIssues []ReportIssue      `json:"deterministic_issues"`
	Bottlenecks         []ReportBottleneck `json:"bottlenecks"`
	Limitations         []string           `json:"limitations"`
}

type ExecutionReport struct {
	Summary    string               `json:"summary"`
	Priorities []AnalysisPriority   `json:"priorities"`
	Hypotheses []AnalysisHypothesis `json:"hypotheses"`
	Proposals  []ReportProposal     `json:"proposals"`
}

type ReportWriter interface {
	WriteAtomic(ctx context.Context, path string, content []byte) error
}

type ReportAnalyzer interface {
	Analyze(ctx context.Context, input ExecutionReportInput) (ExecutionReport, error)
}

type ReportAnalyzerFactory func() ReportAnalyzer

type Clock interface {
	Now() time.Time
}

type RealClock struct{}

func (RealClock) Now() time.Time {
	return time.Now().UTC()
}
