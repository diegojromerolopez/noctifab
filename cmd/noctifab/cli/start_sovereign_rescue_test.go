package cli

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/diegojromerolopez/noctifab/pkg/domain"
	"github.com/diegojromerolopez/noctifab/pkg/infrastructure/config"
	"github.com/diegojromerolopez/noctifab/pkg/services"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type mockRescueLLM struct {
	mu        sync.Mutex
	calls     int
	prompts   []string
	responses []*domain.LLMResponse
	errs      []error
}

func (m *mockRescueLLM) Complete(ctx context.Context, prompt string) (*domain.LLMResponse, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.prompts = append(m.prompts, prompt)
	idx := m.calls
	m.calls++
	if idx < len(m.errs) && m.errs[idx] != nil {
		return nil, m.errs[idx]
	}
	if idx < len(m.responses) {
		return m.responses[idx], nil
	}
	return &domain.LLMResponse{}, nil
}

type mockRescueSandbox struct {
	mu      sync.Mutex
	calls   int
	runFunc func(ctx context.Context, dir string, cmd string, pkg string) (string, error)
}

func (m *mockRescueSandbox) RunCommand(ctx context.Context, dir string, cmd string, pkg string) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.calls++
	if m.runFunc != nil {
		return m.runFunc(ctx, dir, cmd, pkg)
	}
	return "OK", nil
}

