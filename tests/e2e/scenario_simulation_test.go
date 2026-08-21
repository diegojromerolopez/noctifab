package e2e

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/diegojromerolopez/noctifab/pkg/domain"
)

func runSimulatedOrchestrator(ctx context.Context, repo domain.StateRepository, client *mockLLMClient, workspace string, tokenLimit int64) error {
	var currentTokens int64

	state, err := repo.Load(context.Background())
	if err != nil {
		return err
	}

	// Requirements file verification
	reqPath := filepath.Join(workspace, state.Metadata.InputPath)
	if _, err := os.Stat(reqPath); err != nil {
		return fmt.Errorf("requirements file not found: %w", err)
	}

	files, err := scanWorkspaceFiles(workspace)
	if err == nil {
		state.Files = files
	}

	state.ActiveAgents = []domain.Agent{
		{ID: "agent-planner", Name: "Planner", Role: domain.AgentRolePlanner, Status: domain.AgentIdle},
		{ID: "agent-generator", Name: "Generator", Role: domain.AgentRoleGenerator, Status: domain.AgentIdle},
		{ID: "agent-tester", Name: "Tester", Role: domain.AgentRoleTester, Status: domain.AgentIdle},
	}

	// Recover interrupted tasks on startup
	for i := range state.Tasks {
		if state.Tasks[i].Status == domain.TaskInterrupted {
			state.Tasks[i].Status = domain.TaskPending
		}
	}

	if err := repo.Save(context.Background(), state); err != nil {
		return err
	}

	for cycle := 0; cycle < 15; cycle++ {
		// Graceful shutdown context check
		select {
		case <-ctx.Done():
			state, loadErr := repo.Load(context.Background())
			if loadErr == nil {
				// Mark any in-progress tasks as INTERRUPTED
				for i := range state.Tasks {
					if state.Tasks[i].Status == domain.TaskInProgress {
						state.Tasks[i].Status = domain.TaskInterrupted
					}
				}
				state.LastActions = append(state.LastActions, domain.Action{
					Timestamp: time.Now(),
					Tool:      "graceful_shutdown",
					Success:   true,
					Result:    "Graceful shutdown triggered",
				})
				_ = repo.Save(context.Background(), state)
			}
			return ctx.Err()
		default:
		}

		state, err = repo.Load(ctx)
		if err != nil {
			return err
		}

		if len(state.Tasks) == 0 {
			clarificationRequested, err := simulatePlanningStep(ctx, repo, client, state, tokenLimit, &currentTokens)
			if err != nil {
				return err
			}
			if clarificationRequested {
				return nil
			}
			continue
		}

		var readyTask *domain.Task
		for i := range state.Tasks {
			t := &state.Tasks[i]
			if t.Status == domain.TaskPending {
				depsMet := true
				for _, dep := range t.DependsOn {
					for _, other := range state.Tasks {
						if (other.ID == dep || other.Title == dep) && other.Status != domain.TaskSuccess {
							depsMet = false
							break
						}
					}
				}
				if depsMet {
					readyTask = t
					break
				}
			}
		}

		if readyTask != nil {
			if err := simulateTaskExecutionStep(ctx, repo, client, state, readyTask, workspace, tokenLimit, &currentTokens); err != nil {
				return err
			}
			continue
		}

		allCompleted := true
		for _, tTask := range state.Tasks {
			if tTask.Status != domain.TaskSuccess {
				allCompleted = false
				break
			}
		}

		if allCompleted {
			return simulateFinalValidationStep(ctx, repo, state, workspace)
		}
	}
	return nil
}
