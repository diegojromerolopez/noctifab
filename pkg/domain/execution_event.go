package domain

import (
	"context"
	"time"
)

type ExecutionEventKind string

const (
	EventRunStarted          ExecutionEventKind = "run_started"
	EventRunFinished         ExecutionEventKind = "run_finished"
	EventStoryStarted        ExecutionEventKind = "story_started"
	EventStoryFinished       ExecutionEventKind = "story_finished"
	EventPhaseStarted        ExecutionEventKind = "phase_started"
	EventPhaseFinished       ExecutionEventKind = "phase_finished"
	EventTaskAttemptStarted  ExecutionEventKind = "task_attempt_started"
	EventTaskAttemptFinished ExecutionEventKind = "task_attempt_finished"
	EventAgentStarted        ExecutionEventKind = "agent_started"
	EventAgentFinished       ExecutionEventKind = "agent_finished"
	EventLLMCallFinished     ExecutionEventKind = "llm_call_finished"
	EventToolFinished        ExecutionEventKind = "tool_finished"
	EventSandboxFinished     ExecutionEventKind = "sandbox_finished"
	EventValidationFinished  ExecutionEventKind = "validation_finished"
	EventVCSFinished         ExecutionEventKind = "vcs_finished"
	EventWaitFinished        ExecutionEventKind = "wait_finished"
	EventRetryRecorded       ExecutionEventKind = "retry_recorded"
	EventFindingRecorded     ExecutionEventKind = "finding_recorded"
	EventClarification       ExecutionEventKind = "clarification"
	EventReporterDiagnostic  ExecutionEventKind = "reporter_diagnostic"
)

type EventOutcome string

const (
	OutcomeUnknown   EventOutcome = "UNKNOWN"
	OutcomeSuccess   EventOutcome = "SUCCESS"
	OutcomeFailed    EventOutcome = "FAILED"
	OutcomeBlocked   EventOutcome = "BLOCKED"
	OutcomeCancelled EventOutcome = "CANCELLED"
	OutcomeTimeout   EventOutcome = "TIMEOUT"
	OutcomeSkipped   EventOutcome = "SKIPPED"
)

type ExecutionEvent struct {
	ID                string             `json:"id"`
	SpanID            string             `json:"span_id,omitempty"`
	ParentSpanID      string             `json:"parent_span_id,omitempty"`
	RunID             string             `json:"run_id"`
	StoryID           string             `json:"story_id,omitempty"`
	TaskID            string             `json:"task_id,omitempty"`
	AgentInvocationID string             `json:"agent_invocation_id,omitempty"`
	AgentRole         string             `json:"agent_role,omitempty"`
	Kind              ExecutionEventKind `json:"kind"`
	Category          string             `json:"category,omitempty"`
	Name              string             `json:"name,omitempty"`
	At                time.Time          `json:"at"`
	DurationMillis    *int64             `json:"duration_ms,omitempty"`
	Outcome           EventOutcome       `json:"outcome,omitempty"`
	Attempt           int                `json:"attempt,omitempty"`
	Turn              int                `json:"turn,omitempty"`
	Provider          string             `json:"provider,omitempty"`
	Model             string             `json:"model,omitempty"`
	PromptTokens      *int64             `json:"prompt_tokens,omitempty"`
	CompletionTokens  *int64             `json:"completion_tokens,omitempty"`
	CachedTokens      *int64             `json:"cached_tokens,omitempty"`
	CostUSD           string             `json:"cost_usd,omitempty"`
	UsageKind         string             `json:"usage_kind,omitempty"` // exact | estimated | unknown
	Count             *int64             `json:"count,omitempty"`
	Total             *int64             `json:"total,omitempty"`
	ExitCode          *int               `json:"exit_code,omitempty"`
	LinesAdded        *int64             `json:"lines_added,omitempty"`
	LinesDeleted      *int64             `json:"lines_deleted,omitempty"`
	FilesChanged      *int64             `json:"files_changed,omitempty"`
	ErrorCategory     string             `json:"error_category,omitempty"`
	Evidence          string             `json:"evidence,omitempty"`
	Blocked           bool               `json:"blocked,omitempty"`
	Metadata          map[string]string  `json:"metadata,omitempty"`
}

type ExecutionObserver interface {
	Observe(ctx context.Context, event ExecutionEvent)
}

type ExecutionReporter interface {
	ExecutionObserver
	Start(ctx context.Context, run RunMetadata)
	BeginStory(ctx context.Context, story StoryMetadata)
	EndStory(ctx context.Context, storyID string, outcome ExecutionOutcome)
	Finish(ctx context.Context, outcome ExecutionOutcome)
}

type observerContextKey struct{}

// WithObserver attaches an ExecutionObserver to context.
func WithObserver(ctx context.Context, obs ExecutionObserver) context.Context {
	if obs == nil {
		return ctx
	}
	return context.WithValue(ctx, observerContextKey{}, obs)
}

// ObserverFromContext extracts an ExecutionObserver from context, returning nil if absent.
func ObserverFromContext(ctx context.Context) ExecutionObserver {
	if ctx == nil {
		return nil
	}
	if obs, ok := ctx.Value(observerContextKey{}).(ExecutionObserver); ok {
		return obs
	}
	return nil
}
