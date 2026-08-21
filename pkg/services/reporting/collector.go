package reporting

import (
	"context"
	"fmt"
	"sync"

	"github.com/diegojromerolopez/noctifab/pkg/domain"
)

const (
	MaxOrdinaryProcessEvents  = 1000
	MaxOrdinaryEventsPerStory = 10000
)

type Collector struct {
	mu sync.Mutex

	clock      domain.Clock
	redactor   *Redactor
	btEngine   *BottleneckEngine
	issueEng   *IssueEngine
	snapshot   ReportSnapshot
	eventSeq   int64
	seenIDs    map[string]bool
	activeSpan map[string]domain.ExecutionEvent

	activeStoryID string
	storyEvents   map[string][]domain.ExecutionEvent
}

func NewCollector(clock domain.Clock) *Collector {
	if clock == nil {
		clock = domain.RealClock{}
	}
	c := &Collector{
		clock:       clock,
		redactor:    NewRedactor(),
		btEngine:    NewBottleneckEngine(),
		issueEng:    NewIssueEngine(),
		seenIDs:     make(map[string]bool),
		activeSpan:  make(map[string]domain.ExecutionEvent),
		storyEvents: make(map[string][]domain.ExecutionEvent),
	}
	c.snapshot.PhaseIntervals = make(map[string][]TimeInterval)
	c.snapshot.RoleActiveMS = make(map[string]int64)
	c.snapshot.OperationSumMS = make(map[string]int64)
	c.snapshot.StoryOutcomes = make(map[string]domain.ExecutionOutcome)
	return c
}

func (c *Collector) Start(ctx context.Context, run domain.RunMetadata) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.snapshot.Run = run
	c.snapshot.Status = domain.ExecutionRunning
	c.snapshot.IsCheckpoint = true
	if run.StartedAt.IsZero() {
		c.snapshot.StartedAt = c.clock.Now()
	} else {
		c.snapshot.StartedAt = run.StartedAt
	}
	c.snapshot.LastProgressAt = c.snapshot.StartedAt
	c.snapshot.LastEventAt = c.snapshot.StartedAt
}

func (c *Collector) BeginStory(ctx context.Context, story domain.StoryMetadata) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.activeStoryID = story.StoryID
	c.snapshot.Stories = append(c.snapshot.Stories, story)
	c.snapshot.StoryOutcomes[story.StoryID] = domain.ExecutionRunning
	c.snapshot.LastProgressAt = c.clock.Now()
}

func (c *Collector) EndStory(ctx context.Context, storyID string, outcome domain.ExecutionOutcome) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.snapshot.StoryOutcomes[storyID] = outcome
	if c.activeStoryID == storyID {
		c.activeStoryID = ""
	}
	c.snapshot.LastProgressAt = c.clock.Now()
}

func (c *Collector) Finish(ctx context.Context, outcome domain.ExecutionOutcome) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.snapshot.Status != domain.ExecutionRunning && c.snapshot.Status != "" {
		return // First terminal outcome wins
	}
	c.snapshot.Status = outcome
	c.snapshot.IsCheckpoint = false
	now := c.clock.Now()
	c.snapshot.FinishedAt = &now
	if !c.snapshot.StartedAt.IsZero() {
		c.snapshot.ExecutionWallMS = now.Sub(c.snapshot.StartedAt).Milliseconds()
	}

	// Analyze bottlenecks & issue proposals
	c.snapshot.Bottlenecks = c.btEngine.Analyze(&c.snapshot)
	c.snapshot.Proposals = c.issueEng.DeriveProposals(c.snapshot.Issues)
}

