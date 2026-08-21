package services

import (
	"testing"
	"time"
)

func TestMetricsCollectorQAMetrics(t *testing.T) {
	t.Run("when QA events are recorded, it exports deterministic counters and totals", func(t *testing.T) {
		collector := NewMetricsCollector(true)
		collector.RecordQAPhase(125*time.Millisecond, 100)
		collector.RecordQAPhase(75*time.Millisecond, 50)
		collector.RecordQASkipped("disabled")
		collector.RecordQASkipped("disabled")
		collector.RecordQASkipped("not_applicable")
		collector.RecordQASkipped("contract-id-must-not-be-a-label")
		collector.RecordQADuplicateSuppressed()
		collector.RecordQAFixRound()
		collector.RecordQARegression()
		collector.RecordQAFindingDisposition("FIXED")
		collector.RecordQAFindingDisposition("arbitrary")

		summary := collector.Summary()
		if summary.QAPhaseCalls != 2 || summary.QATokensUsed != 150 || summary.QALatencyMs != 200 {
			t.Errorf("unexpected QA phase totals: %+v", summary)
		}
		if summary.QASkippedReasons["disabled"] != 2 || summary.QASkippedReasons["not_applicable"] != 1 {
			t.Errorf("unexpected skip counters: %v", summary.QASkippedReasons)
		}
		if summary.QADuplicatesSuppressed != 1 || summary.QAFixRounds != 1 || summary.QARegressionsFound != 1 {
			t.Errorf("unexpected QA outcome counters: %+v", summary)
		}
		if summary.QAFindingDispositions["FIXED"] != 1 {
			t.Errorf("unexpected disposition counters: %v", summary.QAFindingDispositions)
		}

		summary.QASkippedReasons["disabled"] = 99
		if collector.Summary().QASkippedReasons["disabled"] != 2 {
			t.Error("exported skip map aliases collector state")
		}
	})

	t.Run("when metrics are disabled, QA events are ignored", func(t *testing.T) {
		collector := NewMetricsCollector(false)
		collector.RecordQAPhase(time.Second, 10)
		collector.RecordQASkipped("disabled")
		collector.RecordQADuplicateSuppressed()
		collector.RecordQAFixRound()
		collector.RecordQARegression()
		collector.RecordQAFindingDisposition("OPEN")

		summary := collector.Summary()
		if summary.QAPhaseCalls != 0 || summary.QASkippedReasons["disabled"] != 0 || summary.QAFixRounds != 0 {
			t.Errorf("disabled collector recorded QA metrics: %+v", summary)
		}
	})
}
