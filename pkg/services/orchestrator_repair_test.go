package services

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/diegojromerolopez/noctifab/pkg/domain"
)

type mockLLMRepairClient struct {
	responses []domain.LLMResponse
	callCount int
}

func (m *mockLLMRepairClient) Complete(ctx context.Context, prompt string) (*domain.LLMResponse, error) {
	if m.callCount < len(m.responses) {
		resp := m.responses[m.callCount]
		m.callCount++
		return &resp, nil
	}
	return &domain.LLMResponse{Reasoning: "noop"}, nil
}

func (m *mockLLMRepairClient) Stream(ctx context.Context, prompt string) (<-chan string, error) {
	ch := make(chan string)
	close(ch)
	return ch, nil
}

type mockPostMergeRepairSandbox struct {
	passSequence []bool
	callCount    int
}

func (m *mockPostMergeRepairSandbox) RunCommand(ctx context.Context, projectPath string, command string, pkg string) (string, error) {
	if m.callCount < len(m.passSequence) {
		pass := m.passSequence[m.callCount]
		m.callCount++
		if pass {
			return "PASS\nok", nil
		}
		return "FAIL\nAssertion error in test_module.go", os.ErrInvalid
	}
	return "PASS\nok", nil
}

func initGitRepoForRepairTest(t *testing.T, dir string) *GitClient {
	t.Helper()
	runCmd := func(args ...string) {
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		cmd.Env = append(os.Environ(), "GIT_AUTHOR_NAME=test", "GIT_AUTHOR_EMAIL=test@test.com", "GIT_COMMITTER_NAME=test", "GIT_COMMITTER_EMAIL=test@test.com")
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v failed: %v\nOutput: %s", args, err, string(out))
		}
	}

	runCmd("init", "-b", "main")
	_ = os.WriteFile(filepath.Join(dir, "README.md"), []byte("# Test Repo\n"), 0644)
	runCmd("add", "README.md")
	runCmd("commit", "-m", "initial commit")
	runCmd("checkout", "-b", "noctifab/feature-test")

	return NewGitClient(dir)
}

func TestOrchestrator_RunPostMergeRepairPhase(t *testing.T) {
	t.Run("when global test suite passes on first attempt it returns immediately without repairs", func(t *testing.T) {
		tmp := t.TempDir()
		gitClient := initGitRepoForRepairTest(t, tmp)

		sandbox := &mockPostMergeRepairSandbox{passSequence: []bool{true}}
		validator := NewTestValidator(sandbox, false, nil, nil)
		llm := &mockLLMRepairClient{}

		orch := &Orchestrator{
			git:       gitClient,
			evaluator: validator,
			llmClient: llm,
		}

		state := &domain.State{
			ID:          "test-story-1",
			ProjectPath: tmp,
			Metadata: domain.StateMetadata{
				BaseBranch:        "main",
				IntegrationBranch: "noctifab/feature-test",
				FeatureName:       "Test Feature",
			},
		}

		err := orch.RunPostMergeRepairPhase(context.Background(), state)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if llm.callCount != 0 {
			t.Errorf("expected 0 LLM calls, got %d", llm.callCount)
		}
	})

	t.Run("when global test suite fails and repair agent fixes it on turn 1", func(t *testing.T) {
		tmp := t.TempDir()
		gitClient := initGitRepoForRepairTest(t, tmp)

		sandbox := &mockPostMergeRepairSandbox{passSequence: []bool{false, true}}
		validator := NewTestValidator(sandbox, false, nil, nil)
		llm := &mockLLMRepairClient{
			responses: []domain.LLMResponse{
				{
					Reasoning: "Fixed assertions",
					Actions: []domain.LLMAction{
						{
							Tool: "write_file",
							Args: map[string]interface{}{
								"path":    "repaired.txt",
								"content": "repaired content",
							},
						},
					},
				},
			},
		}

		registry := NewToolRegistry()
		registry.Register(&mockWriteFileTool{})

		orch := &Orchestrator{
			git:       gitClient,
			evaluator: validator,
			llmClient: llm,
			registry:  registry,
		}

		state := &domain.State{
			ID:          "test-story-2",
			ProjectPath: tmp,
			Metadata: domain.StateMetadata{
				BaseBranch:        "main",
				IntegrationBranch: "noctifab/feature-test",
				FeatureName:       "Test Feature",
			},
		}

		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		err := orch.RunPostMergeRepairPhase(ctx, state)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if llm.callCount != 1 {
			t.Errorf("expected 1 LLM call, got %d", llm.callCount)
		}
	})
}

type mockWriteFileTool struct{}

func (m *mockWriteFileTool) Name() string {
	return "write_file"
}

func (m *mockWriteFileTool) Description() string {
	return "mock write file"
}

func (m *mockWriteFileTool) Execute(ctx context.Context, state *domain.State, args map[string]any) (string, error) {
	relPath, _ := args["path"].(string)
	content, _ := args["content"].(string)
	fullPath := filepath.Join(state.ProjectPath, relPath)
	_ = os.WriteFile(fullPath, []byte(content), 0644)
	return "ok", nil
}
