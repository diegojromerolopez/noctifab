package services

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/diegojromerolopez/noctifab/pkg/domain"
	"github.com/diegojromerolopez/noctifab/pkg/infrastructure/prompts"
)

// promptCapturingLLM records every prompt it receives and always noops.
type promptCapturingLLM struct {
	mu      sync.Mutex
	prompts []string
}

func (m *promptCapturingLLM) Complete(_ context.Context, prompt string) (*domain.LLMResponse, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.prompts = append(m.prompts, prompt)
	return &domain.LLMResponse{Actions: []domain.LLMAction{{Tool: "noop"}}}, nil
}

func newPromptTestOrchestrator(t *testing.T, tempDir string, llm domain.LLMClient, renderer PromptRenderer) *Orchestrator {
	t.Helper()
	state := &domain.State{ProjectPath: tempDir}
	repo := &mockRepo{state: state}
	reg := NewToolRegistry()
	reg.Register(&customTool{
		name: "run_tests",
		executeFn: func(ctx context.Context, state *domain.State, args map[string]any) (string, error) {
			return "tests passed", nil
		},
	})
	validator := NewPolicyValidator(nil, "main", nil)
	scheduler := NewScheduler(NewFileLockRegistry())
	git := NewGitClient(tempDir)
	queue := NewRebaseQueue(git)
	evaluator := NewTestValidator(nil, false, llm, nil)
	cfg := OrchestratorConfig{PollInterval: 10 * time.Millisecond}
	return NewOrchestrator(repo, reg, llm, validator, scheduler, git, queue, evaluator, &mockVCS{}, cfg, nil, nil, renderer)
}

func TestOrchestratorPromptRendering(t *testing.T) {
	t.Run("when no renderer is injected the generator receives the embedded default prompt with contract", func(t *testing.T) {
		tempDir := t.TempDir()
		llm := &promptCapturingLLM{}
		orch := newPromptTestOrchestrator(t, tempDir, llm, nil)
		state := &domain.State{ProjectPath: tempDir}
		task := domain.Task{ID: "t1", Title: "My task", Description: "Do the thing"}

		orch.RunGeneratorAgent(context.Background(), task, state, nil, "", "implement")

		if len(llm.prompts) == 0 {
			t.Fatal("expected at least one LLM call")
		}
		got := llm.prompts[len(llm.prompts)-1]
		for _, needle := range []string{
			"You are acting as the Generator Agent.",
			"My task - Do the thing",
			"Focus on creating the minimal implementation/functionality",
			"ANTI-STALLING MANDATE:",
		} {
			if !strings.Contains(got, needle) {
				t.Errorf("generator prompt missing %q", needle)
			}
		}
		if !strings.Contains(got, prompts.Contract(prompts.AgentGenerator)) {
			t.Error("generator prompt missing the output contract block")
		}
	})

	t.Run("when a workspace override exists the generator uses the custom body and still receives the contract", func(t *testing.T) {
		tempDir := t.TempDir()
		promptDir := filepath.Join(tempDir, ".noctifab", "prompts", "generator")
		if err := os.MkdirAll(promptDir, 0755); err != nil {
			t.Fatal(err)
		}
		custom := "CUSTOM GENERATOR PROMPT for {{.Title}}: {{.Description}}{{.Context}}\n"
		if err := os.WriteFile(filepath.Join(promptDir, "implement.tmpl"), []byte(custom), 0644); err != nil {
			t.Fatal(err)
		}
		renderer, err := prompts.NewRenderer(tempDir, nil)
		if err != nil {
			t.Fatalf("renderer construction failed: %v", err)
		}

		llm := &promptCapturingLLM{}
		orch := newPromptTestOrchestrator(t, tempDir, llm, renderer)
		state := &domain.State{ProjectPath: tempDir}
		task := domain.Task{ID: "t1", Title: "My task", Description: "Do the thing"}

		orch.RunGeneratorAgent(context.Background(), task, state, nil, "", "implement")

		if len(llm.prompts) == 0 {
			t.Fatal("expected at least one LLM call")
		}
		got := llm.prompts[len(llm.prompts)-1]
		if !strings.Contains(got, "CUSTOM GENERATOR PROMPT for My task: Do the thing") {
			t.Errorf("expected custom body, got:\n%s", got)
		}
		if !strings.Contains(got, prompts.Contract(prompts.AgentGenerator)) {
			t.Error("custom prompt missing the non-overridable output contract block")
		}
	})

	t.Run("when the action is unknown the agent run aborts without calling the LLM", func(t *testing.T) {
		tempDir := t.TempDir()
		llm := &promptCapturingLLM{}
		orch := newPromptTestOrchestrator(t, tempDir, llm, nil)
		state := &domain.State{ProjectPath: tempDir}
		task := domain.Task{ID: "t1", Title: "T", Description: "D"}

		orch.RunGeneratorAgent(context.Background(), task, state, nil, "", "nonexistent")
		orch.RunTesterAgent(context.Background(), task, state, nil, "nonexistent", "")

		if len(llm.prompts) != 0 {
			t.Errorf("expected no LLM calls for unknown actions, got %d", len(llm.prompts))
		}
	})

	t.Run("when the tester fix action runs the feedback is embedded in the prompt", func(t *testing.T) {
		tempDir := t.TempDir()
		llm := &promptCapturingLLM{}
		orch := newPromptTestOrchestrator(t, tempDir, llm, nil)
		state := &domain.State{ProjectPath: tempDir}
		task := domain.Task{ID: "t1", Title: "T", Description: "D"}

		orch.RunTesterAgent(context.Background(), task, state, nil, "fix", "the mock asserts the wrong value")

		if len(llm.prompts) == 0 {
			t.Fatal("expected at least one LLM call")
		}
		got := llm.prompts[len(llm.prompts)-1]
		if !strings.Contains(got, "Feedback from generator agent:\nthe mock asserts the wrong value") {
			t.Errorf("tester fix prompt missing feedback, got:\n%s", got)
		}
	})
}
