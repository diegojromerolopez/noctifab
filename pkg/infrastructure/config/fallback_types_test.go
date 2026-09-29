package config

import (
	"os"
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

	sr := cfg.GetSovereignRescue()
	if !sr.IsEnabled() {
		t.Errorf("expected SovereignRescue to be enabled by default")
	}
	if sr.GetMaxTurns() != 10 {
		t.Errorf("expected SovereignRescue.GetMaxTurns to default to 10, got %d", sr.GetMaxTurns())
	}
	if sr.GetTimeout() != 5*time.Minute {
		t.Errorf("expected SovereignRescue.GetTimeout to default to 5m, got %v", sr.GetTimeout())
	}
}

func TestSovereignRescueConfig_CustomAndResolution(t *testing.T) {
	t.Run("custom fallback.sovereign_rescue values are respected", func(t *testing.T) {
		cfg := DefaultConfig()
		cfg.Fallback.SovereignRescue.MaxTurns = 4
		cfg.Fallback.SovereignRescue.Timeout = Duration(10 * time.Minute)

		sr := cfg.GetSovereignRescue()
		if sr.GetMaxTurns() != 4 {
			t.Errorf("expected MaxTurns 4, got %d", sr.GetMaxTurns())
		}
		if sr.GetTimeout() != 10*time.Minute {
			t.Errorf("expected Timeout 10m, got %v", sr.GetTimeout())
		}
	})

	t.Run("agents.fallback.rescue_max_turns is resolved if fallback.sovereign_rescue is unset", func(t *testing.T) {
		cfg := DefaultConfig()
		cfg.Fallback.SovereignRescue.MaxTurns = 0
		cfg.Agents.Fallback.RescueMaxTurns = 5

		sr := cfg.GetSovereignRescue()
		if sr.GetMaxTurns() != 5 {
			t.Errorf("expected MaxTurns 5 from agents.fallback.rescue_max_turns, got %d", sr.GetMaxTurns())
		}
	})

	t.Run("nil config returns safe default MaxTurns of 10", func(t *testing.T) {
		var cfg *Config
		sr := cfg.GetSovereignRescue()
		if sr.GetMaxTurns() != 10 {
			t.Errorf("expected default MaxTurns 10, got %d", sr.GetMaxTurns())
		}
	})

	t.Run("env var NOCTIFAB_RESCUE_MAX_TURNS overrides MaxTurns", func(t *testing.T) {
		cfg := DefaultConfig()
		_ = os.Setenv("NOCTIFAB_RESCUE_MAX_TURNS", "6")
		defer func() { _ = os.Unsetenv("NOCTIFAB_RESCUE_MAX_TURNS") }()

		applyEnvOverrides(cfg)
		sr := cfg.GetSovereignRescue()
		if sr.GetMaxTurns() != 6 {
			t.Errorf("expected MaxTurns 6 from NOCTIFAB_RESCUE_MAX_TURNS env var, got %d", sr.GetMaxTurns())
		}
	})

	t.Run("env var NOCTIFAB_RESCUE_MAX_TURNS works directly on GetSovereignRescue without explicit applyEnvOverrides", func(t *testing.T) {
		_ = os.Setenv("NOCTIFAB_RESCUE_MAX_TURNS", "8")
		defer func() { _ = os.Unsetenv("NOCTIFAB_RESCUE_MAX_TURNS") }()

		cfg := &Config{}
		sr := cfg.GetSovereignRescue()
		if sr.GetMaxTurns() != 8 {
			t.Errorf("expected MaxTurns 8, got %d", sr.GetMaxTurns())
		}

		var nilCfg *Config
		srNil := nilCfg.GetSovereignRescue()
		if srNil.GetMaxTurns() != 8 {
			t.Errorf("expected nilCfg MaxTurns 8, got %d", srNil.GetMaxTurns())
		}
	})

	t.Run("missing toolchain strategy defaults to auto and supports custom and env overrides", func(t *testing.T) {
		cfg := DefaultConfig()
		sr := cfg.GetSovereignRescue()
		if sr.GetMissingToolchainStrategy() != "auto" {
			t.Errorf("expected default strategy 'auto', got %q", sr.GetMissingToolchainStrategy())
		}

		cfg.Fallback.SovereignRescue.MissingToolchainStrategy = "docker"
		sr = cfg.GetSovereignRescue()
		if sr.GetMissingToolchainStrategy() != "docker" {
			t.Errorf("expected strategy 'docker', got %q", sr.GetMissingToolchainStrategy())
		}

		_ = os.Setenv("NOCTIFAB_RESCUE_TOOLCHAIN_STRATEGY", "local")
		defer func() { _ = os.Unsetenv("NOCTIFAB_RESCUE_TOOLCHAIN_STRATEGY") }()

		applyEnvOverrides(cfg)
		sr = cfg.GetSovereignRescue()
		if sr.GetMissingToolchainStrategy() != "local" {
			t.Errorf("expected strategy 'local' from env override, got %q", sr.GetMissingToolchainStrategy())
		}

		var nilCfg *Config
		srNil := nilCfg.GetSovereignRescue()
		if srNil.GetMissingToolchainStrategy() != "local" {
			t.Errorf("expected nilCfg strategy 'local' from env override, got %q", srNil.GetMissingToolchainStrategy())
		}
	})

	t.Run("sovereign rescue providers are resolved and preserved", func(t *testing.T) {
		cfg := DefaultConfig()
		temp := 0.3
		cfg.Fallback.SovereignRescue.Providers = []AgentProviderRef{
			{Name: "gemini", Temperature: &temp},
		}

		sr := cfg.GetSovereignRescue()
		if len(sr.Providers) != 1 {
			t.Fatalf("expected 1 provider, got %d", len(sr.Providers))
		}
		if sr.Providers[0].Name != "gemini" {
			t.Errorf("expected provider name 'gemini', got %q", sr.Providers[0].Name)
		}
		if sr.Providers[0].Temperature == nil || *sr.Providers[0].Temperature != 0.3 {
			t.Errorf("expected provider temperature 0.3, got %v", sr.Providers[0].Temperature)
		}
	})
}
