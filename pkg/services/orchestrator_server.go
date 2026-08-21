package services

import (
	"context"
	"fmt"
	"time"

	"github.com/diegojromerolopez/noctifab/pkg/domain"
	"github.com/diegojromerolopez/noctifab/pkg/infrastructure/prompts"
	"github.com/diegojromerolopez/noctifab/pkg/infrastructure/telemetry"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
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
	ctx, span := telemetry.Tracer().Start(ctx, "PlanStory",
		trace.WithAttributes(
			attribute.String("feature_name", state.Metadata.FeatureName),
			attribute.Int("existing_tasks", len(state.Tasks)),
		))
	defer span.End()
	if len(state.Tasks) > 0 {
		// Tasks already planned (e.g., resuming from saved state).
		return nil
	}

	rendered, err := o.promptRenderer.Render(prompts.AgentPlanner, "decompose", prompts.PlannerPromptData{Spec: spec})
	if err != nil {
		return fmt.Errorf("planner prompt rendering failed: %w", err)
	}
	prompt := rendered.Full()
	plannerCtx := context.WithValue(ctx, AgentRoleKey, "planner")
	// Compaction must never rewrite the output contract at the end of the prompt.
	plannerCtx = domain.WithUncompactableTail(plannerCtx, len(rendered.Contract))

	maxAttempts := 3
	var lastErr error
	for attempt := 1; attempt <= maxAttempts; attempt++ {
		resp, err := o.llmClient.Complete(plannerCtx, prompt)
		o.recordTokenUsage(ctx, prompt, resp)
		if err != nil {
			lastErr = fmt.Errorf("LLM planning failed: %w", err)
			continue
		}

		reg := NewToolRegistry()
		reg.Register(&AddTaskTool{})
		for _, action := range resp.Actions {
			if tool, ok := reg.Get(action.Tool); ok {
				_, _ = tool.Execute(ctx, state, action.Args)
			}
		}

		// Validate the planned tasks
		if err := ValidatePlannedTasks(state.Tasks, state.ProjectPath); err != nil {
			state.Tasks = nil
			lastErr = err
			if attempt < maxAttempts {
				fmt.Printf("⚠️ Planning attempt %d/%d produced no valid tasks (%v). Retrying...\n", attempt, maxAttempts, err)
			}
			continue
		}

		if err := o.repo.Save(ctx, state); err != nil {
			return fmt.Errorf("failed to persist planned tasks: %w", err)
		}

		fmt.Printf("📋 Plan created: %d tasks for story %s\n", len(state.Tasks), state.Metadata.FeatureName)
		return nil
	}

	return lastErr
}

// RefreshState reloads the current state from the repository.
// Convenience helper used by serve.go's story processing loop.
func (o *Orchestrator) RefreshState(ctx context.Context) (*domain.State, error) {
	return o.repo.Load(ctx)
}
