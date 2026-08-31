package ensemble_test

import (
	"testing"

	"github.com/diegojromerolopez/noctifab/pkg/infrastructure/llm/ensemble"
)

func TestTelemetryTracker_Operations(t *testing.T) {
	tracker := ensemble.NewTelemetryTracker()

	tracker.RecordInvocation("adaptive")
	tracker.RecordInvocation("parallel")
	tracker.RecordQuorum(2)
	tracker.RecordEarlyExit(5000)
	tracker.RecordConsensus(true)
	tracker.RecordConsensus(false)
	tracker.RecordAdaptivePath("fast")
	tracker.RecordAdaptivePath("heavy")

	snap := tracker.Snapshot()
	if snap.TotalInvocations != 2 {
		t.Errorf("expected 2 total invocations, got %d", snap.TotalInvocations)
	}
	if snap.InvocationsByStrategy["adaptive"] != 1 || snap.InvocationsByStrategy["parallel"] != 1 {
		t.Errorf("strategy breakdown mismatch: %v", snap.InvocationsByStrategy)
	}
	if snap.SpeculativeQuorumWins != 1 || snap.StragglersCancelled != 2 {
		t.Errorf("quorum metrics mismatch: wins=%d, stragglers=%d", snap.SpeculativeQuorumWins, snap.StragglersCancelled)
	}
	if snap.EarlyExitPasses != 1 || snap.EstimatedTokensSaved != 5000 {
		t.Errorf("early exit mismatch: passes=%d, tokens=%d", snap.EarlyExitPasses, snap.EstimatedTokensSaved)
	}
	if snap.ConsensusUnanimous != 1 || snap.ConsensusTieBreakers != 1 {
		t.Errorf("consensus metrics mismatch")
	}
	if snap.AdaptiveFastPaths != 1 || snap.AdaptiveHeavyPaths != 1 {
		t.Errorf("adaptive metrics mismatch")
	}
}
