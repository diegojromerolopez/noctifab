package services

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/diegojromerolopez/noctifab/pkg/domain"
	"github.com/diegojromerolopez/noctifab/pkg/infrastructure/config"
	"github.com/diegojromerolopez/noctifab/pkg/infrastructure/prompts"
)

type qaLLMFake struct {
	response *domain.LLMResponse
	err      error
	calls    int
}

func (f *qaLLMFake) Complete(context.Context, string) (*domain.LLMResponse, error) {
	f.calls++
	return f.response, f.err
}

type qaWorkspaceFake struct {
	createErr    error
	cleanupErr   error
	createCalls  int
	cleanupCalls int
	events       *[]string
}

func (f *qaWorkspaceFake) Create(context.Context, string, string) (ReviewWorkspace, ReviewWorkspace, ReviewWorkspace, error) {
	f.createCalls++
	if f.events != nil {
		*f.events = append(*f.events, "external")
	}
	return ReviewWorkspace{Path: "/build"}, ReviewWorkspace{Path: "/tester"}, ReviewWorkspace{Path: "/qa"}, f.createErr
}

func (f *qaWorkspaceFake) Cleanup(context.Context, ...ReviewWorkspace) error {
	f.cleanupCalls++
	if f.events != nil {
		*f.events = append(*f.events, "cleanup")
	}
	err := f.cleanupErr
	f.cleanupErr = nil
	return err
}

type qaBuilderFake struct {
	verifyCalls int
	changeAt    int
	buildErr    error
}

func (f *qaBuilderFake) Build(context.Context, ReviewWorkspace, string, []string, []string, string, int) (QAArtifact, QACommandResult, error) {
	return QAArtifact{ID: "abc:manifest", Path: "/artifact", Manifest: []ArtifactManifestEntry{{Path: "dist/app"}}}, QACommandResult{}, f.buildErr
}

func (f *qaBuilderFake) Verify(QAArtifact) error {
	f.verifyCalls++
	if f.changeAt > 0 && f.verifyCalls >= f.changeAt {
		return errors.New("changed")
	}
	return nil
}

type qaSandboxFake struct {
	results []QACommandResult
	err     error
	runs    int
}

func (f *qaSandboxFake) Verify(context.Context, string, string, string) error { return nil }
func (f *qaSandboxFake) Run(context.Context, QACommand) (QACommandResult, error) {
	index := f.runs
	f.runs++
	if index >= len(f.results) {
		return QACommandResult{}, f.err
	}
	return f.results[index], f.err
}

type qaClockFake struct{ now time.Time }

func (f qaClockFake) Now() time.Time { return f.now }

func TestQARuntimeCoordinatorOutcomes(t *testing.T) {
	passing := QACommandResult{ExitCode: 0, Stdout: "ok"}
	failing := QACommandResult{ExitCode: 1, Stderr: "ERROR: bad"}
	timedOut := QACommandResult{TimedOut: true}
	tests := []struct {
		name       string
		result     QACommandResult
		wantStatus domain.ReviewStatus
		wantReason string
		findings   int
	}{
		{"pass", passing, domain.ReviewPass, "", 0},
		{"findings", failing, domain.ReviewFindings, "public_contract_failed", 1},
		{"inconclusive", timedOut, domain.ReviewInconclusive, "scenario_environment_failed", 0},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			llm := &qaLLMFake{response: scenarioResponse(false)}
			sandbox := &qaSandboxFake{results: []QACommandResult{test.result}}
			coordinator := newQATestCoordinator(llm, &qaWorkspaceFake{}, &qaBuilderFake{}, sandbox)
			request := qaRequest()
			result := coordinator.Review(context.Background(), request, coordinator.Begin(request)).Result
			if result.Phase.Status != test.wantStatus || result.Phase.TerminalReason != test.wantReason {
				t.Fatalf("got %s/%s, want %s/%s", result.Phase.Status, result.Phase.TerminalReason, test.wantStatus, test.wantReason)
			}
			if len(result.Findings) != test.findings {
				t.Fatalf("got %d findings, want %d", len(result.Findings), test.findings)
			}
		})
	}
}

