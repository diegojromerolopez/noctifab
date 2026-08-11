package reporting

import (
	"sort"
	"time"

	"github.com/diegojromerolopez/noctifab/pkg/domain"
)

type TimeInterval struct {
	Start time.Time
	End   time.Time
}

func MergeIntervals(intervals []TimeInterval) []TimeInterval {
	if len(intervals) == 0 {
		return nil
	}

	sorted := make([]TimeInterval, len(intervals))
	copy(sorted, intervals)
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].Start.Before(sorted[j].Start)
	})

	merged := []TimeInterval{sorted[0]}
	for i := 1; i < len(sorted); i++ {
		last := &merged[len(merged)-1]
		curr := sorted[i]
		if curr.Start.After(last.End) {
			merged = append(merged, curr)
		} else if curr.End.After(last.End) {
			last.End = curr.End
		}
	}
	return merged
}

func TotalIntervalDurationMS(intervals []TimeInterval) int64 {
	merged := MergeIntervals(intervals)
	var total int64
	for _, inv := range merged {
		d := inv.End.Sub(inv.Start).Milliseconds()
		if d > 0 {
			total += d
		}
	}
	return total
}

type AgentInvocationSummary struct {
	ID        string              `json:"id"`
	Role      string              `json:"role"`
	StoryID   string              `json:"story_id"`
	TaskID    string              `json:"task_id"`
	ActiveMS  int64               `json:"active_ms"`
	LLMMS     int64               `json:"llm_ms"`
	ToolsMS   int64               `json:"tools_ms"`
	WaitMS    *int64              `json:"wait_ms,omitempty"`
	Turns     int                 `json:"turns"`
	Outcome   domain.EventOutcome `json:"outcome"`
	StartedAt time.Time           `json:"started_at"`
	Finished  bool                `json:"finished"`
}

type TaskExecutionSummary struct {
	TaskID       string              `json:"task_id"`
	StoryID      string              `json:"story_id"`
	AttemptCount int                 `json:"attempt_count"`
	Status       domain.EventOutcome `json:"status"`
	ElapsedMS    *int64              `json:"elapsed_ms,omitempty"`
	Evidence     string              `json:"evidence,omitempty"`
}

type ReportSnapshot struct {
	Run              domain.RunMetadata                 `json:"run"`
	Status           domain.ExecutionOutcome            `json:"status"`
	IsCheckpoint     bool                               `json:"is_checkpoint"`
	StartedAt        time.Time                          `json:"started_at"`
	FinishedAt       *time.Time                         `json:"finished_at,omitempty"`
	ExecutionWallMS  int64                              `json:"execution_wall_ms"`
	ReportOverheadMS int64                              `json:"report_overhead_ms"`
	Stories          []domain.StoryMetadata             `json:"stories"`
	StoryOutcomes    map[string]domain.ExecutionOutcome `json:"story_outcomes"`
	AgentInvocations []AgentInvocationSummary           `json:"agent_invocations"`
	TaskSummaries    []TaskExecutionSummary             `json:"task_summaries"`
	PhaseIntervals   map[string][]TimeInterval          `json:"phase_intervals"`
	RoleActiveMS     map[string]int64                   `json:"role_active_ms"`
	OperationSumMS   map[string]int64                   `json:"operation_sum_ms"`
	MeasuredTokens   int64                              `json:"measured_tokens"`
	EstimatedTokens  int64                              `json:"estimated_tokens"`
	TotalCostUSD     string                             `json:"total_cost_usd"`
	FailoverCount    int                                `json:"failover_count"`
	ProvidersUsed    []string                           `json:"providers_used"`
	ErrorCount       int                                `json:"error_count"`
	RetryCount       int                                `json:"retry_count"`
	DroppedEvents    int                                `json:"dropped_events"`
	LastProgressAt   time.Time                          `json:"last_progress_at"`
	LastEventAt      time.Time                          `json:"last_event_at"`
	CurrentActivity  string                             `json:"current_activity"`
	Stuck            bool                               `json:"stuck"`
	StuckReason      string                             `json:"stuck_reason,omitempty"`
	Issues           []domain.ReportIssue               `json:"issues"`
	Bottlenecks      []domain.ReportBottleneck          `json:"bottlenecks"`
	Proposals        []domain.ReportProposal            `json:"proposals"`
	Limitations      []string                           `json:"limitations"`
	Diagnostics      []string                           `json:"diagnostics"`
	Report           *domain.ExecutionReport            `json:"report,omitempty"`
}
