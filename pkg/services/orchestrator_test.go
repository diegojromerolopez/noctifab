package services

import (
	"context"
	"encoding/json"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/diegojromerolopez/noctifab/pkg/domain"
	"github.com/stretchr/testify/assert"
)

type mockVCS struct {
	mu         sync.Mutex
	prCalls    int
	mergeCalls int
}

func (m *mockVCS) CreatePullRequest(ctx context.Context, title, body, headBranch, baseBranch string) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.prCalls++
	return "https://github.com/owner/repo/pull/1", nil
}

func (m *mockVCS) MergePullRequest(ctx context.Context, prID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.mergeCalls++
	return nil
}

type mockRepo struct {
	mu    sync.Mutex
	state *domain.State
}

func (m *mockRepo) cloneState(s *domain.State) *domain.State {
	if s == nil {
		return nil
	}
	bytes, err := json.Marshal(s)
	if err != nil {
		return s
	}
	var copyState domain.State
	if err := json.Unmarshal(bytes, &copyState); err != nil {
		return s
	}
	return &copyState
}

func (m *mockRepo) Load(ctx context.Context) (*domain.State, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.cloneState(m.state), nil
}

func (m *mockRepo) LoadByID(ctx context.Context, id string) (*domain.State, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.cloneState(m.state), nil
}

func (m *mockRepo) LoadAll(ctx context.Context) ([]*domain.State, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return []*domain.State{m.cloneState(m.state)}, nil
}

func (m *mockRepo) Save(ctx context.Context, s *domain.State) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.state = m.cloneState(s)
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

type mockConflictRepo struct {
	mockRepo
	cMu       sync.Mutex
	saveCount int
	failSaves int
}

func (m *mockConflictRepo) Load(ctx context.Context) (*domain.State, error) {
	m.cMu.Lock()
	defer m.cMu.Unlock()
	return m.mockRepo.Load(ctx)
}

func (m *mockConflictRepo) Save(ctx context.Context, s *domain.State) error {
	m.cMu.Lock()
	defer m.cMu.Unlock()
	m.saveCount++
	if m.saveCount <= m.failSaves {
		return domain.ErrVersionConflict
	}
	return m.mockRepo.Save(ctx, s)
}

func TestOrchestrator_InstantWakeupOnTaskCompletion(t *testing.T) {
	state := &domain.State{
		ID:          "session-wakeup",
		ProjectPath: "/tmp",
	}
	repo := &mockRepo{state: state}
	reg := NewToolRegistry()
	llmClient := &mockLLM{}
	validator := NewPolicyValidator(nil, "main", nil)
	scheduler := NewScheduler(NewFileLockRegistry())
	git := NewGitClient("/tmp")
	queue := NewRebaseQueue(git)
	evaluator := NewTestValidator(nil, false, nil, nil)
	vcsClient := &mockVCS{}
	cfg := OrchestratorConfig{PollInterval: 5 * time.Minute}

	orch := NewOrchestrator(repo, reg, llmClient, validator, scheduler, git, queue, evaluator, vcsClient, cfg, nil, nil)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	start := time.Now()
	go func() {
		time.Sleep(50 * time.Millisecond)
		orch.taskCompletedChan <- struct{}{}
	}()

	err := SleepWithInterrupt(ctx, orch.cfg.PollInterval, orch.taskCompletedChan)
	elapsed := time.Since(start)

	assert.True(t, err == nil || strings.Contains(err.Error(), "interrupted") || strings.Contains(err.Error(), "context"))
	assert.Less(t, elapsed, 1*time.Second, "Main loop should wake up within milliseconds of task completion signal")
}

func TestUpdateStateWithRetry_JitterAndTargetedUpdate(t *testing.T) {
	state := &domain.State{
		ID:          "session-jitter",
		ProjectPath: "/tmp",
		BuildStatus: domain.BuildUnknown,
	}
	repo := &mockConflictRepo{mockRepo: mockRepo{state: state}, failSaves: 2}
	reg := NewToolRegistry()
	llmClient := &mockLLM{}
	validator := NewPolicyValidator(nil, "main", nil)
	scheduler := NewScheduler(NewFileLockRegistry())
	git := NewGitClient("/tmp")
	queue := NewRebaseQueue(git)
	evaluator := NewTestValidator(nil, false, nil, nil)
	vcsClient := &mockVCS{}
	cfg := OrchestratorConfig{
		OCCMaxRetries:    5,
		OCCBackoffBase:   10 * time.Millisecond,
		OCCBackoffFactor: 2.0,
	}

	orch := NewOrchestrator(repo, reg, llmClient, validator, scheduler, git, queue, evaluator, vcsClient, cfg, nil, nil)

	start := time.Now()
	err := orch.updateStateWithRetry(context.Background(), func(st *domain.State) error {
		st.BuildStatus = domain.BuildPassing
		return nil
	})
	elapsed := time.Since(start)

	assert.NoError(t, err)
	assert.Equal(t, domain.BuildPassing, repo.state.BuildStatus)
	assert.Equal(t, 3, repo.saveCount) // 2 conflicts, 1 success
	assert.Less(t, elapsed, 500*time.Millisecond, "OCC retry backoff with jitter should resolve collisions quickly")
}
