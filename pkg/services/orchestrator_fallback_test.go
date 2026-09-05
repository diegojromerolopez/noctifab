package services

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/diegojromerolopez/noctifab/pkg/domain"
	"github.com/diegojromerolopez/noctifab/pkg/infrastructure/config"
	"github.com/diegojromerolopez/noctifab/pkg/infrastructure/prompts"
	"github.com/stretchr/testify/assert"
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

func TestOrchestrator_RunFallbackAgent_Success(t *testing.T) {
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
		Fallback: config.FallbackAgentConfig{
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

	passed, _ := orch.RunFallbackAgent(context.Background(), &task, &taskState, nil, "compilation error: signature mismatch", "retries_exhausted")
	if !passed {
		t.Errorf("expected RunFallbackAgent to return true after successful fix")
	}

	// Verify database audit logging in taskState.LastActions
	if len(taskState.LastActions) < 2 {
		t.Fatalf("expected at least 2 LastActions (trigger + success), got %d", len(taskState.LastActions))
	}
	if taskState.LastActions[0].Tool != "fallback_agent_trigger" && taskState.LastActions[0].Tool != "last_resort_agent_trigger" {
		t.Errorf("expected first action to be 'fallback_agent_trigger' or 'last_resort_agent_trigger', got %q", taskState.LastActions[0].Tool)
	}
	if taskState.LastActions[len(taskState.LastActions)-1].Tool != "fallback_agent_success" && taskState.LastActions[len(taskState.LastActions)-1].Tool != "last_resort_agent_success" {
		t.Errorf("expected final action to be 'fallback_agent_success' or 'last_resort_agent_success', got %q", taskState.LastActions[len(taskState.LastActions)-1].Tool)
	}
	assert.True(t, task.FallbackUsed)
	assert.True(t, task.LastResortUsed)
}

func TestOrchestrator_RunFallbackAgent_Disabled(t *testing.T) {
	cfg := OrchestratorConfig{
		Fallback: config.FallbackAgentConfig{
			Enabled: false,
		},
	}

	orch := &Orchestrator{
		cfg: cfg,
	}

	task := domain.Task{ID: "T-200", Title: "Disabled Task"}
	taskState := domain.State{ID: "story-1"}

	passed, logOut := orch.RunFallbackAgent(context.Background(), &task, &taskState, nil, "some failure", "retries_exhausted")
	if passed {
		t.Errorf("expected RunFallbackAgent to return false when disabled")
	}
	if logOut != "some failure" {
		t.Errorf("expected original log trace to be preserved, got %q", logOut)
	}
}

type mockFailingSandbox struct{}

func (m *mockFailingSandbox) RunCommand(ctx context.Context, projectPath string, command string, pkg string) (string, error) {
	return "FAIL: persistent compilation error", errors.New("exit 1")
}

func TestOrchestrator_RunFallbackAgent_TurnsExhausted(t *testing.T) {
	mockLLM := &testMockLLM{
		responses: []*domain.LLMResponse{},
	}

	cfg := OrchestratorConfig{
		Fallback: config.FallbackAgentConfig{
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

	passed, _ := orch.RunFallbackAgent(context.Background(), &task, &taskState, nil, "error log trace", "qa_gate_deadlock")
	if passed {
		t.Errorf("expected RunFallbackAgent to return false when all turns fail to pass tests")
	}

	// Verify database audit logging of trigger and failure
	if len(taskState.LastActions) < 2 {
		t.Fatalf("expected at least 2 LastActions (trigger + failure), got %d", len(taskState.LastActions))
	}
	if taskState.LastActions[0].Tool != "fallback_agent_trigger" && taskState.LastActions[0].Tool != "last_resort_agent_trigger" {
		t.Errorf("expected first action to be 'fallback_agent_trigger' or 'last_resort_agent_trigger', got %q", taskState.LastActions[0].Tool)
	}
	if taskState.LastActions[len(taskState.LastActions)-1].Tool != "fallback_agent_failed" && taskState.LastActions[len(taskState.LastActions)-1].Tool != "last_resort_agent_failed" {
		t.Errorf("expected final action to be 'fallback_agent_failed' or 'last_resort_agent_failed', got %q", taskState.LastActions[len(taskState.LastActions)-1].Tool)
	}
}

func TestBuildFallbackContext_SanitizesSecrets(t *testing.T) {
	rawLog := "Error: connection failed with api_key=sk-secretkey123456789012345678901234 and ghp_111122223333444455556666777788889999"
	rawDiff := "+ token: \"sk-anothersecret999888777666555444333222\"\n- token: \"old\""

	contextBlock := buildFallbackContext(rawLog, rawDiff, "fallback_stall_escalation", 1, 2)

	if strings.Contains(contextBlock, "sk-secretkey123456789012345678901234") {
		t.Errorf("expected failure log secret to be redacted from context block")
	}
	if strings.Contains(contextBlock, "ghp_111122223333444455556666777788889999") {
		t.Errorf("expected ghp token to be redacted from context block")
	}
	if strings.Contains(contextBlock, "sk-anothersecret999888777666555444333222") {
		t.Errorf("expected diff secret to be redacted from context block")
	}
	if !strings.Contains(contextBlock, "[REDACTED_SECRET]") {
		t.Errorf("expected [REDACTED_SECRET] placeholder in sanitized context block")
	}
}

func TestOrchestrator_RunFallbackAgent_StallEscalation(t *testing.T) {
	reg := NewToolRegistry()
	reg.Register(&mockTool{name: "write_file"})

	mockLLM := &testMockLLM{
		responses: []*domain.LLMResponse{
			{
				Reasoning: "Resolving repeated stall deadlock",
				Actions: []domain.LLMAction{
					{
						Tool: "write_file",
						Args: map[string]interface{}{"path": "service.go", "content": "package service"},
					},
				},
			},
		},
	}

	sandbox := &mockLRASandbox{out: "FAIL: deadlocked"}
	evaluator := NewTestValidator(sandbox, false, mockLLM, reg.Tools())

	cfg := OrchestratorConfig{
		Fallback: config.FallbackAgentConfig{
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
		ID:                "T-400",
		Title:             "Stalled Task",
		StallCount:        4,
		RecoveryDirective: "SOVEREIGN REPAIR DIRECTIVE: Task T-400 stalled 4 times.",
	}
	taskState := domain.State{ID: "story-1"}

	passed, _ := orch.RunLastResortAgent(context.Background(), &task, &taskState, nil, task.RecoveryDirective, "fallback_stall_escalation")
	if !passed {
		t.Errorf("expected RunLastResortAgent to return true for stall escalation resolution")
	}
	if !task.FallbackUsed {
		t.Errorf("expected task.FallbackUsed to be true")
	}
}

func TestOrchestrator_RunFallbackAgent_NilGuards(t *testing.T) {
	cfg := OrchestratorConfig{
		Fallback: config.FallbackAgentConfig{
			Enabled:  true,
			MaxTurns: 2,
		},
	}

	// Orchestrator with nil llmClient, registry, evaluator should return false safely without panic
	orch := &Orchestrator{
		cfg: cfg,
	}

	passed, logOut := orch.RunFallbackAgent(context.Background(), nil, nil, nil, "some failure", "retries_exhausted")
	assert.False(t, passed)
	assert.Equal(t, "some failure", logOut)
}
