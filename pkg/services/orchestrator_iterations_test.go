package services

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/diegojromerolopez/noctifab/pkg/domain"
)

// turnCountingLLM counts agent-loop turns (excluding the Reader phase) and
// always returns a non-noop action so the loop runs until its turn cap.
type turnCountingLLM struct {
	mu        sync.Mutex
	turnCalls int
}

func (m *turnCountingLLM) Complete(ctx context.Context, prompt string) (*domain.LLMResponse, error) {
	if strings.Contains(prompt, "Context Gathering phase") {
		return &domain.LLMResponse{Actions: []domain.LLMAction{{Tool: "noop"}}}, nil
	}
	m.mu.Lock()
	m.turnCalls++
	m.mu.Unlock()
	// Non-noop unknown tool: loop must continue to the next turn.
	return &domain.LLMResponse{Actions: []domain.LLMAction{{Tool: "unknown_tool"}}}, nil
}

func (m *turnCountingLLM) turns() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.turnCalls
}

func newIterationsTestOrchestrator(t *testing.T, llm domain.LLMClient, cfg OrchestratorConfig) (*Orchestrator, *domain.State) {
	t.Helper()
	state := &domain.State{ProjectPath: t.TempDir()}
	repo := &mockRepo{state: state}
	reg := NewToolRegistry()
	validator := NewPolicyValidator(nil, "main", nil)
	scheduler := NewScheduler(NewFileLockRegistry())
	git := NewGitClient(state.ProjectPath)
	queue := NewRebaseQueue(git)
	evaluator := NewTestValidator(nil, false, llm, nil)
	orch := NewOrchestrator(repo, reg, llm, validator, scheduler, git, queue, evaluator, &mockVCS{}, cfg, nil, nil, nil)
	return orch, state
}

func TestAgentLoopIterationsConfig(t *testing.T) {
	task := domain.Task{ID: "task-iter", Title: "Iterations Task", Description: "implement the widget counting logic"}

	t.Run("when GeneratorsIterations is configured, the generator loop uses it", func(t *testing.T) {
		llm := &turnCountingLLM{}
		orch, state := newIterationsTestOrchestrator(t, llm, OrchestratorConfig{
			PollInterval:         10 * time.Millisecond,
			GeneratorsIterations: 2,
		})
		orch.RunGeneratorAgent(context.Background(), task, state, nil, "", "implement")
		if got := llm.turns(); got != 2 {
			t.Errorf("expected 2 generator turns, got %d", got)
		}
	})

	t.Run("when GeneratorsIterations is zero, the generator loop defaults to 5 turns", func(t *testing.T) {
		llm := &turnCountingLLM{}
		orch, state := newIterationsTestOrchestrator(t, llm, OrchestratorConfig{
			PollInterval: 10 * time.Millisecond,
		})
		orch.RunGeneratorAgent(context.Background(), task, state, nil, "", "implement")
		if got := llm.turns(); got != 5 {
			t.Errorf("expected default 5 generator turns, got %d", got)
		}
	})

	t.Run("when TestersIterations is configured, the tester loop uses it", func(t *testing.T) {
		llm := &turnCountingLLM{}
		orch, state := newIterationsTestOrchestrator(t, llm, OrchestratorConfig{
			PollInterval:      10 * time.Millisecond,
			TestersIterations: 3,
		})
		orch.RunTesterAgent(context.Background(), task, state, nil, "write", "")
		if got := llm.turns(); got != 3 {
			t.Errorf("expected 3 tester turns, got %d", got)
		}
	})

	t.Run("when TestersIterations is negative, the tester loop defaults to 5 turns", func(t *testing.T) {
		llm := &turnCountingLLM{}
		orch, state := newIterationsTestOrchestrator(t, llm, OrchestratorConfig{
			PollInterval:      10 * time.Millisecond,
			TestersIterations: -1,
		})
		orch.RunTesterAgent(context.Background(), task, state, nil, "write", "")
		if got := llm.turns(); got != 5 {
			t.Errorf("expected default 5 tester turns, got %d", got)
		}
	})
}