func (c *Collector) Observe(ctx context.Context, event domain.ExecutionEvent) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if event.ID == "" {
		c.eventSeq++
		event.ID = fmt.Sprintf("event-%06d", c.eventSeq)
	}

	if c.seenIDs[event.ID] {
		c.snapshot.Diagnostics = append(c.snapshot.Diagnostics, fmt.Sprintf("Ignored duplicate event ID %s", event.ID))
		return
	}
	c.seenIDs[event.ID] = true

	if event.At.IsZero() {
		event.At = c.clock.Now()
	}
	c.snapshot.LastEventAt = event.At

	// Redact evidence
	if event.Evidence != "" {
		event.Evidence = c.redactor.BoundedRedact(event.Evidence, 4096)
	}

	// Update aggregated counters
	if event.PromptTokens != nil {
		c.snapshot.MeasuredTokens += *event.PromptTokens
	}
	if event.CompletionTokens != nil {
		c.snapshot.MeasuredTokens += *event.CompletionTokens
	}
	if event.Provider != "" {
		found := false
		for _, p := range c.snapshot.ProvidersUsed {
			if p == event.Provider {
				found = true
				break
			}
		}
		if !found {
			c.snapshot.ProvidersUsed = append(c.snapshot.ProvidersUsed, event.Provider)
		}
	}

	if event.Outcome == domain.OutcomeFailed {
		c.snapshot.ErrorCount++
	}
	if event.Kind == domain.EventRetryRecorded {
		c.snapshot.RetryCount++
		c.snapshot.SelfCorrection.RetryCount++
	}

	// Churn metrics
	if event.FilesChanged != nil {
		c.snapshot.Churn.FilesChanged += *event.FilesChanged
	}
	if event.LinesAdded != nil {
		c.snapshot.Churn.LinesAdded += *event.LinesAdded
	}
	if event.LinesDeleted != nil {
		c.snapshot.Churn.LinesDeleted += *event.LinesDeleted
	}

	// Self-correction agent tracking
	if event.AgentRole == string(domain.AgentRoleUnblocker) {
		c.snapshot.SelfCorrection.UnblockerInvocations++
	}
	if event.AgentRole == "watchdog" || event.Name == "watchdog_repair" {
		c.snapshot.SelfCorrection.WatchdogInvocations++
	}

	// Track spans & durations
	if event.SpanID != "" {
		if event.DurationMillis != nil {
			dur := *event.DurationMillis
			if event.AgentRole != "" {
				c.snapshot.RoleActiveMS[event.AgentRole] += dur
			}
			if event.Category != "" {
				c.snapshot.OperationSumMS[event.Category] += dur
			}
		}
	}

	switch event.Kind {
	case domain.EventPhaseStarted:
		c.activeSpan[event.Name] = event
	case domain.EventPhaseFinished:
		if startEv, ok := c.activeSpan[event.Name]; ok {
			c.snapshot.PhaseIntervals[event.Name] = append(c.snapshot.PhaseIntervals[event.Name], TimeInterval{
				Start: startEv.At,
				End:   event.At,
			})
			delete(c.activeSpan, event.Name)
		}
	case domain.EventTaskAttemptStarted:
		c.snapshot.SelfCorrection.TaskAttempts++
		stID := event.StoryID
		if stID == "" {
			stID = c.activeStoryID
		}
		found := false
		for i := range c.snapshot.TaskSummaries {
			if c.snapshot.TaskSummaries[i].TaskID == event.TaskID {
				c.snapshot.TaskSummaries[i].AttemptCount++
				c.snapshot.TaskSummaries[i].Status = domain.OutcomeUnknown
				if event.Name != "" {
					c.snapshot.TaskSummaries[i].Title = event.Name
				}
				if c.snapshot.TaskSummaries[i].StoryID == "" {
					c.snapshot.TaskSummaries[i].StoryID = stID
				}
				found = true
				break
			}
		}
		if !found && event.TaskID != "" {
			c.snapshot.TaskSummaries = append(c.snapshot.TaskSummaries, TaskExecutionSummary{
				TaskID:       event.TaskID,
				Title:        event.Name,
				StoryID:      stID,
				AttemptCount: 1,
				Status:       domain.OutcomeUnknown,
			})
		}
	case domain.EventTaskAttemptFinished:
		stID := event.StoryID
		if stID == "" {
			stID = c.activeStoryID
		}
		found := false
		for i := range c.snapshot.TaskSummaries {
			if c.snapshot.TaskSummaries[i].TaskID == event.TaskID {
				c.snapshot.TaskSummaries[i].Status = event.Outcome
				c.snapshot.TaskSummaries[i].ElapsedMS = event.DurationMillis
				c.snapshot.TaskSummaries[i].Evidence = event.Evidence
				if event.Name != "" && c.snapshot.TaskSummaries[i].Title == "" {
					c.snapshot.TaskSummaries[i].Title = event.Name
				}
				if c.snapshot.TaskSummaries[i].StoryID == "" {
					c.snapshot.TaskSummaries[i].StoryID = stID
				}
				found = true
				break
			}
		}
		if !found && event.TaskID != "" {
			c.snapshot.TaskSummaries = append(c.snapshot.TaskSummaries, TaskExecutionSummary{
				TaskID:       event.TaskID,
				Title:        event.Name,
				StoryID:      stID,
				AttemptCount: 1,
				Status:       event.Outcome,
				ElapsedMS:    event.DurationMillis,
				Evidence:     event.Evidence,
			})
		}
		if event.Outcome == domain.OutcomeSuccess {
			c.snapshot.SelfCorrection.TaskSuccesses++
		}
	case domain.EventAgentStarted:
		invID := event.AgentInvocationID
		if invID == "" {
			invID = fmt.Sprintf("inv-%s-%s", event.AgentRole, event.TaskID)
		}
		c.activeSpan["agent:"+invID] = event
	}

	// Track agent invocations
	if event.AgentRole != "" || event.AgentInvocationID != "" {
		invID := event.AgentInvocationID
		if invID == "" {
			// Find existing unfinished invocation for role/task
			for _, inv := range c.snapshot.AgentInvocations {
				if !inv.Finished && inv.Role == event.AgentRole && inv.TaskID == event.TaskID {
					invID = inv.ID
					break
				}
			}
			if invID == "" {
				invID = fmt.Sprintf("inv-%s-%s", event.AgentRole, event.TaskID)
			}
		}
		stID := event.StoryID
		if stID == "" {
			stID = c.activeStoryID
		}
		found := false
		for i := range c.snapshot.AgentInvocations {
			if c.snapshot.AgentInvocations[i].ID == invID {
				inv := &c.snapshot.AgentInvocations[i]
				if inv.StoryID == "" {
					inv.StoryID = stID
				}
				if event.DurationMillis != nil {
					inv.ActiveMS += *event.DurationMillis
				}
				if event.Kind == domain.EventLLMCallFinished && event.DurationMillis != nil {
					inv.LLMMS += *event.DurationMillis
				}
				if (event.Kind == domain.EventToolFinished || event.Kind == domain.EventSandboxFinished || event.Kind == domain.EventValidationFinished || event.Kind == domain.EventVCSFinished) && event.DurationMillis != nil {
					inv.ToolsMS += *event.DurationMillis
				}
				if event.Kind == domain.EventWaitFinished && event.DurationMillis != nil {
					if inv.WaitMS == nil {
						zero := int64(0)
						inv.WaitMS = &zero
					}
					*inv.WaitMS += *event.DurationMillis
				}
				if event.Turn > inv.Turns {
					inv.Turns = event.Turn
				}
				if event.Kind == domain.EventAgentFinished {
					inv.Finished = true
					inv.Outcome = event.Outcome
					if startEv, ok := c.activeSpan["agent:"+invID]; ok {
						dur := event.At.Sub(startEv.At).Milliseconds()
						if dur > inv.ActiveMS {
							inv.ActiveMS = dur
						}
						delete(c.activeSpan, "agent:"+invID)
					}
				}
				found = true
				break
			}
		}
		if !found {
			var activeMS, llmMS, toolsMS int64
			if event.DurationMillis != nil {
				activeMS = *event.DurationMillis
			}
			if event.Kind == domain.EventLLMCallFinished && event.DurationMillis != nil {
				llmMS = *event.DurationMillis
			}
			if (event.Kind == domain.EventToolFinished || event.Kind == domain.EventSandboxFinished || event.Kind == domain.EventValidationFinished || event.Kind == domain.EventVCSFinished) && event.DurationMillis != nil {
				toolsMS = *event.DurationMillis
			}
			var waitMS *int64
			if event.Kind == domain.EventWaitFinished && event.DurationMillis != nil {
				w := *event.DurationMillis
				waitMS = &w
			}
			c.snapshot.AgentInvocations = append(c.snapshot.AgentInvocations, AgentInvocationSummary{
				ID:        invID,
				Role:      event.AgentRole,
				StoryID:   stID,
				TaskID:    event.TaskID,
				ActiveMS:  activeMS,
				LLMMS:     llmMS,
				ToolsMS:   toolsMS,
				WaitMS:    waitMS,
				Turns:     event.Turn,
				Outcome:   event.Outcome,
				StartedAt: event.At,
				Finished:  event.Kind == domain.EventAgentFinished,
			})
		}
	}

	// Record deterministic issues for failed events or retries
	if event.Outcome == domain.OutcomeFailed || event.Outcome == domain.OutcomeTimeout || event.Outcome == domain.OutcomeBlocked || event.Kind == domain.EventRetryRecorded {
		title := fmt.Sprintf("Event %s failed (%s)", event.Kind, event.Name)
		if event.Kind == domain.EventTaskAttemptFinished && event.Outcome == domain.OutcomeFailed {
			title = fmt.Sprintf("Task %s attempt failed", event.TaskID)
		} else if event.Kind == domain.EventRetryRecorded {
			title = fmt.Sprintf("Retry recorded for %s", event.Name)
		}
		issueID := c.issueEng.GenerateIssueID("functional", string(event.Outcome), event.StoryID, event.TaskID, "execution", title)
		issueFound := false
		for _, iss := range c.snapshot.Issues {
			if iss.ID == issueID {
				issueFound = true
				break
			}
		}
		if !issueFound {
			c.snapshot.Issues = append(c.snapshot.Issues, domain.ReportIssue{
				ID:       issueID,
				Category: "functional",
				Severity: "high",
				Kind:     "confirmed",
				Title:    title,
				Impact:   "Delayed execution progress",
				Scope:    "noctifab",
				StoryID:  event.StoryID,
				TaskID:   event.TaskID,
				Evidence: []domain.EvidenceRef{{EventID: event.ID, Excerpt: event.Evidence}},
			})
		}
	}

}

func (c *Collector) Snapshot() ReportSnapshot {
	c.mu.Lock()
	defer c.mu.Unlock()

	snap := c.snapshot
	if snap.StartedAt.IsZero() {
		snap.StartedAt = c.clock.Now()
	}
	if snap.FinishedAt == nil {
		snap.ExecutionWallMS = c.clock.Now().Sub(snap.StartedAt).Milliseconds()
	}

	// Deep copy maps
	snap.StoryOutcomes = make(map[string]domain.ExecutionOutcome)
	for k, v := range c.snapshot.StoryOutcomes {
		snap.StoryOutcomes[k] = v
	}

	snap.PhaseIntervals = make(map[string][]TimeInterval)
	for k, v := range c.snapshot.PhaseIntervals {
		invs := make([]TimeInterval, len(v))
		copy(invs, v)
		snap.PhaseIntervals[k] = invs
	}

	if snap.Run.ProjectPath != "" {
		snap.Churn = computeWorkspaceChurn(snap.Run.ProjectPath)
		snap.PublicContracts = collectStoryContracts(snap.Run.ProjectPath)
	}

	return snap
}
