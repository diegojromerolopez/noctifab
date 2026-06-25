package usecase

import (
	"context"
	"fmt"
	"time"

	"github.com/diegojromerolopez/noctifab/pkg/domain"
)

// PollInterval returns the configured polling interval for the execution loop.
// Used by external callers (e.g., serve.go) that drive the loop themselves.
func (o *Orchestrator) PollInterval() time.Duration {
	if o.cfg.PollInterval <= 0 {
		return 5 * time.Second
	}
	return o.cfg.PollInterval
}

// PlanStory calls the LLM planner for the given specification and saves the resulting
// task DAG into state. It is safe to call from both single-story and server-mode contexts.
func (o *Orchestrator) PlanStory(ctx context.Context, state *domain.State, spec string) error {
	if len(state.Tasks) > 0 {
		// Tasks already planned (e.g., resuming from saved state).
		return nil
	}

	prompt := fmt.Sprintf("Decompose specification into tasks:\n\n%s", spec)
	resp, err := o.llmClient.Complete(ctx, prompt)
	if err != nil {
		return fmt.Errorf("LLM planning failed: %w", err)
	}

	reg := NewToolRegistry()
	reg.Register(&AddTaskTool{})
	for _, action := range resp.Actions {
		if tool, ok := reg.Get(action.Tool); ok {
			_, _ = tool.Execute(ctx, state, action.Args)
		}
	}

	if err := o.repo.Save(ctx, state); err != nil {
		return fmt.Errorf("failed to persist planned tasks: %w", err)
	}

	fmt.Printf("📋 Plan created: %d tasks for story %s\n", len(state.Tasks), state.Metadata.FeatureName)
	return nil
}

// RefreshState reloads the current state from the repository.
// Convenience helper used by serve.go's story processing loop.
func (o *Orchestrator) RefreshState(ctx context.Context) (*domain.State, error) {
	return o.repo.Load(ctx)
}