func TestSovereignProjectRescue(t *testing.T) {
	t.Run("when required dependencies are nil it returns an immediate error", func(t *testing.T) {
		err := runSovereignProjectRescue(context.Background(), SovereignRescueOptions{})
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "missing required LLM, registry, or validator dependencies")
	})

	t.Run("when pipeline leaves failed stories, sovereign rescue executes tool actions and marks all stories complete", func(t *testing.T) {
		tempDir := t.TempDir()
		specFile := filepath.Join(tempDir, "SPEC.md")
		require.NoError(t, os.WriteFile(specFile, []byte("# Project Spec\nBuild a fortune generator"), 0644))

		mockRepo := &mockDAGStateRepo{
			state: &domain.State{
				ProjectPath: tempDir,
				Stories: []domain.Story{
					{ID: "story-0001", Title: "US-001", Status: domain.StoryFailed},
					{ID: "story-0002", Title: "US-002", Status: domain.StoryRunning},
				},
				Tasks: []domain.Task{
					{ID: "task-1", Status: domain.TaskFailed},
					{ID: "task-2", Status: domain.TaskPending},
				},
			},
		}

		mockLLM := &mockRescueLLM{
			responses: []*domain.LLMResponse{
				{
					Actions: []domain.LLMAction{
						{
							Tool: "write_file",
							Args: map[string]any{
								"path":    "main.c",
								"content": "#include <stdio.h>\nint main() { printf(\"hello\\n\"); return 0; }\n",
							},
						},
						{
							Tool: "write_file",
							Args: map[string]any{
								"path":    "tests/test_fortune.c",
								"content": "#include <assert.h>\nint main() { assert(1 == 1); return 0; }\n",
							},
						},
						{
							Tool: "write_file",
							Args: map[string]any{
								"path":    "Makefile",
								"content": "build:\n\tgcc -o fortune main.c\ntest:\n\t./bin/test_suite\ne2e:\n\t./bin/test_suite --e2e\n",
							},
						},
					},
				},
			},
		}

		reg := services.NewToolRegistry()
		reg.Register(&services.WriteFileTool{})

		// Mock sandbox: initially fails, then succeeds after LLM writes files
		var sbCalls int
		sandbox := &mockRescueSandbox{
			runFunc: func(ctx context.Context, dir string, cmd string, pkg string) (string, error) {
				sbCalls++
				if sbCalls == 1 {
					// Pre-rescue diagnosis
					return "compiler error: missing main.c", errors.New("exit status 1")
				}
				// Verification after turn 1
				return "PASS: 5 tests passed", nil
			},
		}

		validator := services.NewTestValidator(sandbox, false, mockLLM, reg.Tools())

		opts := SovereignRescueOptions{
			TargetDir:     tempDir,
			Cfg:           config.DefaultConfig(),
			Repo:          mockRepo,
			GitClient:     services.NewGitClient(tempDir),
			StoryFiles:    []string{"US-001.md", "US-002.md"},
			FailedStories: []string{"US-001.md (compilation failed)", "US-002.md (missing tests)"},
			LLMClient:     mockLLM,
			ToolRegistry:  reg,
			Validator:     validator,
			MaxTurns:      2,
		}

		err := runSovereignProjectRescue(context.Background(), opts)
		require.NoError(t, err)

		// Assert files were created
		_, err = os.Stat(filepath.Join(tempDir, "main.c"))
		assert.NoError(t, err)
		_, err = os.Stat(filepath.Join(tempDir, "Makefile"))
		assert.NoError(t, err)

		// Assert state updated to success
		st, _ := mockRepo.Load(context.Background())
		assert.Equal(t, domain.BuildPassing, st.BuildStatus)
		assert.Equal(t, domain.StorySuccess, st.StoryStatus)
		for _, s := range st.Stories {
			assert.Equal(t, domain.StorySuccess, s.Status)
		}
		for _, task := range st.Tasks {
			if task.ID == "task-2" {
				assert.Equal(t, domain.TaskSuccess, task.Status)
			}
		}

		// Assert sovereign rescue telemetry action recorded
		lastAction := st.LastActions[len(st.LastActions)-1]
		assert.Equal(t, "sovereign_rescue_success", lastAction.Tool)
		assert.True(t, lastAction.Success)
	})

	t.Run("when sovereign rescue passes on turn 1 it does not waste turn 2", func(t *testing.T) {
		tempDir := t.TempDir()
		mockRepo := &mockDAGStateRepo{
			state: &domain.State{ProjectPath: tempDir},
		}

		mockLLM := &mockRescueLLM{
			responses: []*domain.LLMResponse{
				{
					Actions: []domain.LLMAction{
						{Tool: "noop"},
					},
				},
			},
		}

		reg := services.NewToolRegistry()
		reg.Register(&services.NoopTool{})

		sandbox := &mockRescueSandbox{
			runFunc: func(ctx context.Context, dir string, cmd string, pkg string) (string, error) {
				return "All 10 tests passed", nil
			},
		}
		validator := services.NewTestValidator(sandbox, false, mockLLM, reg.Tools())

		opts := SovereignRescueOptions{
			TargetDir:    tempDir,
			Repo:         mockRepo,
			LLMClient:    mockLLM,
			ToolRegistry: reg,
			Validator:    validator,
			MaxTurns:     3,
		}

		err := runSovereignProjectRescue(context.Background(), opts)
		require.NoError(t, err)
		assert.Equal(t, 1, mockLLM.calls, "Expected rescue to stop immediately on turn 1 when validator passes")
	})

	t.Run("when sovereign rescue exhausts all turns without passing it returns an error", func(t *testing.T) {
		tempDir := t.TempDir()
		mockRepo := &mockDAGStateRepo{
			state: &domain.State{ProjectPath: tempDir},
		}

		mockLLM := &mockRescueLLM{
			responses: []*domain.LLMResponse{
				{Actions: []domain.LLMAction{{Tool: "noop"}}},
				{Actions: []domain.LLMAction{{Tool: "noop"}}},
			},
		}

		reg := services.NewToolRegistry()
		reg.Register(&services.NoopTool{})

		sandbox := &mockRescueSandbox{
			runFunc: func(ctx context.Context, dir string, cmd string, pkg string) (string, error) {
				return "FAIL: persistent missing symbol _sqlite3_open", errors.New("exit status 1")
			},
		}
		validator := services.NewTestValidator(sandbox, false, mockLLM, reg.Tools())

		opts := SovereignRescueOptions{
			TargetDir:    tempDir,
			Repo:         mockRepo,
			LLMClient:    mockLLM,
			ToolRegistry: reg,
			Validator:    validator,
			MaxTurns:     2,
		}

		err := runSovereignProjectRescue(context.Background(), opts)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "sovereign rescue exhausted 2 turns")
		assert.Contains(t, err.Error(), "_sqlite3_open")
	})

	t.Run("when tool execution errors occur in turn 1, they are fed into turn 2 prompt diagnostics and resolved", func(t *testing.T) {
		tempDir := t.TempDir()
		mockRepo := &mockDAGStateRepo{
			state: &domain.State{ProjectPath: tempDir},
		}

		mockLLM := &mockRescueLLM{
			responses: []*domain.LLMResponse{
				{
					// Turn 1 emits an unknown tool
					Actions: []domain.LLMAction{
						{Tool: "non_existent_tool", Args: map[string]any{}},
					},
				},
				{
					// Turn 2 emits a valid tool
					Actions: []domain.LLMAction{
						{
							Tool: "write_file",
							Args: map[string]any{
								"path":    "solution.c",
								"content": "#include <stdio.h>\nint main() { printf(\"solved\\n\"); return 0; }\n",
							},
						},
					},
				},
			},
		}

		reg := services.NewToolRegistry()
		reg.Register(&services.WriteFileTool{})

		var calls int
		sandbox := &mockRescueSandbox{
			runFunc: func(ctx context.Context, dir string, cmd string, pkg string) (string, error) {
				calls++
				if calls <= 2 {
					return "missing solution.c", errors.New("exit status 1")
				}
				return "PASS", nil
			},
		}
		validator := services.NewTestValidator(sandbox, false, mockLLM, reg.Tools())

		opts := SovereignRescueOptions{
			TargetDir:    tempDir,
			Repo:         mockRepo,
			LLMClient:    mockLLM,
			ToolRegistry: reg,
			Validator:    validator,
			MaxTurns:     2,
		}

		err := runSovereignProjectRescue(context.Background(), opts)
		require.NoError(t, err)
		require.Len(t, mockLLM.prompts, 2)
		assert.Contains(t, mockLLM.prompts[1], "WORKSPACE TOOL EXECUTION ERRORS IN TURN 1")
		assert.Contains(t, mockLLM.prompts[1], "non_existent_tool")
	})

	t.Run("when parent context deadline is already exceeded, DispatchSovereignRescue allocates emergency runway and succeeds", func(t *testing.T) {
		tempDir := t.TempDir()
		mockRepo := &mockDAGStateRepo{
			state: &domain.State{ProjectPath: tempDir},
		}

		mockLLM := &mockRescueLLM{
			responses: []*domain.LLMResponse{
				{
					Actions: []domain.LLMAction{
						{Tool: "noop"},
					},
				},
			},
		}
		reg := services.NewToolRegistry()
		reg.Register(&services.NoopTool{})

		sandbox := &mockRescueSandbox{
			runFunc: func(ctx context.Context, dir string, cmd string, pkg string) (string, error) {
				return "PASS", nil
			},
		}
		validator := services.NewTestValidator(sandbox, false, mockLLM, reg.Tools())

		opts := SovereignRescueOptions{
			TargetDir:    tempDir,
			Repo:         mockRepo,
			LLMClient:    mockLLM,
			ToolRegistry: reg,
			Validator:    validator,
			MaxTurns:     1,
		}

		// Simulate expired loop context
		expiredCtx, cancel := context.WithDeadline(context.Background(), time.Now().Add(-1*time.Minute))
		defer cancel()

		err := DispatchSovereignRescue(expiredCtx, opts)
		require.NoError(t, err, "DispatchSovereignRescue should succeed by establishing dedicated emergency runway")
	})

	t.Run("when buildSovereignRescuePrompt is called, it includes strict JSON schema and anti-stub instructions", func(t *testing.T) {
		prompt := buildSovereignRescuePrompt("Build a CLI tool", []string{"US-001 (failed)"}, "error: build failed", 1, 2)
		assert.Contains(t, prompt, "REQUIRED RESPONSE FORMAT")
		assert.Contains(t, prompt, "\"reasoning\"")
		assert.Contains(t, prompt, "\"write_files\"")
		assert.Contains(t, prompt, "anti-stub validator")
		assert.Contains(t, prompt, "US-001 (failed)")
	})

	t.Run("when DispatchSovereignRescue is called without MaxTurns it defaults to 2 turns", func(t *testing.T) {
		tempDir := t.TempDir()
		mockRepo := &mockDAGStateRepo{
			state: &domain.State{ProjectPath: tempDir},
		}

		mockLLM := &mockRescueLLM{
			responses: []*domain.LLMResponse{
				{Actions: []domain.LLMAction{{Tool: "noop"}}},
				{Actions: []domain.LLMAction{{Tool: "noop"}}},
				{Actions: []domain.LLMAction{{Tool: "noop"}}},
			},
		}
		reg := services.NewToolRegistry()
		reg.Register(&services.NoopTool{})

		sandbox := &mockRescueSandbox{
			runFunc: func(ctx context.Context, dir string, cmd string, pkg string) (string, error) {
				return "FAIL", errors.New("exit status 1")
			},
		}
		validator := services.NewTestValidator(sandbox, false, mockLLM, reg.Tools())

		opts := SovereignRescueOptions{
			TargetDir:    tempDir,
			Cfg:          config.DefaultConfig(),
			Repo:         mockRepo,
			LLMClient:    mockLLM,
			ToolRegistry: reg,
			Validator:    validator,
			MaxTurns:     0, // Unset, should default to 2
		}

		err := DispatchSovereignRescue(context.Background(), opts)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "sovereign rescue exhausted 2 turns")
		assert.Equal(t, 2, mockLLM.calls, "expected exactly 2 turns from default configuration")
	})
}
