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
		t.Run("it produces required section headings, status, and human readable durations", func(t *testing.T) {
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

			pTokens := int64(100)
			cTokens := int64(50)
			agent.Observe(context.Background(), domain.ExecutionEvent{
				ID:               "event-001",
				RunID:            "run-test-01",
				StoryID:          "story-0001",
				TaskID:           "task-01",
				Kind:             domain.EventTaskAttemptFinished,
				Outcome:          domain.OutcomeSuccess,
				DurationMillis:   pointerInt64(1200),
				PromptTokens:     &pTokens,
				CompletionTokens: &cTokens,
			})

			agent.EndStory(context.Background(), "story-0001", domain.ExecutionSuccess)
			agent.Finish(context.Background(), domain.ExecutionSuccess)

			readBytes, readErr := os.ReadFile(reportPath)
			require.NoError(t, readErr)
			content := string(readBytes)

			assert.Contains(t, content, "# Noctifab Execution Report")
			assert.Contains(t, content, "> Status: SUCCESS")
			assert.Contains(t, content, "> Run ID: run-test-01")
			assert.NotContains(t, content, "Checkpoint:")
			assert.Contains(t, content, "## Executive Summary")
			assert.Contains(t, content, "## Execution Status")
			assert.Contains(t, content, "## Agent Performance")
			assert.Contains(t, content, "## Phase Performance")
			assert.Contains(t, content, "## Codebase Changes & Workspace Impact")
			assert.Contains(t, content, "## Self-Correction and Turn Efficiency")
			assert.Contains(t, content, "## User Story and Task Results")
			assert.Contains(t, content, "## LLM and Token Usage")
			assert.Contains(t, content, "- **Tokens:** 150")
			assert.NotContains(t, content, "Total Cost USD")
			assert.Contains(t, content, "1s 200ms")
		})

		t.Run("it includes all created workspace artifacts in the execution report", func(t *testing.T) {
			projDir := t.TempDir()
			require.NoError(t, os.MkdirAll(filepath.Join(projDir, "app"), 0755))
			require.NoError(t, os.WriteFile(filepath.Join(projDir, "app", "main.py"), []byte("print('hello')\n"), 0644))
			require.NoError(t, os.WriteFile(filepath.Join(projDir, "pyproject.toml"), []byte("[project]\nname='test'\n"), 0644))

			repFile := filepath.Join(projDir, "report.md")
			agent, err := reporting.NewReporterAgent(repFile, clock, writer, nil, nil)
			require.NoError(t, err)

			run := domain.RunMetadata{
				RunID:       "run-test-02",
				Command:     "noctifab start",
				ProjectPath: projDir,
				ReportPath:  repFile,
				StartedAt:   clock.Now(),
			}
			agent.Start(context.Background(), run)
			agent.Finish(context.Background(), domain.ExecutionSuccess)

			readBytes, readErr := os.ReadFile(repFile)
			require.NoError(t, readErr)
			content := string(readBytes)

			assert.NotContains(t, content, "### Modified & Created Files")
			assert.NotContains(t, content, "### Created & Modified Artifacts")
			assert.Contains(t, content, "### Filesystem Hierarchy")
			assert.Contains(t, content, "├── app/")
			assert.Contains(t, content, "│   └── main.py")
		})

		t.Run("it renders empty filesystem tree when no files provided", func(t *testing.T) {
			assert.Empty(t, reporting.RenderFilesystemTree(nil))
			assert.Empty(t, reporting.RenderFilesystemTree([]string{}))
		})

		t.Run("it renders multi-loop convergence matrix when convergence data is present", func(t *testing.T) {
			renderer := reporting.NewRenderer(nil)
			snapshot := &reporting.ReportSnapshot{
				Run: domain.RunMetadata{
					RunID:           "run-test-conv",
					Command:         "noctifab start",
					ProjectPath:     "/work/project",
					ReportPath:      reportPath,
					StartedAt:       clock.Now(),
					NoctifabVersion: "2.0.0",
				},
				Status: domain.ExecutionSuccess,
				Convergence: []reporting.LoopConvergenceSummary{
					{
						LoopIndex:             1,
						StoriesAttempted:      4,
						StoriesSucceeded:      3,
						RemediationsTriggered: 2,
						TokensUsed:            125000,
						DurationMS:            180000,
						Outcome:               domain.ExecutionFailed,
					},
					{
						LoopIndex:             2,
						StoriesAttempted:      1,
						StoriesSucceeded:      1,
						RemediationsTriggered: 1,
						TokensUsed:            45000,
						DurationMS:            65000,
						Outcome:               domain.ExecutionSuccess,
					},
				},
			}

			output := string(renderer.RenderMarkdown(snapshot))
			assert.Contains(t, output, "## Multi-Loop Convergence Matrix")
			assert.Contains(t, output, "| **Loop 1** | 4 | 3 | 2 | 125000 | 3m | FAILED |")
			assert.Contains(t, output, "| **Loop 2** | 1 | 1 | 1 | 45000 | 1m 5s | SUCCESS |")
		})
	})
}

func pointerInt64(v int64) *int64 {
	return &v
}

