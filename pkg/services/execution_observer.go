package services

import (
	"context"

	"github.com/diegojromerolopez/noctifab/pkg/domain"
)

type NoopExecutionReporter struct{}

func (n *NoopExecutionReporter) Observe(ctx context.Context, event domain.ExecutionEvent)   {}
func (n *NoopExecutionReporter) Start(ctx context.Context, run domain.RunMetadata)          {}
func (n *NoopExecutionReporter) BeginStory(ctx context.Context, story domain.StoryMetadata) {}
func (n *NoopExecutionReporter) EndStory(ctx context.Context, storyID string, outcome domain.ExecutionOutcome) {
}
func (n *NoopExecutionReporter) Finish(ctx context.Context, outcome domain.ExecutionOutcome) {}

type FanoutExecutionObserver struct {
	observers []domain.ExecutionObserver
}

func NewFanoutExecutionObserver(observers ...domain.ExecutionObserver) *FanoutExecutionObserver {
	var valid []domain.ExecutionObserver
	for _, obs := range observers {
		if obs != nil {
			valid = append(valid, obs)
		}
	}
	return &FanoutExecutionObserver{observers: valid}
}

func (f *FanoutExecutionObserver) Observe(ctx context.Context, event domain.ExecutionEvent) {
	for _, obs := range f.observers {
		obs.Observe(ctx, event)
	}
}

type ExecutionCorrelation struct {
	RunID             string
	StoryID           string
	TaskID            string
	AgentInvocationID string
	AgentRole         string
}

type correlationKey struct{}

func WithExecutionCorrelation(ctx context.Context, corr ExecutionCorrelation) context.Context {
	return context.WithValue(ctx, correlationKey{}, corr)
}

func FromExecutionCorrelation(ctx context.Context) ExecutionCorrelation {
	if corr, ok := ctx.Value(correlationKey{}).(ExecutionCorrelation); ok {
		return corr
	}
	return ExecutionCorrelation{}
}