func TestQARuntimeSuppressesDuplicatesAndDetectsArtifactChange(t *testing.T) {
	t.Run("duplicate executes once", func(t *testing.T) {
		llm := &qaLLMFake{response: scenarioResponse(true)}
		sandbox := &qaSandboxFake{results: []QACommandResult{{ExitCode: 0, Stdout: "ok"}}}
		coordinator := newQATestCoordinator(llm, &qaWorkspaceFake{}, &qaBuilderFake{}, sandbox)
		request := qaRequest()
		result := coordinator.Review(context.Background(), request, coordinator.Begin(request)).Result
		if result.Phase.Status != domain.ReviewPass || sandbox.runs != 1 || len(result.Scenarios) != 1 {
			t.Fatalf("status=%s runs=%d scenarios=%d", result.Phase.Status, sandbox.runs, len(result.Scenarios))
		}
	})
	t.Run("artifact changes before terminal save", func(t *testing.T) {
		builder := &qaBuilderFake{changeAt: 2}
		coordinator := newQATestCoordinator(&qaLLMFake{response: scenarioResponse(false)}, &qaWorkspaceFake{}, builder,
			&qaSandboxFake{results: []QACommandResult{{ExitCode: 0, Stdout: "ok"}}})
		request := qaRequest()
		result := coordinator.Review(context.Background(), request, coordinator.Begin(request)).Result
		if result.Phase.Status != domain.ReviewInconclusive || result.Phase.TerminalReason != "artifact_changed" {
			t.Fatalf("got %s/%s", result.Phase.Status, result.Phase.TerminalReason)
		}
	})
}

func TestQARuntimeCancellationCleansUp(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	workspace := &qaWorkspaceFake{}
	llm := &qaLLMFake{response: scenarioResponse(false)}
	coordinator := newQATestCoordinator(llm, workspace, &qaBuilderFake{}, &qaSandboxFake{})
	request := qaRequest()
	result := coordinator.Review(ctx, request, coordinator.Begin(request)).Result
	if result.Phase.Status != domain.ReviewInterrupted || llm.calls != 0 || workspace.createCalls != 0 {
		t.Fatalf("status=%s llm=%d creates=%d", result.Phase.Status, llm.calls, workspace.createCalls)
	}
}

func TestQARuntimeBudgetManifestPromptAndSourceChecks(t *testing.T) {
	t.Run("budget exhausted is terminal", func(t *testing.T) {
		llm := &qaLLMFake{err: domain.ErrBudgetExhausted}
		coordinator := newQATestCoordinator(llm, &qaWorkspaceFake{}, &qaBuilderFake{}, &qaSandboxFake{})
		request := qaRequest()
		result := coordinator.Review(context.Background(), request, coordinator.Begin(request)).Result
		if result.Phase.Status != domain.ReviewBudgetExhausted || result.Phase.TerminalReason != "budget_exhausted" {
			t.Fatalf("got %s/%s", result.Phase.Status, result.Phase.TerminalReason)
		}
	})
	t.Run("positive exact cost limit fails closed without usage", func(t *testing.T) {
		llm := &qaLLMFake{response: scenarioResponse(false)}
		coordinator := newQATestCoordinator(llm, &qaWorkspaceFake{}, &qaBuilderFake{}, &qaSandboxFake{})
		coordinator.cfg.MaxCostUSD = "0.01"
		request := qaRequest()
		result := coordinator.Review(context.Background(), request, coordinator.Begin(request)).Result
		if result.Phase.Status != domain.ReviewBudgetExhausted || llm.calls != 0 {
			t.Fatalf("status=%s calls=%d", result.Phase.Status, llm.calls)
		}
	})
	t.Run("manifest persists and full state is omitted", func(t *testing.T) {
		llm := &qaLLMFake{response: scenarioResponse(false)}
		coordinator := newQATestCoordinator(llm, &qaWorkspaceFake{}, &qaBuilderFake{},
			&qaSandboxFake{results: []QACommandResult{{ExitCode: 0, Stdout: "ok"}}})
		request := qaRequest()
		request.State.Files = []domain.FileInfo{{Path: "secret-full-state-marker"}}
		result := coordinator.Review(context.Background(), request, coordinator.Begin(request)).Result
		if len(result.Phase.ArtifactManifest) != 1 || result.Phase.ArtifactManifest[0].Path != "dist/app" {
			t.Fatalf("manifest not persisted: %+v", result.Phase.ArtifactManifest)
		}
	})
	t.Run("source changes before external work", func(t *testing.T) {
		workspace := &qaWorkspaceFake{}
		coordinator := newQATestCoordinator(&qaLLMFake{}, workspace, &qaBuilderFake{}, &qaSandboxFake{})
		request := qaRequest()
		request.VerifySource = func() error { return errors.New("changed") }
		result := coordinator.Review(context.Background(), request, coordinator.Begin(request)).Result
		if result.Phase.TerminalReason != "story_changed" || workspace.createCalls != 0 {
			t.Fatalf("result=%+v createCalls=%d", result.Phase, workspace.createCalls)
		}
	})
}

