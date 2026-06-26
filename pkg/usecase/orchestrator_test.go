package usecase

import (
	"context"
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
	validator := NewPolicyValidator(nil, "main")
	scheduler := NewScheduler(NewFileLockRegistry())
	git := NewGitClient("/tmp")
	queue := NewRebaseQueue(git)
	evaluator := NewTestValidator(NewHostSandbox(nil, ""), false)
	vcsClient := &mockVCS{}

	cfg := OrchestratorConfig{
		PollInterval: 10 * time.Millisecond,
		Concurrency:  1,
	}

	orch := NewOrchestrator(repo, reg, llmClient, validator, scheduler, git, queue, evaluator, vcsClient, cfg)
	if orch == nil {
		t.Fatal("expected orchestrator instance, got nil")
	}

	if orch.vcsClient != vcsClient {
		t.Error("vcs client not wired correctly")
	}
}
