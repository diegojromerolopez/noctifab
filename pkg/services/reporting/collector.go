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
	if event.CostUSD != "" {
		c.snapshot.TotalCostUSD = event.CostUSD
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
	}

	// Record deterministic issues for failed tasks
	if event.Kind == domain.EventTaskAttemptFinished && event.Outcome == domain.OutcomeFailed {
		issueID := c.issueEng.GenerateIssueID("functional", string(domain.OutcomeFailed), event.StoryID, event.TaskID, "execution", event.Name)
		c.snapshot.Issues = append(c.snapshot.Issues, domain.ReportIssue{
			ID:       issueID,
			Category: "functional",
			Severity: "high",
			Kind:     "confirmed",
			Title:    fmt.Sprintf("Task %s attempt failed", event.TaskID),
			Impact:   "Delayed story progress",
			Scope:    "noctifab",
			StoryID:  event.StoryID,
			TaskID:   event.TaskID,
			Evidence: []domain.EvidenceRef{{EventID: event.ID, Excerpt: event.Evidence}},
		})
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

	return snap
}
