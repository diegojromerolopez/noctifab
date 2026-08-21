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
	Enabled                bool             `json:"enabled"`
	StartTime              time.Time        `json:"start_time"`
	TotalExecutionSec      float64          `json:"total_execution_sec"`
	TimeToFileFirstCommit  *float64         `json:"time_to_first_commit_sec,omitempty"`
	PhaseLatenciesMs       map[string]int64 `json:"phase_latencies_ms"`
	LLMWaitDurationMs      int64            `json:"llm_wait_duration_ms"`
	LLMCalls               int64            `json:"llm_calls"`
	TotalTokensGenerated   int64            `json:"total_tokens_generated"`
	TokensPerSecond        float64          `json:"tokens_per_second"`
	SandboxDurationMs      int64            `json:"sandbox_duration_ms"`
	SandboxRuns            int64            `json:"sandbox_runs"`
	TotalRetries           int64            `json:"total_retries"`
	QAPhaseCalls           int64            `json:"qa_phase_calls"`
	QATokensUsed           int64            `json:"qa_tokens_used"`
	QALatencyMs            int64            `json:"qa_latency_ms"`
	QASkippedReasons       map[string]int64 `json:"qa_skipped_reasons"`
	QADuplicatesSuppressed int64            `json:"qa_duplicates_suppressed"`
	QAFixRounds            int64            `json:"qa_fix_rounds"`
	QARegressionsFound     int64            `json:"qa_regressions_found"`
	QAFindingDispositions  map[string]int64 `json:"qa_finding_dispositions"`
}

// MetricsCollector tracks real-time performance and speed metrics in a thread-safe manner.
type MetricsCollector struct {
	mu                     sync.RWMutex
	enabled                bool
	startTime              time.Time
	firstCommitTime        *time.Time
	phaseStarts            map[string]time.Time
	phaseLatenciesMs       map[string]int64
	llmWaitDurationMs      int64
	llmCalls               int64
	totalTokensGenerated   int64
	sandboxDurationMs      int64
	sandboxRuns            int64
	totalRetries           int64
	qaPhaseCalls           int64
	qaTokensUsed           int64
	qaLatencyMs            int64
	qaSkippedReasons       map[string]int64
	qaDuplicatesSuppressed int64
	qaFixRounds            int64
	qaRegressionsFound     int64
	qaFindingDispositions  map[string]int64
}

// NewMetricsCollector initializes a thread-safe metrics collector.
func NewMetricsCollector(enabled bool) *MetricsCollector {
	return &MetricsCollector{
		enabled:               enabled,
		startTime:             time.Now(),
		phaseStarts:           make(map[string]time.Time),
		phaseLatenciesMs:      make(map[string]int64),
		qaSkippedReasons:      make(map[string]int64),
		qaFindingDispositions: make(map[string]int64),
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

// RecordQAPhase registers one bounded QA review invocation and its resource totals.
func (m *MetricsCollector) RecordQAPhase(duration time.Duration, tokens int64) {
	if m == nil || !m.IsEnabled() {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.qaPhaseCalls++
	m.qaLatencyMs += duration.Milliseconds()
	if tokens > 0 {
		m.qaTokensUsed += tokens
	}
}

// RecordQASkipped increments the bounded skip-reason counter.
func (m *MetricsCollector) RecordQASkipped(reason string) {
	if m == nil || !m.IsEnabled() {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.qaSkippedReasons[reason]++
}

// RecordQADuplicateSuppressed increments the duplicate suppression counter.
func (m *MetricsCollector) RecordQADuplicateSuppressed() {
	if m == nil || !m.IsEnabled() {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.qaDuplicatesSuppressed++
}

// RecordQAFixRound increments the fix rounds count.
func (m *MetricsCollector) RecordQAFixRound() {
	if m == nil || !m.IsEnabled() {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.qaFixRounds++
}

// RecordQARegression increments the detected regression count.
func (m *MetricsCollector) RecordQARegression() {
	if m == nil || !m.IsEnabled() {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.qaRegressionsFound++
}

// RecordQAFindingDisposition tracks disposition outcomes.
func (m *MetricsCollector) RecordQAFindingDisposition(disposition string) {
	if m == nil || !m.IsEnabled() {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.qaFindingDispositions[disposition]++
}

func (m *MetricsCollector) timeToFirstCommit() *float64 {
	if m.firstCommitTime == nil {
		return nil
	}
	sec := m.firstCommitTime.Sub(m.startTime).Seconds()
	return &sec
}

// Summary returns a snapshot of all accumulated metrics.
func (m *MetricsCollector) Summary() MetricsSummary {
	if m == nil || !m.IsEnabled() {
		return MetricsSummary{Enabled: false}
	}
	m.mu.RLock()
	defer m.mu.RUnlock()

	var tokensPerSec float64
	if m.llmWaitDurationMs > 0 {
		tokensPerSec = float64(m.totalTokensGenerated) / (float64(m.llmWaitDurationMs) / 1000.0)
	}

	phaseLatencies := make(map[string]int64, len(m.phaseLatenciesMs))
	for k, v := range m.phaseLatenciesMs {
		phaseLatencies[k] = v
	}

	qaSkippedReasons := make(map[string]int64, len(m.qaSkippedReasons))
	for k, v := range m.qaSkippedReasons {
		qaSkippedReasons[k] = v
	}

	qaFindingDispositions := make(map[string]int64, len(m.qaFindingDispositions))
	for k, v := range m.qaFindingDispositions {
		qaFindingDispositions[k] = v
	}

	return MetricsSummary{
		Enabled:                m.enabled,
		StartTime:              m.startTime,
		TotalExecutionSec:      time.Since(m.startTime).Seconds(),
		TimeToFileFirstCommit:  m.timeToFirstCommit(),
		PhaseLatenciesMs:       phaseLatencies,
		LLMWaitDurationMs:      m.llmWaitDurationMs,
		LLMCalls:               m.llmCalls,
		TotalTokensGenerated:   m.totalTokensGenerated,
		TokensPerSecond:        tokensPerSec,
		SandboxDurationMs:      m.sandboxDurationMs,
		SandboxRuns:            m.sandboxRuns,
		TotalRetries:           m.totalRetries,
		QAPhaseCalls:           m.qaPhaseCalls,
		QATokensUsed:           m.qaTokensUsed,
		QALatencyMs:            m.qaLatencyMs,
		QASkippedReasons:       qaSkippedReasons,
		QADuplicatesSuppressed: m.qaDuplicatesSuppressed,
		QAFixRounds:            m.qaFixRounds,
		QARegressionsFound:     m.qaRegressionsFound,
		QAFindingDispositions:  qaFindingDispositions,
	}
}

// ExportSummary returns a copy of MetricsSummary.
func (m *MetricsCollector) ExportSummary() MetricsSummary {
	return m.Summary()
}

// ExportJSON writes the current metrics summary to a JSON file on disk.
func (m *MetricsCollector) ExportJSON(path string) error {
	if m == nil || !m.IsEnabled() {
		return nil
	}
	summary := m.Summary()
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
