package ensemble

import (
	"sync"
)

// TelemetryTracker collects real-time metrics across all ensemble executions.
type TelemetryTracker struct {
	mu sync.RWMutex

	TotalInvocations      int64
	InvocationsByStrategy map[string]int64
	SpeculativeQuorumWins int64
	StragglersCancelled   int64
	EarlyExitPasses       int64
	ConsensusUnanimous    int64
	ConsensusTieBreakers  int64
	AdaptiveFastPaths     int64
	AdaptiveStandardPaths int64
	AdaptiveHeavyPaths    int64
	EstimatedTokensSaved  int64
}

var globalTelemetry = NewTelemetryTracker()

// GlobalTelemetry returns the shared singleton telemetry tracker.
func GlobalTelemetry() *TelemetryTracker {
	return globalTelemetry
}

// NewTelemetryTracker creates an initialized tracker instance.
func NewTelemetryTracker() *TelemetryTracker {
	return &TelemetryTracker{
		InvocationsByStrategy: make(map[string]int64),
	}
}

// RecordInvocation increments counts for a given strategy.
func (t *TelemetryTracker) RecordInvocation(strategy string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.TotalInvocations++
	t.InvocationsByStrategy[strategy]++
}

// RecordQuorum records a speculative quorum completion.
func (t *TelemetryTracker) RecordQuorum(stragglers int) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.SpeculativeQuorumWins++
	t.StragglersCancelled += int64(stragglers)
}

// RecordEarlyExit records an AST/anti-stub early exit pass.
func (t *TelemetryTracker) RecordEarlyExit(estimatedTokensSaved int64) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.EarlyExitPasses++
	t.EstimatedTokensSaved += estimatedTokensSaved
}

// RecordConsensus records whether consensus was unanimous or required a tie-breaker.
func (t *TelemetryTracker) RecordConsensus(unanimous bool) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if unanimous {
		t.ConsensusUnanimous++
	} else {
		t.ConsensusTieBreakers++
	}
}

// RecordAdaptivePath records the dynamic routing decision made by the adaptive client.
func (t *TelemetryTracker) RecordAdaptivePath(path string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	switch path {
	case "fast":
		t.AdaptiveFastPaths++
	case "heavy":
		t.AdaptiveHeavyPaths++
	default:
		t.AdaptiveStandardPaths++
	}
}

// TelemetrySnapshot represents an immutable point-in-time copy of ensemble metrics.
type TelemetrySnapshot struct {
	TotalInvocations      int64
	InvocationsByStrategy map[string]int64
	SpeculativeQuorumWins int64
	StragglersCancelled   int64
	EarlyExitPasses       int64
	ConsensusUnanimous    int64
	ConsensusTieBreakers  int64
	AdaptiveFastPaths     int64
	AdaptiveStandardPaths int64
	AdaptiveHeavyPaths    int64
	EstimatedTokensSaved  int64
}

// Snapshot returns a copy of the current metrics.
func (t *TelemetryTracker) Snapshot() TelemetrySnapshot {
	t.mu.RLock()
	defer t.mu.RUnlock()

	strategyCopy := make(map[string]int64, len(t.InvocationsByStrategy))
	for k, v := range t.InvocationsByStrategy {
		strategyCopy[k] = v
	}

	return TelemetrySnapshot{
		TotalInvocations:      t.TotalInvocations,
		InvocationsByStrategy: strategyCopy,
		SpeculativeQuorumWins: t.SpeculativeQuorumWins,
		StragglersCancelled:   t.StragglersCancelled,
		EarlyExitPasses:       t.EarlyExitPasses,
		ConsensusUnanimous:    t.ConsensusUnanimous,
		ConsensusTieBreakers:  t.ConsensusTieBreakers,
		AdaptiveFastPaths:     t.AdaptiveFastPaths,
		AdaptiveStandardPaths: t.AdaptiveStandardPaths,
		AdaptiveHeavyPaths:    t.AdaptiveHeavyPaths,
		EstimatedTokensSaved:  t.EstimatedTokensSaved,
	}
}
