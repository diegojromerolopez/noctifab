package services

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

func TestMetricsCollector_Disabled(t *testing.T) {
	mc := NewMetricsCollector(false)
	if mc.IsEnabled() {
		t.Errorf("expected metrics collector to be disabled")
	}

	mc.RecordPhaseStart("Reader")
	mc.RecordPhaseEnd("Reader")
	mc.RecordLLMCall(100*time.Millisecond, 500)
	mc.RecordSandboxRun(200 * time.Millisecond)
	mc.RecordCommit()
	mc.RecordRetry()

	summary := mc.ExportSummary()
	if summary.Enabled {
		t.Errorf("expected exported summary to report disabled")
	}
	if summary.LLMCalls != 0 || summary.SandboxRuns != 0 || summary.TimeToFileFirstCommit != nil {
		t.Errorf("expected 0 metrics recorded when disabled, got %+v", summary)
	}

	tmpDir, err := os.MkdirTemp("", "metrics-disabled-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer func() { _ = os.RemoveAll(tmpDir) }()

	jsonPath := filepath.Join(tmpDir, "metrics.json")
	if err := mc.ExportJSON(jsonPath); err != nil {
		t.Fatalf("unexpected error exporting disabled metrics json: %v", err)
	}
	if _, err := os.Stat(jsonPath); !os.IsNotExist(err) {
		t.Errorf("expected metrics.json NOT to be created when disabled")
	}
}

func TestMetricsCollector_NilSafety(t *testing.T) {
	var mc *MetricsCollector
	if mc.IsEnabled() {
		t.Errorf("expected nil metrics collector IsEnabled to be false")
	}

	// Operations on nil should not panic
	mc.RecordPhaseStart("Reader")
	mc.RecordPhaseEnd("Reader")
	mc.RecordLLMCall(100*time.Millisecond, 500)
	mc.RecordSandboxRun(200 * time.Millisecond)
	mc.RecordCommit()
	mc.RecordRetry()

	summary := mc.ExportSummary()
	if summary.Enabled {
		t.Errorf("expected summary of nil collector to be disabled")
	}

	if err := mc.ExportJSON("/tmp/nonexistent.json"); err != nil {
		t.Errorf("unexpected error on nil ExportJSON: %v", err)
	}
}

func TestMetricsCollector_Enabled(t *testing.T) {
	mc := NewMetricsCollector(true)
	if !mc.IsEnabled() {
		t.Fatalf("expected metrics collector to be enabled")
	}

	mc.RecordPhaseStart("Reader")
	time.Sleep(10 * time.Millisecond)
	mc.RecordPhaseEnd("Reader")

	// Phase end for non-existent start should not panic or fail
	mc.RecordPhaseEnd("NonExistent")

	mc.RecordLLMCall(200*time.Millisecond, 1000)
	mc.RecordLLMCall(300*time.Millisecond, 500)
	mc.RecordSandboxRun(150 * time.Millisecond)

	mc.RecordCommit()
	// Second commit should not overwrite first commit time
	firstTTFC := mc.ExportSummary().TimeToFileFirstCommit
	if firstTTFC == nil || *firstTTFC < 0 {
		t.Errorf("expected valid TTFC recorded on first commit")
	}
	time.Sleep(5 * time.Millisecond)
	mc.RecordCommit()

	mc.RecordRetry()
	mc.RecordRetry()

	summary := mc.ExportSummary()
	if !summary.Enabled {
		t.Errorf("expected summary Enabled to be true")
	}
	if summary.LLMCalls != 2 {
		t.Errorf("expected 2 LLM calls, got %d", summary.LLMCalls)
	}
	if summary.TotalTokensGenerated != 1500 {
		t.Errorf("expected 1500 tokens generated, got %d", summary.TotalTokensGenerated)
	}
	if summary.LLMWaitDurationMs < 500 {
		t.Errorf("expected LLM wait duration >= 500ms, got %d", summary.LLMWaitDurationMs)
	}
	if summary.TokensPerSecond <= 0 {
		t.Errorf("expected positive TokensPerSecond, got %f", summary.TokensPerSecond)
	}
	if summary.SandboxRuns != 1 {
		t.Errorf("expected 1 sandbox run, got %d", summary.SandboxRuns)
	}
	if summary.TotalRetries != 2 {
		t.Errorf("expected 2 retries, got %d", summary.TotalRetries)
	}
	if summary.PhaseLatenciesMs["Reader"] < 10 {
		t.Errorf("expected Reader phase latency >= 10ms, got %d", summary.PhaseLatenciesMs["Reader"])
	}

	// Test ExportJSON
	tmpDir, err := os.MkdirTemp("", "metrics-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer func() { _ = os.RemoveAll(tmpDir) }()

	jsonPath := filepath.Join(tmpDir, "metrics.json")
	if err := mc.ExportJSON(jsonPath); err != nil {
		t.Fatalf("failed to export metrics JSON: %v", err)
	}

	data, err := os.ReadFile(jsonPath)
	if err != nil {
		t.Fatalf("failed to read exported JSON: %v", err)
	}

	var readSummary MetricsSummary
	if err := json.Unmarshal(data, &readSummary); err != nil {
		t.Fatalf("failed to unmarshal exported JSON: %v", err)
	}

	if readSummary.LLMCalls != 2 || readSummary.TotalTokensGenerated != 1500 {
		t.Errorf("unmarshaled summary mismatch: %+v", readSummary)
	}
}

func TestMetricsCollector_Concurrency(t *testing.T) {
	mc := NewMetricsCollector(true)
	var wg sync.WaitGroup
	workers := 10

	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			phase := "WorkerPhase"
			mc.RecordPhaseStart(phase)
			mc.RecordLLMCall(50*time.Millisecond, 100)
			mc.RecordSandboxRun(30 * time.Millisecond)
			mc.RecordCommit()
			mc.RecordRetry()
			mc.RecordPhaseEnd(phase)
			_ = mc.ExportSummary()
		}(i)
	}

	wg.Wait()

	summary := mc.ExportSummary()
	if summary.LLMCalls != int64(workers) {
		t.Errorf("expected %d LLM calls under concurrency, got %d", workers, summary.LLMCalls)
	}
	if summary.SandboxRuns != int64(workers) {
		t.Errorf("expected %d sandbox runs under concurrency, got %d", workers, summary.SandboxRuns)
	}
	if summary.TotalRetries != int64(workers) {
		t.Errorf("expected %d retries under concurrency, got %d", workers, summary.TotalRetries)
	}
}