func newQATestCoordinator(llm domain.LLMClient, workspace ReviewWorkspaceFactory, builder QAArtifactBuilder, sandbox QASandboxRunner) *QARuntimeCoordinator {
	cfg := config.DefaultConfig().Agents.QA
	cfg.Enabled = true
	cfg.ValidationCommands = []string{"./dist/app"}
	cfg.BuildCommand = []string{"make", "build"}
	return NewQARuntimeCoordinator(cfg, llm, prompts.NewDefaultRenderer(), workspace, builder, sandbox,
		OSQAFileSystem{}, qaClockFake{now: time.Unix(100, 0)})
}

func qaRequest() QAReviewRequest {
	contract := domain.StoryContract{StoryID: "US-1", PublicContracts: []domain.PublicContract{{
		ID: "cli.run", AllowedExecutables: []string{"./dist/app"}, ExitCodes: []int{0},
		StdoutContains: []string{"ok"}, StderrPrefixes: []string{"ERROR:"},
	}}}
	return QAReviewRequest{State: &domain.State{ID: "state"}, Task: domain.Task{ID: "task"},
		Contract: contract, SourceCommit: "abc", RepositoryPath: "/repo", Attempt: 1}
}

func scenarioResponse(duplicate bool) *domain.LLMResponse {
	exit := 0
	scenario := map[string]any{"name": "valid", "public_contract_id": "cli.run", "steps": []any{
		map[string]any{"command": []string{"./dist/app"}, "expected_exit_code": exit, "stdout_contains": []string{"ok"}},
	}}
	scenarios := []any{scenario}
	if duplicate {
		copy := map[string]any{"name": "same behavior", "public_contract_id": "cli.run", "steps": scenario["steps"]}
		scenarios = append(scenarios, copy)
	}
	return &domain.LLMResponse{Actions: []domain.LLMAction{{Tool: "propose_scenarios", Args: map[string]any{"scenarios": scenarios}}}}
}

type occRepo struct {
	mu        sync.Mutex
	state     *domain.State
	conflicts int
}

func (r *occRepo) Load(context.Context) (*domain.State, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.state.Clone(), nil
}

func (r *occRepo) LoadByID(ctx context.Context, _ string) (*domain.State, error) { return r.Load(ctx) }
func (r *occRepo) LoadAll(ctx context.Context) ([]*domain.State, error) {
	state, err := r.Load(ctx)
	return []*domain.State{state}, err
}
func (r *occRepo) LoadAllSummaries(context.Context) ([]domain.StateSummary, error) { return nil, nil }
func (r *occRepo) PruneFinishedStates(context.Context, int) (int, error)           { return 0, nil }

func (r *occRepo) Save(_ context.Context, state *domain.State) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.conflicts > 0 {
		r.conflicts--
		return domain.ErrVersionConflict
	}
	r.state = state.Clone()
	return nil
}

func TestPersistQAResultRetriesOCCWithoutDuplicates(t *testing.T) {
	repo := &occRepo{state: &domain.State{ID: "state"}, conflicts: 1}
	o := &Orchestrator{repo: repo, cfg: OrchestratorConfig{OCCMaxRetries: 2, OCCBackoffBase: time.Nanosecond}}
	result := QAReviewResult{Phase: domain.ReviewPhase{ID: "phase", TaskID: "task", Role: "qa", Status: domain.ReviewPass},
		Scenarios: []domain.QAScenario{{ID: "scenario", ReviewPhaseID: "phase", Fingerprint: "fp"}},
		Findings:  []domain.QAFinding{{ID: "finding", ArtifactID: "artifact", ScenarioFingerprint: "fp"}}}
	if err := o.persistQAResult(context.Background(), domain.StoryContract{StoryID: "US-1"}, result); err != nil {
		t.Fatal(err)
	}
	if len(repo.state.ReviewPhases) != 1 || len(repo.state.QAScenarios) != 1 || len(repo.state.QAFindings) != 1 {
		t.Fatalf("duplicate records after OCC retry: %+v", repo.state)
	}
}

