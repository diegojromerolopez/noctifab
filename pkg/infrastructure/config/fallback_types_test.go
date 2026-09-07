package config

import (
	"testing"
	"time"
)

func TestFallbackConfig_Defaults(t *testing.T) {
	cfg := DefaultConfig()

	fb := cfg.Agents.Fallback
	if !fb.Enabled {
		t.Errorf("expected Fallback.Enabled to be true, got %v", fb.Enabled)
	}
	if fb.Temperature != 0.1 {
		t.Errorf("expected Fallback.Temperature to be 0.1, got %v", fb.Temperature)
	}
	if fb.MaxTurns != 2 {
		t.Errorf("expected Fallback.MaxTurns to be 2, got %v", fb.MaxTurns)
	}
	if time.Duration(fb.Timeout) != 180*time.Second {
		t.Errorf("expected Fallback.Timeout to be 180s, got %v", fb.Timeout)
	}
	if !fb.AllowSpecMutation {
		t.Errorf("expected Fallback.AllowSpecMutation to be true, got %v", fb.AllowSpecMutation)
	}
	if !fb.AllowScopeReduction {
		t.Errorf("expected Fallback.AllowScopeReduction to be true, got %v", fb.AllowScopeReduction)
	}
	if !fb.EnforceSpecQuality {
		t.Errorf("expected Fallback.EnforceSpecQuality to be true, got %v", fb.EnforceSpecQuality)
	}

	lr := cfg.Agents.LastResort
	if !lr.Enabled {
		t.Errorf("expected LastResort.Enabled to be true, got %v", lr.Enabled)
	}

	triggers := cfg.Fallback.Triggers
	if !triggers.RetriesExhaustion {
		t.Errorf("expected Fallback.Triggers.RetriesExhaustion to be true, got %v", triggers.RetriesExhaustion)
	}
	if !triggers.CyclicLoopDetection {
		t.Errorf("expected Fallback.Triggers.CyclicLoopDetection to be true, got %v", triggers.CyclicLoopDetection)
	}
	if !triggers.MissingToolchainFastAbort {
		t.Errorf("expected Fallback.Triggers.MissingToolchainFastAbort to be true, got %v", triggers.MissingToolchainFastAbort)
	}
	if triggers.QADeadlockTurns != 2 {
		t.Errorf("expected Fallback.Triggers.QADeadlockTurns to be 2, got %v", triggers.QADeadlockTurns)
	}
	if triggers.WatchdogTimeoutTurns != 2 {
		t.Errorf("expected Fallback.Triggers.WatchdogTimeoutTurns to be 2, got %v", triggers.WatchdogTimeoutTurns)
	}
	if triggers.StallCountThreshold != 2 {
		t.Errorf("expected Fallback.Triggers.StallCountThreshold to be 2, got %v", triggers.StallCountThreshold)
	}
}
