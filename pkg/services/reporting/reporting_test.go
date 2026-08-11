package reporting_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/diegojromerolopez/noctifab/pkg/domain"
	"github.com/diegojromerolopez/noctifab/pkg/infrastructure/reportfs"
	"github.com/diegojromerolopez/noctifab/pkg/services/reporting"
)

type FixedClock struct {
	NowTime time.Time
}

func (f FixedClock) Now() time.Time {
	return f.NowTime
}

func TestReportingPipeline(t *testing.T) {
	tmpDir := t.TempDir()
	reportPath := filepath.Join(tmpDir, "report.md")
	clock := FixedClock{NowTime: time.Date(2026, 8, 11, 22, 0, 0, 0, time.UTC)}
	writer := reportfs.NewAtomicWriter(reportfs.OSFileSystem{})

	t.Run("when running complete execution report pipeline", func(t *testing.T) {
		t.Run("it produces required section headings and status", func(t *testing.T) {
			agent, err := reporting.NewReporterAgent(reportPath, clock, writer, nil, nil)
			require.NoError(t, err)

			run := domain.RunMetadata{
				RunID:           "run-test-01",
				Command:         "noctifab start",
				ProjectPath:     "/work/project",
				ReportPath:      reportPath,
				StartedAt:       clock.Now(),
				NoctifabVersion: "2.0.0",
			}
			agent.Start(context.Background(), run)

			story := domain.StoryMetadata{
				StoryID:     "story-0001",
				FeatureName: "Test Feature",
				Sequence:    1,
			}
			agent.BeginStory(context.Background(), story)

			agent.Observe(context.Background(), domain.ExecutionEvent{
				ID:             "event-001",
				RunID:          "run-test-01",
				StoryID:        "story-0001",
				TaskID:         "task-01",
				Kind:           domain.EventTaskAttemptFinished,
				Outcome:        domain.OutcomeSuccess,
				DurationMillis: pointerInt64(1200),
			})

			agent.EndStory(context.Background(), "story-0001", domain.ExecutionSuccess)
			agent.Finish(context.Background(), domain.ExecutionSuccess)

			readBytes, readErr := os.ReadFile(reportPath)
			require.NoError(t, readErr)
			content := string(readBytes)

			assert.Contains(t, content, "# Noctifab Execution Report")
			assert.Contains(t, content, "> Status: SUCCESS")
			assert.Contains(t, content, "> Run ID: run-test-01")
			assert.Contains(t, content, "## Executive Summary")
			assert.Contains(t, content, "## Live Status")
			assert.Contains(t, content, "## Bottlenecks")
			assert.Contains(t, content, "## Issues Found")
			assert.Contains(t, content, "## Proposals and Next Actions")
		})
	})
}

func pointerInt64(v int64) *int64 {
	return &v
}
