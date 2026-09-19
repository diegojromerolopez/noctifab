package cli

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/diegojromerolopez/noctifab/pkg/domain"
	"github.com/diegojromerolopez/noctifab/pkg/infrastructure/config"
	"github.com/diegojromerolopez/noctifab/pkg/services"
)

func TestSovereignRescue_E2EGateAndAntiStub(t *testing.T) {
	t.Run("when unit tests pass but E2E fails, sovereign rescue rejects turn and feeds E2E failure into next turn", func(t *testing.T) {
		tempDir := t.TempDir()
		// Create docker-compose.e2e.yml so DetectE2ECommand detects E2E suite
		err := os.WriteFile(filepath.Join(tempDir, "docker-compose.e2e.yml"), []byte("version: '3.8'\nservices:\n  test-runner:\n    image: alpine\n"), 0644)
		require.NoError(t, err)

		mockRepo := &mockDAGStateRepo{
			state: &domain.State{
				ProjectPath: tempDir,
				Stories: []domain.Story{
					{ID: "story-0001", Title: "US-001-core", Status: domain.StoryFailed},
					{ID: "story-0002", Title: "US-002-next", Status: domain.StoryPending},
				},
				Tasks: []domain.Task{
					{ID: "US-001-TASK-001", StoryID: "US-001-core", Status: domain.TaskPending},
					{ID: "US-002-TASK-001", StoryID: "US-002-next", Status: domain.TaskPending},
				},
			},
		}

		mockLLM := &mockRescueLLM{
			responses: []*domain.LLMResponse{
				{Actions: []domain.LLMAction{{Tool: "noop"}}},
				{Actions: []domain.LLMAction{{Tool: "noop"}}},
			},
		}
		reg := services.NewToolRegistry()
		reg.Register(&services.NoopTool{})

		e2eCalls := 0
		sandbox := &mockRescueSandbox{
			runFunc: func(ctx context.Context, dir string, cmd string, pkg string) (string, error) {
				if cmd == "docker compose -f docker-compose.e2e.yml up --build --exit-code-from test-runner" {
					e2eCalls++
					if e2eCalls == 1 {
						return "FAILED: container exited with code 1\nrun_tests.sh not found", errors.New("exit status 1")
					}
					return "SUCCESS: 10/10 e2e assertions passed", nil
				}
				// Unit tests pass
				return "OK: unit tests passed", nil
			},
		}
		validator := services.NewTestValidator(sandbox, false, mockLLM, reg.Tools())

		opts := SovereignRescueOptions{
			TargetDir:     tempDir,
			Cfg:           config.DefaultConfig(),
			Repo:          mockRepo,
			StoryFiles:    []string{filepath.Join(tempDir, "US-001-core.md"), filepath.Join(tempDir, "US-002-next.md")},
			FailedStories: []string{"US-001-core.md (e2e failed)"},
			LLMClient:     mockLLM,
			ToolRegistry:  reg,
			Validator:     validator,
			SandboxRunner: sandbox,
			MaxTurns:      2,
		}

		rescueErr := DispatchSovereignRescue(context.Background(), opts)
		require.NoError(t, rescueErr)
		assert.Equal(t, 2, e2eCalls, "expected e2e to be executed on both turns")
		assert.Equal(t, 2, mockLLM.calls, "expected 2 turns because turn 1 E2E failed")

		// Verify turn 2 prompt contained the E2E failure diagnostics
		require.GreaterOrEqual(t, len(mockLLM.prompts), 2)
		assert.Contains(t, mockLLM.prompts[1], "E2E verification gate failed")
		assert.Contains(t, mockLLM.prompts[1], "run_tests.sh not found")

		// Verify state update: ONLY story-0001 was marked SUCCESS, story-0002 remains pending!
		finalState, sErr := mockRepo.Load(context.Background())
		require.NoError(t, sErr)
		for _, s := range finalState.Stories {
			switch s.ID {
			case "story-0001":
				assert.Equal(t, domain.StorySuccess, s.Status, "story-0001 should be marked success")
			case "story-0002":
				assert.Equal(t, domain.StoryPending, s.Status, "story-0002 must remain pending")
			}
		}
		for _, task := range finalState.Tasks {
			switch task.ID {
			case "US-001-TASK-001":
				assert.Equal(t, domain.TaskSuccess, task.Status, "US-001-TASK-001 should be marked success")
			case "US-002-TASK-001":
				assert.Equal(t, domain.TaskPending, task.Status, "US-002-TASK-001 must remain pending")
			}
		}
	})

	t.Run("when anti-stub validator finds tautological tests, sovereign rescue rejects turn", func(t *testing.T) {
		tempDir := t.TempDir()

		mockRepo := &mockDAGStateRepo{
			state: &domain.State{ProjectPath: tempDir},
		}

		// Turn 1 writes a tautological test file; Turn 2 fixes it with a genuine test
		mockLLM := &mockRescueLLM{
			responses: []*domain.LLMResponse{
				{
					Actions: []domain.LLMAction{
						{
							Tool: "write_file",
							Args: map[string]any{
								"path":    "tests/test_app.py",
								"content": "import unittest\nclass TestApp(unittest.TestCase):\n    def test_pass(self):\n        assert True\n",
							},
						},
					},
				},
				{
					Actions: []domain.LLMAction{
						{
							Tool: "write_file",
							Args: map[string]any{
								"path":    "tests/test_app.py",
								"content": "import unittest\nclass TestApp(unittest.TestCase):\n    def test_real(self):\n        self.assertEqual(len([1,2]), 2)\n",
							},
						},
					},
				},
			},
		}
		reg := services.NewToolRegistry()
		reg.Register(&services.WriteFileTool{})

		sandbox := &mockRescueSandbox{
			runFunc: func(ctx context.Context, dir string, cmd string, pkg string) (string, error) {
				return "OK", nil
			},
		}
		validator := services.NewTestValidator(sandbox, false, mockLLM, reg.Tools())

		opts := SovereignRescueOptions{
			TargetDir:     tempDir,
			Cfg:           config.DefaultConfig(),
			Repo:          mockRepo,
			FailedStories: []string{"US-001.md"},
			LLMClient:     mockLLM,
			ToolRegistry:  reg,
			Validator:     validator,
			SandboxRunner: sandbox,
			MaxTurns:      2,
		}

		rescueErr := DispatchSovereignRescue(context.Background(), opts)
		require.NoError(t, rescueErr)
		assert.Equal(t, 2, mockLLM.calls, "expected turn 1 to fail due to tautological test")

		// Verify turn 2 prompt contained the anti-stub violation
		require.GreaterOrEqual(t, len(mockLLM.prompts), 2)
		assert.Contains(t, mockLLM.prompts[1], "Anti-stub / anti-gaming validation failed")
		assert.Contains(t, mockLLM.prompts[1], "tautological_test_assertion")
	})
}
