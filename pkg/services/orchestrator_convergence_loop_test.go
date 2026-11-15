package services

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/diegojromerolopez/noctifab/pkg/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type sequenceMockSandbox struct {
	mu          sync.Mutex
	testOutputs []string
	testErrs    []error
	testCount   int
}

func (s *sequenceMockSandbox) RunCommand(ctx context.Context, projectPath string, command string, pkg string) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Deterministic formatters return cleanly
	if strings.Contains(command, "gofmt") || strings.Contains(command, "format") || strings.Contains(command, "fmt") {
		return "", nil
	}

	idx := s.testCount
	s.testCount++
	if idx < len(s.testOutputs) {
		return s.testOutputs[idx], s.testErrs[idx]
	}
	return "ok", nil
}

func TestConvergenceLoop_DeterministicFixConvergesTurn1(t *testing.T) {
	repoDir, _, cleanup := setupTestGitRepo(t)
	defer cleanup()

	task := &domain.Task{
		ID:          "task-conv-det",
		Title:       "Deterministic Fix Task",
		TargetFiles: []string{"main.go"},
		Status:      domain.TaskPending,
		MaxRetries:  1,
	}

	state := &domain.State{
		ID:          "state-conv-det",
		ProjectPath: repoDir,
		Tasks:       []domain.Task{*task},
		Metadata: domain.StateMetadata{
			BaseBranch:        "main",
			IntegrationBranch: "noctifab/feature-conv-det",
		},
	}

	repo := &mockRepo{state: state}
	reg := NewToolRegistry()
	reg.Register(&mockTool{name: "read_file"})
	reg.Register(&mockTool{name: "write_file"})
	reg.Register(&mockTool{name: "edit_file"})
	reg.Register(&mockTool{name: "run_tests"})

	llmClient := &testMockLLM{}
	validator := NewPolicyValidator(nil, "main", nil)
	scheduler := NewScheduler(NewFileLockRegistry())
	git := NewGitClient(repoDir)
	queue := NewRebaseQueue(git)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go queue.Start(ctx)

	// In this test, after deterministic auto-fix on turn 1, test validation passes immediately
	sandbox := &sequenceMockSandbox{
		testOutputs: []string{
			"PASS\nok\n",
		},
		testErrs: []error{
			nil,
		},
	}

	evaluator := NewTestValidator(sandbox, false, llmClient, nil)
	vcsClient := &mockVCS{}
	cfg := OrchestratorConfig{
		Concurrency:  1,
		UseWorktrees: false,
	}

	orch := NewOrchestrator(repo, reg, llmClient, validator, scheduler, git, queue, evaluator, vcsClient, cfg, nil, nil, nil)

	passed, log := orch.executeConvergenceRepairLoop(ctx, task, state, git, "syntax error: unformatted code", "noctifab/feature-conv-det")
	assert.True(t, passed, "expected convergence loop to pass via deterministic auto-fix on turn 1")
	assert.Contains(t, log, "All validation runs passed successfully")
	// LLM should not have been called because deterministic fix resolved it
	assert.Empty(t, llmClient.prompts, "expected zero LLM prompts dispatched when deterministic fix succeeds")
}

func TestConvergenceLoop_AgentRepairConvergesTurn2(t *testing.T) {
	repoDir, _, cleanup := setupTestGitRepo(t)
	defer cleanup()

	// Write broken code with line 5 having the syntax error
	srcFile := filepath.Join(repoDir, "broken.go")
	brokenCode := `package main

func Calc() int {
    val := 10
    return val +
}
`
	require.NoError(t, os.WriteFile(srcFile, []byte(brokenCode), 0644))
	git := NewGitClient(repoDir)
	_, _ = git.Run(context.Background(), true, "add", "broken.go")
	_, _ = git.Run(context.Background(), true, "commit", "-m", "add broken.go")

	task := &domain.Task{
		ID:          "task-conv-agent",
		Title:       "Agent Convergence Task",
		TargetFiles: []string{"broken.go"},
		Status:      domain.TaskPending,
		MaxRetries:  1,
	}

	state := &domain.State{
		ID:          "state-conv-agent",
		ProjectPath: repoDir,
		Tasks:       []domain.Task{*task},
		Metadata: domain.StateMetadata{
			BaseBranch:        "main",
			IntegrationBranch: "noctifab/feature-conv-agent",
		},
	}

	repo := &mockRepo{state: state}
	reg := NewToolRegistry()
	reg.Register(&mockTool{name: "read_file"})
	reg.Register(&mockTool{name: "write_file"})
	reg.Register(&mockTool{name: "edit_file"})
	reg.Register(&mockTool{name: "run_tests"})

	// LLM responds with noop
	llmClient := &testMockLLM{
		responses: []*domain.LLMResponse{
			{Actions: []domain.LLMAction{{Tool: "noop"}}},
			{Actions: []domain.LLMAction{{Tool: "noop"}}},
			{Actions: []domain.LLMAction{{Tool: "noop"}}},
		},
	}

	validator := NewPolicyValidator(nil, "main", nil)
	scheduler := NewScheduler(NewFileLockRegistry())
	queue := NewRebaseQueue(git)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go queue.Start(ctx)

	// Turn 1 deterministic fix fails, Turn 1 validation fails,
	// Turn 2 deterministic fix fails, Turn 2 re-validation passes
	sandbox := &sequenceMockSandbox{
		testOutputs: []string{
			"broken.go:5:17: syntax error: unexpected newline, expecting expression", // turn 1 det check
			"broken.go:5:17: syntax error: unexpected newline, expecting expression", // turn 1 agent check
			"broken.go:5:17: syntax error: unexpected newline, expecting expression", // turn 2 det check
			"PASS\nok\n", // turn 2 agent check passes
		},
		testErrs: []error{
			errors.New("exit 1"),
			errors.New("exit 1"),
			errors.New("exit 1"),
			nil,
		},
	}

	evaluator := NewTestValidator(sandbox, false, llmClient, nil)
	vcsClient := &mockVCS{}
	cfg := OrchestratorConfig{
		Concurrency:  1,
		UseWorktrees: false,
	}

	orch := NewOrchestrator(repo, reg, llmClient, validator, scheduler, git, queue, evaluator, vcsClient, cfg, nil, nil, nil)

	passed, log := orch.executeConvergenceRepairLoop(ctx, task, state, git, "broken.go:5:17: syntax error: unexpected newline", "noctifab/feature-conv-agent")
	assert.True(t, passed, "expected convergence loop to pass on turn 2")
	assert.Contains(t, log, "All validation runs passed successfully")

	// Verify that the prompt sent to the LLM contained the offending code snippet from disk
	foundOffendingSnippet := false
	for _, p := range llmClient.prompts {
		if strings.Contains(p, "OFFENDING CODE SNIPPET(S) FROM DISK") &&
			strings.Contains(p, "File: broken.go (around line 5)") &&
			strings.Contains(p, ">>    5 |     return val +   <-- OFFENDING CODE") {
			foundOffendingSnippet = true
			break
		}
	}
	assert.True(t, foundOffendingSnippet, "expected prompt to contain extracted offending code snippet with pointer >>")
}