func TestExecuteQAReviewPersistsLifecycleBeforeCleanup(t *testing.T) {
	events := []string{}
	repo := &eventRepo{state: &domain.State{ID: "state"}, events: &events}
	workspace := &qaWorkspaceFake{events: &events}
	llm := &qaLLMFake{response: scenarioResponse(false)}
	coordinator := newQATestCoordinator(llm, workspace, &qaBuilderFake{},
		&qaSandboxFake{results: []QACommandResult{{ExitCode: 0, Stdout: "ok"}}})
	o := &Orchestrator{repo: repo, qa: coordinator, cfg: OrchestratorConfig{
		OCCMaxRetries: 1, QA: coordinator.cfg, MetricsEnabled: false,
	}, metricsCollector: NewMetricsCollector(false)}
	result, err := o.executeQAReview(context.Background(), domain.StoryContract{StoryID: "US-1"}, qaRequest())
	if err != nil || result.Phase.Status != domain.ReviewPass {
		t.Fatalf("result=%+v err=%v", result.Phase, err)
	}
	want := []string{"working", "external", "terminal", "cleanup"}
	if len(events) != len(want) {
		t.Fatalf("events=%v", events)
	}
	for i := range want {
		if events[i] != want[i] {
			t.Fatalf("events=%v want=%v", events, want)
		}
	}
	if repo.state.ActiveAgents[0].Status != domain.AgentCompleted {
		t.Fatalf("agent was not completed: %+v", repo.state.ActiveAgents)
	}
}

func TestExecuteQAReviewPersistsCleanupFailureAndRetries(t *testing.T) {
	repo := &occRepo{state: &domain.State{ID: "state"}}
	workspace := &qaWorkspaceFake{cleanupErr: errors.New("busy")}
	coordinator := newQATestCoordinator(&qaLLMFake{response: scenarioResponse(false)}, workspace, &qaBuilderFake{},
		&qaSandboxFake{results: []QACommandResult{{ExitCode: 0, Stdout: "ok"}}})
	o := &Orchestrator{repo: repo, qa: coordinator, cfg: OrchestratorConfig{OCCMaxRetries: 1, QA: coordinator.cfg},
		metricsCollector: NewMetricsCollector(false)}
	result, err := o.executeQAReview(context.Background(), domain.StoryContract{StoryID: "US-1"}, qaRequest())
	if err != nil || result.Phase.Status != domain.ReviewInconclusive || result.Phase.TerminalReason != "isolation_failed" {
		t.Fatalf("result=%+v err=%v", result.Phase, err)
	}
	if workspace.cleanupCalls != 2 || repo.state.ReviewPhases[0].TerminalReason != "isolation_failed" {
		t.Fatalf("cleanup=%d persisted=%+v", workspace.cleanupCalls, repo.state.ReviewPhases)
	}
}

type eventRepo struct {
	state  *domain.State
	events *[]string
}

func (r *eventRepo) Load(context.Context) (*domain.State, error) { return r.state.Clone(), nil }
func (r *eventRepo) LoadByID(ctx context.Context, _ string) (*domain.State, error) {
	return r.Load(ctx)
}
func (r *eventRepo) LoadAll(ctx context.Context) ([]*domain.State, error) {
	state, err := r.Load(ctx)
	return []*domain.State{state}, err
}
func (r *eventRepo) LoadAllSummaries(context.Context) ([]domain.StateSummary, error) { return nil, nil }
func (r *eventRepo) PruneFinishedStates(context.Context, int) (int, error)           { return 0, nil }
func (r *eventRepo) Save(_ context.Context, state *domain.State) error {
	if state.ReviewPhases[0].Status == domain.ReviewWorking {
		*r.events = append(*r.events, "working")
	} else {
		*r.events = append(*r.events, "terminal")
	}
	r.state = state.Clone()
	return nil
}

func TestQADisabledAndApplicability(t *testing.T) {
	llm := &qaLLMFake{}
	o := &Orchestrator{cfg: OrchestratorConfig{QA: config.DefaultConfig().Agents.QA}, llmClient: llm}
	_, result := o.prepareQA(context.Background(), &domain.State{}, domain.Task{ID: "task"})
	if result == nil || result.Phase.Status != domain.ReviewSkipped || result.Phase.TerminalReason != "disabled" || llm.calls != 0 {
		t.Fatalf("unexpected disabled result: %+v calls=%d", result, llm.calls)
	}
	contract := domain.StoryContract{PublicContracts: []domain.PublicContract{{
		AllowedExecutables: []string{"./app"}, ApplicablePathPrefixes: []string{"cmd/"},
	}}}
	if qaApplies(contract, []string{"docs/readme.md"}, []string{"./app"}) {
		t.Fatal("non-applicable path was accepted")
	}
	if !qaApplies(contract, []string{"cmd/main.go"}, []string{"./app"}) {
		t.Fatal("applicable path was rejected")
	}
	if err := validateTesterPaths("tests/a.go\npkg/a.go", []string{"tests/"}); err == nil {
		t.Fatal("production path in tester patch was accepted")
	}
}
