package services

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/diegojromerolopez/noctifab/pkg/domain"
	"github.com/diegojromerolopez/noctifab/pkg/infrastructure/llm"
)

func (o *Orchestrator) recordTokenUsage(ctx context.Context, prompt string, resp *domain.LLMResponse) {
	if o == nil || o.repo == nil || resp == nil {
		return
	}
	tokens := llm.EstimateUsageTokens(prompt, resp)
	if tokens <= 0 {
		return
	}
	taskID, _ := ctx.Value(TaskIDKey).(string)
	_ = o.updateStateWithRetry(ctx, func(st *domain.State) error {
		st.Metadata.TotalTokensUsed += tokens
		if taskID != "" {
			for i := range st.Tasks {
				if st.Tasks[i].ID == taskID {
					st.Tasks[i].TokensUsed += tokens
					break
				}
			}
		}
		return nil
	})
}

func (o *Orchestrator) registerAgentStart(ctx context.Context, role string, taskID string) {
	agentID := fmt.Sprintf("agent-%s-%s", role, taskID)
	name := fmt.Sprintf("%s-%s", role, taskID)
	if o.observer != nil {
		o.observer.Observe(ctx, domain.ExecutionEvent{
			Kind:              domain.EventAgentStarted,
			AgentInvocationID: agentID,
			AgentRole:         role,
			TaskID:            taskID,
			At:                time.Now().UTC(),
		})
	}
	updateErr := o.updateStateWithRetry(ctx, func(st *domain.State) error {
		found := false
		for i := range st.ActiveAgents {
			if st.ActiveAgents[i].ID == agentID {
				st.ActiveAgents[i].Status = domain.AgentWorking
				st.ActiveAgents[i].TaskID = taskID
				st.ActiveAgents[i].StartedAt = time.Now()
				st.ActiveAgents[i].CompletedAt = time.Time{}
				found = true
				break
			}
		}
		if !found {
			st.ActiveAgents = append(st.ActiveAgents, domain.Agent{
				ID:        agentID,
				Name:      name,
				Role:      domain.AgentRole(strings.ToUpper(role)),
				Status:    domain.AgentWorking,
				TaskID:    taskID,
				StartedAt: time.Now(),
			})
		}
		return nil
	})
	if updateErr != nil {
		fmt.Fprintf(os.Stderr, "Orchestrator: failed to register agent start for role %s task %s: %v\n", role, taskID, updateErr)
	}
}

func (o *Orchestrator) registerAgentComplete(ctx context.Context, role string, taskID string, err error) {
	agentID := fmt.Sprintf("agent-%s-%s", role, taskID)
	if o.observer != nil {
		outcome := domain.OutcomeSuccess
		if err != nil {
			outcome = domain.OutcomeFailed
		}
		o.observer.Observe(ctx, domain.ExecutionEvent{
			Kind:              domain.EventAgentFinished,
			AgentInvocationID: agentID,
			AgentRole:         role,
			TaskID:            taskID,
			Outcome:           outcome,
			At:                time.Now().UTC(),
		})
	}
	updateErr := o.updateStateWithRetry(ctx, func(st *domain.State) error {
		for i := range st.ActiveAgents {
			if st.ActiveAgents[i].ID == agentID {
				st.ActiveAgents[i].Status = domain.AgentCompleted
				st.ActiveAgents[i].CompletedAt = time.Now()
				if err != nil {
					st.ActiveAgents[i].LastError = err.Error()
				} else {
					st.ActiveAgents[i].LastError = ""
				}
				break
			}
		}
		return nil
	})
	if updateErr != nil {
		fmt.Fprintf(os.Stderr, "Orchestrator: failed to register agent completion for role %s task %s: %v\n", role, taskID, updateErr)
	}
}
