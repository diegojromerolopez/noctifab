package services

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/diegojromerolopez/noctifab/pkg/domain"
)

type mockVCS struct {
	prCalls    int
	mergeCalls int
}

func (m *mockVCS) CreatePullRequest(ctx context.Context, title, body, headBranch, baseBranch string) (string, error) {
	m.prCalls++
	return "https://github.com/owner/repo/pull/1", nil
}

func (m *mockVCS) MergePullRequest(ctx context.Context, prID string) error {
	m.mergeCalls++
	return nil
}

type mockRepo struct {
	state *domain.State
}

func (m *mockRepo) Load(ctx context.Context) (*domain.State, error) {
	return m.state, nil
}

func (m *mockRepo) LoadByID(ctx context.Context, id string) (*domain.State, error) {
	return m.state, nil
}

func (m *mockRepo) LoadAll(ctx context.Context) ([]*domain.State, error) {
	return []*domain.State{m.state}, nil
}

func (m *mockRepo) Save(ctx context.Context, s *domain.State) error {
	m.state = s
	return nil
}

func (m *mockRepo) Close() error {
	return nil
}

type mockLLM struct {
	resp *domain.LLMResponse
}

func (m *mockLLM) Complete(ctx context.Context, prompt string) (*domain.LLMResponse, error) {
	return m.resp, nil
}

func TestOrchestrator_Initialization(t *testing.T) {
	state := &domain.State{
		ID:          "session-1",
		ProjectPath: "/tmp",
	}

	repo := &mockRepo{state: state}
	reg := NewToolRegistry()
	llmClient := &mockLLM{}
	validator := NewPolicyValidator(nil, "main", nil)
	scheduler := NewScheduler(NewFileLockRegistry())
	git := NewGitClient("/tmp")
	queue := NewRebaseQueue(git)
	evaluator := NewTestValidator(NewHostSandbox(nil, "", 0, nil), false, nil, nil)
	vcsClient := &mockVCS{}

	cfg := OrchestratorConfig{
		PollInterval: 10 * time.Millisecond,
		Concurrency:  1,
	}

	orch := NewOrchestrator(repo, reg, llmClient, validator, scheduler, git, queue, evaluator, vcsClient, cfg, nil, nil)
	if orch == nil {
		t.Fatal("expected orchestrator instance, got nil")
		return
	}

	if orch.vcsClient != vcsClient {
		t.Error("vcs client not wired correctly")
	}
}

type mockRepairHandler struct {
	attempts     int
	ReturnResult *RepairResult
	ReturnError  error
}

func (m *mockRepairHandler) AttemptRepair(ctx context.Context, state *domain.State, task domain.Task, watchdogOutput string, watchdogErr error) (*RepairResult, error) {
	m.attempts++
	return m.ReturnResult, m.ReturnError
}

func TestOrchestrator_RepairIntegration(t *testing.T) {
	t.Run("watchdogRepair nil does not attempt repair on failure", func(t *testing.T) {
		state := &domain.State{
			ID: "test-session",
			Tasks: []domain.Task{{
				ID: "task-1", Title: "Test", Description: "A test task",
				Status: domain.TaskInProgress, MaxRetries: 3,
			}},
		}
		repo := &mockRepo{state: state}
		reg := NewToolRegistry()
		llmClient := &mockLLM{}
		validator := NewPolicyValidator(nil, "main", nil)
		scheduler := NewScheduler(NewFileLockRegistry())
		git := NewGitClient("/tmp")
		queue := NewRebaseQueue(git)
		evaluator := NewTestValidator(NewHostSandbox(nil, "", 0, nil), false, nil, nil)
		vcsClient := &mockVCS{}
		cfg := OrchestratorConfig{PollInterval: 10 * time.Millisecond, Concurrency: 1}

		orch := NewOrchestrator(repo, reg, llmClient, validator, scheduler, git, queue, evaluator, vcsClient, cfg, nil, nil)
		if orch.watchdogRepair != nil {
			t.Error("expected watchdogRepair to be nil")
		}
	})

	t.Run("repair handler injected correctly", func(t *testing.T) {
		mockRepair := &mockRepairHandler{
			ReturnResult: &RepairResult{Success: true, Attempts: 1},
		}
		state := &domain.State{
			ID: "test-session",
			Tasks: []domain.Task{{
				ID: "task-1", Title: "Test", Description: "A test task",
				Status: domain.TaskInProgress, MaxRetries: 3,
			}},
		}
		repo := &mockRepo{state: state}
		reg := NewToolRegistry()
		llmClient := &mockLLM{}
		validator := NewPolicyValidator(nil, "main", nil)
		scheduler := NewScheduler(NewFileLockRegistry())
		git := NewGitClient("/tmp")
		queue := NewRebaseQueue(git)
		evaluator := NewTestValidator(NewHostSandbox(nil, "", 0, nil), false, nil, nil)
		vcsClient := &mockVCS{}
		cfg := OrchestratorConfig{PollInterval: 10 * time.Millisecond, Concurrency: 1}

		orch := NewOrchestrator(repo, reg, llmClient, validator, scheduler, git, queue, evaluator, vcsClient, cfg, nil, mockRepair)
		if orch.watchdogRepair == nil {
			t.Error("expected watchdogRepair to be set")
		}
		if _, ok := orch.watchdogRepair.(*mockRepairHandler); !ok {
			t.Error("expected watchdogRepair to be mockRepairHandler")
		}
	})
}

func TestSummarizeFailureLog(t *testing.T) {
	t.Run("Extract error and failure lines", func(t *testing.T) {
		inputLog := "some generic log line\nERROR: test_failure\n  Traceback info here\nFAIL: test_another_failure\n  Some more details\nsuccess lines"
		expected := "ERROR: test_failure\n  Traceback info here\nFAIL: test_another_failure\n  Some more details\nsuccess lines"
		result := summarizeFailureLog(inputLog)
		if result != expected {
			t.Errorf("expected:\n%q\ngot:\n%q", expected, result)
		}
	})

	t.Run("Extract inline exception or failure keywords", func(t *testing.T) {
		inputLog := "line 1\nImportError: module not found\nline 3\nException occurred here"
		expected := "ImportError: module not found\nException occurred here"
		result := summarizeFailureLog(inputLog)
		if result != expected {
			t.Errorf("expected:\n%q\ngot:\n%q", expected, result)
		}
	})

	t.Run("Fallback to last 15 lines", func(t *testing.T) {
		lines := []string{
			"line 1", "line 2", "line 3", "line 4", "line 5",
			"line 6", "line 7", "line 8", "line 9", "line 10",
			"line 11", "line 12", "line 13", "line 14", "line 15",
			"line 16", "line 17",
		}
		inputLog := strings.Join(lines, "\n")
		expected := strings.Join(lines[2:], "\n") // last 15 lines (from line 3 to 17)
		result := summarizeFailureLog(inputLog)
		if result != expected {
			t.Errorf("expected:\n%q\ngot:\n%q", expected, result)
		}
	})
}
