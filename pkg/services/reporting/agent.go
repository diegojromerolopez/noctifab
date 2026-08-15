package reporting

import (
	"context"
	"fmt"
	"io"
	"os"
	"sync"
	"time"

	"github.com/diegojromerolopez/noctifab/pkg/domain"
)

type ReporterAgent struct {
	collector       *Collector
	writer          domain.ReportWriter
	renderer        *Renderer
	analyzerFactory domain.ReportAnalyzerFactory
	path            string
	warnings        io.Writer

	mu          sync.Mutex
	dirty       bool
	closed      bool
	circuitOpen bool
	flushTicker *time.Ticker
	stopChan    chan struct{}
	doneChan    chan struct{}
}

func NewReporterAgent(
	path string,
	clock domain.Clock,
	writer domain.ReportWriter,
	analyzerFactory domain.ReportAnalyzerFactory,
	warnings io.Writer,
) (*ReporterAgent, error) {
	if path == "" {
		return nil, fmt.Errorf("report path cannot be empty")
	}
	if writer == nil {
		return nil, fmt.Errorf("report writer cannot be nil")
	}
	if warnings == nil {
		warnings = os.Stderr
	}

	collector := NewCollector(clock)
	redactor := NewRedactor()
	renderer := NewRenderer(redactor)

	agent := &ReporterAgent{
		collector:       collector,
		writer:          writer,
		renderer:        renderer,
		analyzerFactory: analyzerFactory,
		path:            path,
		warnings:        warnings,
		flushTicker:     time.NewTicker(5 * time.Second),
		stopChan:        make(chan struct{}),
		doneChan:        make(chan struct{}),
	}

	go agent.flushLoop()
	return agent, nil
}

func (a *ReporterAgent) Start(ctx context.Context, run domain.RunMetadata) {
	a.collector.Start(ctx, run)
	a.markDirtyAndFlush(ctx)
}

func (a *ReporterAgent) BeginStory(ctx context.Context, story domain.StoryMetadata) {
	a.collector.BeginStory(ctx, story)
	a.markDirtyAndFlush(ctx)
}

func (a *ReporterAgent) EndStory(ctx context.Context, storyID string, outcome domain.ExecutionOutcome) {
	a.collector.EndStory(ctx, storyID, outcome)
	a.markDirtyAndFlush(ctx)
}

func (a *ReporterAgent) Observe(ctx context.Context, event domain.ExecutionEvent) {
	a.collector.Observe(ctx, event)
	a.mu.Lock()
	a.dirty = true
	a.mu.Unlock()
}

func (a *ReporterAgent) Finish(ctx context.Context, outcome domain.ExecutionOutcome) {
	a.mu.Lock()
	if a.closed {
		a.mu.Unlock()
		return
	}
	a.closed = true
	a.flushTicker.Stop()
	close(a.stopChan)
	a.mu.Unlock()

	<-a.doneChan

	// Finalize collector
	a.collector.Finish(ctx, outcome)
	snapshot := a.collector.Snapshot()

	// Optional terminal LLM analysis
	if a.analyzerFactory != nil {
		analyzer := a.analyzerFactory()
		if analyzer != nil {
			input := domain.ExecutionReportInput{
				RunID:               snapshot.Run.RunID,
				Outcome:             snapshot.Status,
				ExecutionWallMS:     &snapshot.ExecutionWallMS,
				DeterministicIssues: snapshot.Issues,
				Bottlenecks:         snapshot.Bottlenecks,
				Limitations:         snapshot.Limitations,
			}
			analysis, err := analyzer.Analyze(ctx, input)
			if err == nil {
				snapshot.Report = &analysis
				if len(analysis.Proposals) > 0 {
					snapshot.Proposals = append(snapshot.Proposals, analysis.Proposals...)
				}
			} else {
				snapshot.Limitations = append(snapshot.Limitations, fmt.Sprintf("Report analyzer error: %v", err))
			}
		}
	}

	// Render & write final atomic report
	content := a.renderer.RenderMarkdown(&snapshot)
	if err := a.writer.WriteAtomic(ctx, a.path, content); err != nil {
		_, _ = fmt.Fprintf(a.warnings, "noctifab report write failed: %v\n", err)
	}
}

func (a *ReporterAgent) markDirtyAndFlush(ctx context.Context) {
	a.mu.Lock()
	a.dirty = true
	a.mu.Unlock()
	_ = a.flushCheckpoint(ctx)
}

func (a *ReporterAgent) flushLoop() {
	defer close(a.doneChan)

	for {
		select {
		case <-a.flushTicker.C:
			_ = a.flushCheckpoint(context.Background())
		case <-a.stopChan:
			return
		}
	}
}

func (a *ReporterAgent) flushCheckpoint(ctx context.Context) error {
	a.mu.Lock()
	if !a.dirty || a.circuitOpen || a.closed {
		a.mu.Unlock()
		return nil
	}
	a.dirty = false
	a.mu.Unlock()

	snapshot := a.collector.Snapshot()
	content := a.renderer.RenderMarkdown(&snapshot)

	if err := a.writer.WriteAtomic(ctx, a.path, content); err != nil {
		a.mu.Lock()
		if !a.circuitOpen {
			a.circuitOpen = true
			_, _ = fmt.Fprintf(a.warnings, "noctifab report write failed: %v\n", err)
		}
		a.mu.Unlock()
		return err
	}
	return nil
}
