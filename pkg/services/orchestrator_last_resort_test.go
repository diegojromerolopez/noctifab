package services

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/diegojromerolopez/noctifab/pkg/domain"
	"github.com/diegojromerolopez/noctifab/pkg/infrastructure/config"
	"github.com/diegojromerolopez/noctifab/pkg/infrastructure/prompts"
)

type mockLRASandbox struct {
	calls int
	out   string
	err   error
}

func (m *mockLRASandbox) RunCommand(ctx context.Context, projectPath string, command string, pkg string) (string, error) {
	m.calls++
	if m.calls > 1 {
		return "PASS", nil
	}
	return m.out, m.err
}

func TestOrchestrator_RunLastResortAgent_Success(t *testing.T) {
	reg := NewToolRegistry()
	reg.Register(&mockTool{name: "write_file"})

	mockLLM := &testMockLLM{
		responses: []*domain.LLMResponse{
			{
				Reasoning: "Fixing contradiction between tests and code",
				Actions: []domain.LLMAction{
					{
						Tool: "write_file",
						Args: map[string]interface{}{"path": "main.go", "content": "package main"},
					},
				},
			},
		},
	}

	sandbox := &mockLRASandbox{out: "FAIL: syntax error"}
	evaluator := NewTestValidator(sandbox, false, mockLLM, reg.Tools())

	cfg := OrchestratorConfig{
		LastResort: config.LastResortAgentConfig{
			Enabled:  true,
			MaxTurns: 2,
		},
	}

	orch := &Orchestrator{
		cfg:            cfg,
		llmClient:      mockLLM,
		registry:       reg,
		evaluator:      evaluator,
		promptRenderer: prompts.NewDefaultRenderer(),
	}

	task := domain.Task{
		ID:          "T-100",
		Title:       "Test Task",
		Description: "Task with contradictory signatures",
	}
	taskState := domain.State{ID: "story-1"}

	passed, _ := orch.RunLastResortAgent(context.Background(), &task, &taskState, nil, "compilation error: signature mismatch", "retries_exhausted")
	if !passed {
		t.Errorf("expected RunLastResortAgent to return true after successful fix")
	}

	// Verify database audit logging in taskState.LastActions
	if len(taskState.LastActions) < 2 {
		t.Fatalf("expected at least 2 LastActions (trigger + success), got %d", len(taskState.LastActions))
	}
	if taskState.LastActions[0].Tool != "last_resort_agent_trigger" {
		t.Errorf("expected first action to be 'last_resort_agent_trigger', got %q", taskState.LastActions[0].Tool)
	}
	if taskState.LastActions[len(taskState.LastActions)-1].Tool != "last_resort_agent_success" {
		t.Errorf("expected final action to be 'last_resort_agent_success', got %q", taskState.LastActions[len(taskState.LastActions)-1].Tool)
	}
}

func TestOrchestrator_RunLastResortAgent_Disabled(t *testing.T) {
	cfg := OrchestratorConfig{
		LastResort: config.LastResortAgentConfig{
			Enabled: false,
		},
	}

	orch := &Orchestrator{
		cfg: cfg,
	}

	task := domain.Task{ID: "T-200", Title: "Disabled Task"}
	taskState := domain.State{ID: "story-1"}

	passed, logOut := orch.RunLastResortAgent(context.Background(), &task, &taskState, nil, "some failure", "retries_exhausted")
	if passed {
		t.Errorf("expected RunLastResortAgent to return false when disabled")
	}
	if logOut != "some failure" {
		t.Errorf("expected original log trace to be preserved, got %q", logOut)
	}
}

type mockFailingSandbox struct{}

func (m *mockFailingSandbox) RunCommand(ctx context.Context, projectPath string, command string, pkg string) (string, error) {
	return "FAIL: persistent compilation error", errors.New("exit 1")
}

func TestOrchestrator_RunLastResortAgent_TurnsExhausted(t *testing.T) {
	mockLLM := &testMockLLM{
		responses: []*domain.LLMResponse{},
	}

	cfg := OrchestratorConfig{
		LastResort: config.LastResortAgentConfig{
			Enabled:  true,
			MaxTurns: 2,
			Timeout:  config.Duration(5 * time.Second),
		},
	}

	evaluator := NewTestValidator(&mockFailingSandbox{}, false, mockLLM, nil)

	orch := &Orchestrator{
		cfg:            cfg,
		llmClient:      mockLLM,
		registry:       NewToolRegistry(),
		evaluator:      evaluator,
		promptRenderer: prompts.NewDefaultRenderer(),
	}

	task := domain.Task{ID: "T-300", Title: "Failing Task"}
	taskState := domain.State{ID: "story-1"}

	passed, _ := orch.RunLastResortAgent(context.Background(), &task, &taskState, nil, "error log trace", "qa_gate_deadlock")
	if passed {
		t.Errorf("expected RunLastResortAgent to return false when all turns fail to pass tests")
	}

	// Verify database audit logging of trigger and failure
	if len(taskState.LastActions) < 2 {
		t.Fatalf("expected at least 2 LastActions (trigger + failure), got %d", len(taskState.LastActions))
	}
	if taskState.LastActions[0].Tool != "last_resort_agent_trigger" {
		t.Errorf("expected first action to be 'last_resort_agent_trigger', got %q", taskState.LastActions[0].Tool)
	}
	if taskState.LastActions[len(taskState.LastActions)-1].Tool != "last_resort_agent_failed" {
		t.Errorf("expected final action to be 'last_resort_agent_failed', got %q", taskState.LastActions[len(taskState.LastActions)-1].Tool)
	}
}
