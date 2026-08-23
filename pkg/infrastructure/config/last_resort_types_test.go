package config

import (
	"testing"
	"time"
)

func TestLastResortConfig_Defaults(t *testing.T) {
	cfg := DefaultConfig()

	lr := cfg.Agents.LastResort
	if !lr.Enabled {
		t.Errorf("expected LastResort.Enabled to be true, got %v", lr.Enabled)
	}
	if lr.Temperature != 0.1 {
		t.Errorf("expected LastResort.Temperature to be 0.1, got %v", lr.Temperature)
	}
	if lr.MaxTurns != 2 {
		t.Errorf("expected LastResort.MaxTurns to be 2, got %v", lr.MaxTurns)
	}
	if time.Duration(lr.Timeout) != 180*time.Second {
		t.Errorf("expected LastResort.Timeout to be 180s, got %v", lr.Timeout)
	}
	if !lr.AllowSpecMutation {
		t.Errorf("expected LastResort.AllowSpecMutation to be true, got %v", lr.AllowSpecMutation)
	}
	if !lr.AllowScopeReduction {
		t.Errorf("expected LastResort.AllowScopeReduction to be true, got %v", lr.AllowScopeReduction)
	}
	if !lr.EnforceSpecQuality {
		t.Errorf("expected LastResort.EnforceSpecQuality to be true, got %v", lr.EnforceSpecQuality)
	}

	triggers := cfg.Unblocker.LastResortTriggers
	if !triggers.RetriesExhaustion {
		t.Errorf("expected LastResortTriggers.RetriesExhaustion to be true, got %v", triggers.RetriesExhaustion)
	}
	if !triggers.CyclicLoopDetection {
		t.Errorf("expected LastResortTriggers.CyclicLoopDetection to be true, got %v", triggers.CyclicLoopDetection)
	}
	if !triggers.MissingToolchainFastAbort {
		t.Errorf("expected LastResortTriggers.MissingToolchainFastAbort to be true, got %v", triggers.MissingToolchainFastAbort)
	}
	if triggers.QADeadlockTurns != 2 {
		t.Errorf("expected LastResortTriggers.QADeadlockTurns to be 2, got %v", triggers.QADeadlockTurns)
	}
	if triggers.WatchdogTimeoutTurns != 2 {
		t.Errorf("expected LastResortTriggers.WatchdogTimeoutTurns to be 2, got %v", triggers.WatchdogTimeoutTurns)
	}
	if triggers.StallCountThreshold != 4 {
		t.Errorf("expected LastResortTriggers.StallCountThreshold to be 4, got %v", triggers.StallCountThreshold)
	}
}
