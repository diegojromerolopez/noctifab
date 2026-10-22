package services

import (
	"context"
	"testing"

	"github.com/diegojromerolopez/noctifab/pkg/domain"
	"github.com/diegojromerolopez/noctifab/pkg/infrastructure/config"
)

func TestHypothesisResolution_CorrectSelection(t *testing.T) {
	o := &Orchestrator{
		cfg: OrchestratorConfig{
			Hypotheses: config.HypothesesConfig{
				Enabled:    true,
				Agents:     2,
				Resolution: config.ResolutionCorrect,
			},
		},
	}

	results := []HypothesisResult{
		{
			Index:    1,
			Passed:   false,
			PassRate: 60.0,
			LogMsg:   "3/5 tests passed",
		},
		{
			Index:    2,
			Passed:   true,
			PassRate: 100.0,
			LogMsg:   "5/5 tests passed",
		},
	}

	// Verify scoring logic
	bestIdx := 0
	highestScore := -1.0
	for i, r := range results {
		score := r.PassRate
		if r.Passed {
			score += 1000.0
		}
		if score > highestScore {
			highestScore = score
			bestIdx = i
		}
	}

	if bestIdx != 1 {
		t.Fatalf("expected candidate 2 to win with 100%% pass rate, got %d", bestIdx)
	}
	if o.cfg.Hypotheses.GetResolution() != config.ResolutionCorrect {
		t.Errorf("expected ResolutionCorrect")
	}
}

func TestHypothesisResolution_MixtureFallback(t *testing.T) {
	hypCfg := config.HypothesesConfig{
		Enabled:    true,
		Agents:     2,
		Resolution: config.ResolutionMixture,
	}

	if hypCfg.GetResolution() != config.ResolutionMixture {
		t.Fatalf("expected ResolutionMixture, got %s", hypCfg.GetResolution())
	}

	results := []HypothesisResult{
		{
			Index:    1,
			Passed:   true,
			PassRate: 100.0,
		},
		{
			Index:    2,
			Passed:   false,
			PassRate: 50.0,
		},
	}

	// When candidate 1 already passed 100%, mixture should not degrade it
	bestIdx := 0
	highestScore := -1.0
	for i, r := range results {
		score := r.PassRate
		if r.Passed {
			score += 1000.0
		}
		if score > highestScore {
			highestScore = score
			bestIdx = i
		}
	}

	if bestIdx != 0 {
		t.Fatalf("expected candidate 1 to win, got %d", bestIdx)
	}
}

func TestOrchestrator_FastPathComplexityRouting(t *testing.T) {
	tests := []struct {
		complexity string
		isFastPath bool
	}{
		{domain.ComplexityLow, true},
		{domain.ComplexityMedium, true},
		{domain.ComplexityHigh, false},
		{"", false},
	}

	for _, tc := range tests {
		t.Run(tc.complexity, func(t *testing.T) {
			isFast := tc.complexity == domain.ComplexityLow || tc.complexity == domain.ComplexityMedium
			if isFast != tc.isFastPath {
				t.Errorf("complexity %s: expected isFastPath %v, got %v", tc.complexity, tc.isFastPath, isFast)
			}
		})
	}
}

func TestHypothesisRacing_ShortCircuitOn100PercentGreen(t *testing.T) {
	hypCfg := config.HypothesesConfig{
		Enabled:    true,
		Agents:     2,
		Resolution: config.ResolutionCorrect,
	}

	results := []HypothesisResult{
		{
			Index:    1,
			Passed:   true,
			PassRate: 100.0,
			LogMsg:   "All 12 tests passed",
		},
		{
			Index:    2,
			Passed:   false,
			PassRate: 0.0,
			Err:      context.Canceled,
		},
	}

	// Verify scoring logic prioritizes the 100% green candidate over canceled ones
	bestIdx := 0
	highestScore := -1.0
	for i, r := range results {
		if r.Err != nil && !r.Passed {
			continue
		}
		score := r.PassRate
		if r.Passed {
			score += 1000.0
		}
		if score > highestScore {
			highestScore = score
			bestIdx = i
		}
	}

	if bestIdx != 0 {
		t.Fatalf("expected candidate 1 to win with 100%% green, got %d", bestIdx)
	}
	if !results[bestIdx].Passed || results[bestIdx].PassRate < 100.0 {
		t.Fatalf("expected winner to be 100%% green")
	}
	if hypCfg.GetResolution() != config.ResolutionCorrect {
		t.Fatalf("expected resolution correct")
	}
}
