package services

import (
	"testing"
	"time"
)

func TestMetricsCollectorQAMetrics(t *testing.T) {
	t.Run("when QA events are recorded, it exports deterministic counters and totals", func(t *testing.T) {
		collector := NewMetricsCollector(true)
		collector.RecordQAPhase(125*time.Millisecond, 100, "0.10001")
		collector.RecordQAPhase(75*time.Millisecond, 50, "0.20002")
		collector.RecordQASkipped("disabled")
		collector.RecordQASkipped("disabled")
		collector.RecordQASkipped("not_applicable")
		collector.RecordQASkipped("contract-id-must-not-be-a-label")
		collector.RecordQADuplicateSuppressed()
		collector.RecordQAFixRound()
		collector.RecordQARegressionFound()
		collector.RecordQAFindingDisposition("FIXED")
		collector.RecordQAFindingDisposition("arbitrary")

		summary := collector.ExportSummary()
		if summary.QAPhaseCalls != 2 || summary.QATokensUsed != 150 || summary.QALatencyMs != 200 {
			t.Errorf("unexpected QA phase totals: %+v", summary)
		}
		if summary.QACostUSD != "0.30003" {
			t.Errorf("expected exact decimal cost 0.30003, got %q", summary.QACostUSD)
		}
		if summary.QASkippedReasons["disabled"] != 2 || summary.QASkippedReasons["not_applicable"] != 1 {
			t.Errorf("unexpected skip counters: %v", summary.QASkippedReasons)
		}
		if summary.QASkippedReasons["other"] != 1 {
			t.Errorf("expected unknown skip reasons to be bounded, got %v", summary.QASkippedReasons)
		}
		if summary.QADuplicatesSuppressed != 1 || summary.QAFixRounds != 1 || summary.QARegressionsFound != 1 {
			t.Errorf("unexpected QA outcome counters: %+v", summary)
		}
		if summary.QAFindingDispositions["FIXED"] != 1 {
			t.Errorf("unexpected disposition counters: %v", summary.QAFindingDispositions)
		}
		if summary.QAFindingDispositions["UNKNOWN"] != 1 {
			t.Errorf("expected unknown dispositions to be bounded, got %v", summary.QAFindingDispositions)
		}

		summary.QASkippedReasons["disabled"] = 99
		if collector.ExportSummary().QASkippedReasons["disabled"] != 2 {
			t.Error("exported skip map aliases collector state")
		}
	})

	t.Run("when metrics are disabled, QA events are ignored", func(t *testing.T) {
		collector := NewMetricsCollector(false)
		collector.RecordQAPhase(time.Second, 10, "1.0")
		collector.RecordQASkipped("disabled")
		collector.RecordQADuplicateSuppressed()
		collector.RecordQAFixRound()
		collector.RecordQARegressionFound()
		collector.RecordQAFindingDisposition("OPEN")

		summary := collector.ExportSummary()
		if summary.QAPhaseCalls != 0 || summary.QASkippedReasons["disabled"] != 0 || summary.QAFixRounds != 0 {
			t.Errorf("disabled collector recorded QA metrics: %+v", summary)
		}
	})
}
