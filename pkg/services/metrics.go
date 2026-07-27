package services

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// MetricsSummary represents the snapshot of metrics gathered during dark factory execution.
type MetricsSummary struct {
	Enabled               bool             `json:"enabled"`
	StartTime             time.Time        `json:"start_time"`
	TotalExecutionSec     float64          `json:"total_execution_sec"`
	TimeToFileFirstCommit *float64         `json:"time_to_first_commit_sec,omitempty"`
	PhaseLatenciesMs      map[string]int64 `json:"phase_latencies_ms"`
	LLMWaitDurationMs     int64            `json:"llm_wait_duration_ms"`
	LLMCalls              int64            `json:"llm_calls"`
	TotalTokensGenerated  int64            `json:"total_tokens_generated"`
	TokensPerSecond       float64          `json:"tokens_per_second"`
	SandboxDurationMs     int64            `json:"sandbox_duration_ms"`
	SandboxRuns           int64            `json:"sandbox_runs"`
	TotalRetries          int64            `json:"total_retries"`
}

// MetricsCollector tracks real-time performance and speed metrics in a thread-safe manner.
type MetricsCollector struct {
	mu                   sync.RWMutex
	enabled              bool
	startTime            time.Time
	firstCommitTime      *time.Time
	phaseStarts          map[string]time.Time
	phaseLatenciesMs     map[string]int64
	llmWaitDurationMs    int64
	llmCalls             int64
	totalTokensGenerated int64
	sandboxDurationMs    int64
	sandboxRuns          int64
	totalRetries         int64
}

// NewMetricsCollector initializes a thread-safe metrics collector.
func NewMetricsCollector(enabled bool) *MetricsCollector {
	return &MetricsCollector{
		enabled:          enabled,
		startTime:        time.Now(),
		phaseStarts:      make(map[string]time.Time),
		phaseLatenciesMs: make(map[string]int64),
	}
}

// IsEnabled returns whether metrics tracking is enabled.
func (m *MetricsCollector) IsEnabled() bool {
	if m == nil {
		return false
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.enabled
}

// RecordPhaseStart marks the beginning of a specific pipeline phase.
func (m *MetricsCollector) RecordPhaseStart(phase string) {
	if m == nil || !m.IsEnabled() {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.phaseStarts[phase] = time.Now()
}

// RecordPhaseEnd marks the completion of a specific pipeline phase.
func (m *MetricsCollector) RecordPhaseEnd(phase string) {
	if m == nil || !m.IsEnabled() {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	start, exists := m.phaseStarts[phase]
	if !exists {
		return
	}
	duration := time.Since(start).Milliseconds()
	m.phaseLatenciesMs[phase] += duration
	delete(m.phaseStarts, phase)
}

// RecordLLMCall registers duration and token usage for an LLM request turn.
func (m *MetricsCollector) RecordLLMCall(duration time.Duration, tokens int) {
	if m == nil || !m.IsEnabled() {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.llmCalls++
	m.llmWaitDurationMs += duration.Milliseconds()
	if tokens > 0 {
		m.totalTokensGenerated += int64(tokens)
		fmt.Printf("💰 [Token Telemetry] Turn LLM tokens=%d (cumulative tokens=%d, total calls=%d)\n", tokens, m.totalTokensGenerated, m.llmCalls)
	}
}

// RecordSandboxRun registers container/sandbox test execution time.
func (m *MetricsCollector) RecordSandboxRun(duration time.Duration) {
	if m == nil || !m.IsEnabled() {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.sandboxRuns++
	m.sandboxDurationMs += duration.Milliseconds()
}

// RecordCommit registers a git commit event and records TTFC if it is the first commit.
func (m *MetricsCollector) RecordCommit() {
	if m == nil || !m.IsEnabled() {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.firstCommitTime == nil {
		now := time.Now()
		m.firstCommitTime = &now
	}
}

// RecordRetry increments the retry count.
func (m *MetricsCollector) RecordRetry() {
	if m == nil || !m.IsEnabled() {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.totalRetries++
}

// ExportSummary generates a thread-safe snapshot of all metrics collected so far.
func (m *MetricsCollector) ExportSummary() MetricsSummary {
	if m == nil {
		return MetricsSummary{Enabled: false}
	}
	m.mu.RLock()
	defer m.mu.RUnlock()

	now := time.Now()
	totalSec := now.Sub(m.startTime).Seconds()

	var ttfc *float64
	if m.firstCommitTime != nil {
		val := m.firstCommitTime.Sub(m.startTime).Seconds()
		ttfc = &val
	}

	latencies := make(map[string]int64, len(m.phaseLatenciesMs))
	for k, v := range m.phaseLatenciesMs {
		latencies[k] = v
	}

	llmSec := float64(m.llmWaitDurationMs) / 1000.0
	var tokensPerSec float64
	if llmSec > 0 {
		tokensPerSec = float64(m.totalTokensGenerated) / llmSec
	}

	return MetricsSummary{
		Enabled:               m.enabled,
		StartTime:             m.startTime,
		TotalExecutionSec:     totalSec,
		TimeToFileFirstCommit: ttfc,
		PhaseLatenciesMs:      latencies,
		LLMWaitDurationMs:     m.llmWaitDurationMs,
		LLMCalls:              m.llmCalls,
		TotalTokensGenerated:  m.totalTokensGenerated,
		TokensPerSecond:       tokensPerSec,
		SandboxDurationMs:     m.sandboxDurationMs,
		SandboxRuns:           m.sandboxRuns,
		TotalRetries:          m.totalRetries,
	}
}

// ExportJSON writes the current metrics summary to a JSON file on disk.
func (m *MetricsCollector) ExportJSON(path string) error {
	if m == nil || !m.IsEnabled() {
		return nil
	}
	summary := m.ExportSummary()
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create metrics directory: %w", err)
	}

	data, err := json.MarshalIndent(summary, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal metrics summary: %w", err)
	}

	if err := os.WriteFile(path, data, 0644); err != nil {
		return fmt.Errorf("failed to write metrics json file: %w", err)
	}
	return nil
}
