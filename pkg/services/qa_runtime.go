package services

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/diegojromerolopez/noctifab/pkg/domain"
	"github.com/diegojromerolopez/noctifab/pkg/infrastructure/config"
	"github.com/diegojromerolopez/noctifab/pkg/infrastructure/prompts"
	"github.com/google/uuid"
)

// QAReviewRequest is one immutable acceptance review request.
type QAReviewRequest struct {
	State          *domain.State
	Task           domain.Task
	Contract       domain.StoryContract
	SourceCommit   string
	RepositoryPath string
	Attempt        int
	Tester         func(context.Context, ReviewWorkspace) (string, error)
	VerifySource   func() error
}

// QAReviewResult contains all records that must be persisted atomically.
type QAReviewResult struct {
	Phase                domain.ReviewPhase
	Scenarios            []domain.QAScenario
	Findings             []domain.QAFinding
	TesterPatch          string
	DuplicatesSuppressed int
}

// QARuntimeCoordinator builds an immutable artifact and executes model-proposed scenarios.
type QARuntimeCoordinator struct {
	cfg       config.QAConfig
	llm       domain.LLMClient
	renderer  PromptRenderer
	workspace ReviewWorkspaceFactory
	builder   QAArtifactBuilder
	sandbox   QASandboxRunner
	fs        QAFileSystem
	clock     QAClock
}

func NewQARuntimeCoordinator(cfg config.QAConfig, llm domain.LLMClient, renderer PromptRenderer,
	workspace ReviewWorkspaceFactory, builder QAArtifactBuilder, sandbox QASandboxRunner,
	fsys QAFileSystem, clock QAClock,
) *QARuntimeCoordinator {
	return &QARuntimeCoordinator{cfg: cfg, llm: llm, renderer: renderer, workspace: workspace,
		builder: builder, sandbox: sandbox, fs: fsys, clock: clock}
}

// Begin creates the lifecycle records callers must persist before external work.
func (q *QARuntimeCoordinator) Begin(request QAReviewRequest) QAReviewResult {
	now := time.Now()
	if q.clock != nil {
		now = q.clock.Now()
	}
	return QAReviewResult{Phase: domain.ReviewPhase{
		ID: uuid.NewString(), StoryID: request.Contract.StoryID, TaskID: request.Task.ID,
		Role: "qa", Attempt: request.Attempt, Status: domain.ReviewWorking,
		StartedAt: now, DeadlineAt: now.Add(time.Duration(q.cfg.MaxDuration)),
	}}
}

// Review performs external work after the caller has persisted the WORKING lifecycle.
func (q *QARuntimeCoordinator) Review(ctx context.Context, request QAReviewRequest, result QAReviewResult) QAReviewSession {
	session := QAReviewSession{Result: result, factory: q.workspace}
	if err := ctx.Err(); err != nil {
		session.Result = q.finish(result, domain.ReviewInterrupted, "context_cancelled")
		return session
	}
	if q.workspace == nil || q.builder == nil || q.sandbox == nil || q.fs == nil || q.llm == nil || q.renderer == nil {
		session.Result = q.finish(result, domain.ReviewInconclusive, "isolation_failed")
		return session
	}
	if request.VerifySource != nil && request.VerifySource() != nil {
		session.Result = q.finish(result, domain.ReviewInconclusive, "story_changed")
		return session
	}

	phaseCtx, cancel := context.WithTimeout(ctx, time.Duration(q.cfg.MaxDuration))
	defer cancel()
	build, tester, qa, err := q.workspace.Create(phaseCtx, request.RepositoryPath, request.SourceCommit)
	session.workspaces = nonemptyReviewWorkspaces(build, tester, qa)
	if err != nil {
		session.Result = q.finish(result, statusForContext(phaseCtx, domain.ReviewInconclusive), reasonForContext(phaseCtx, "isolation_failed"))
		return session
	}
	root := filepath.Dir(build.Path)
	artifactPath := filepath.Join(root, "runtime-artifact")
	runtimePath := filepath.Join(root, "qa-runtime")
	var testerPatch string
	var testerErr error
	var testerWG sync.WaitGroup
	if request.Tester != nil {
		testerWG.Add(1)
		go func() {
			defer testerWG.Done()
			testerPatch, testerErr = request.Tester(phaseCtx, tester)
		}()
	}

	artifact, _, buildErr := q.builder.Build(phaseCtx, build, request.SourceCommit, q.cfg.BuildCommand,
		q.cfg.ValidationCommands, artifactPath, q.cfg.MaxOutputBytes)
	if buildErr != nil {
		cancel()
		testerWG.Wait()
		result.TesterPatch = testerPatch
		session.Result = q.finish(result, statusForContext(ctx, domain.ReviewInconclusive), reasonForContext(ctx, "validation_surface_unavailable"))
		return q.verifyTerminalSource(request, session)
	}
	result.Phase.ArtifactID = artifact.ID
	result.Phase.ArtifactManifest = artifactDomainManifest(artifact.Manifest)
	if err := q.sandbox.Verify(phaseCtx, qa.Path, artifact.Path, runtimePath); err != nil {
		cancel()
		testerWG.Wait()
		session.Result = q.finish(result, statusForContext(ctx, domain.ReviewInconclusive), reasonForContext(ctx, "isolation_failed"))
		return q.verifyTerminalSource(request, session)
	}

	review := q.deriveAndExecute(phaseCtx, request, result, artifact)
	testerWG.Wait()
	review.TesterPatch = testerPatch
	if testerErr != nil && review.Phase.Status != domain.ReviewInterrupted {
		session.Result = q.finish(review, domain.ReviewError, "tester_failed")
		return q.verifyTerminalSource(request, session)
	}
	session.Result = review
	return q.verifyTerminalSource(request, session)
}

func nonemptyReviewWorkspaces(workspaces ...ReviewWorkspace) []ReviewWorkspace {
	result := make([]ReviewWorkspace, 0, len(workspaces))
	for _, workspace := range workspaces {
		if workspace.Path != "" {
			result = append(result, workspace)
		}
	}
	return result
}

func (q *QARuntimeCoordinator) verifyTerminalSource(request QAReviewRequest, session QAReviewSession) QAReviewSession {
	if request.VerifySource != nil && request.VerifySource() != nil {
		session.Result = q.finish(session.Result, domain.ReviewInconclusive, "story_changed")
		session.Result.Findings = nil
	}
	return session
}

func (q *QARuntimeCoordinator) deriveAndExecute(ctx context.Context, request QAReviewRequest,
	result QAReviewResult, artifact QAArtifact,
) QAReviewResult {
	stateJSON, _ := json.Marshal(minimalQAState(request.State, request.Task))
	contractJSON, _ := json.Marshal(sanitizeStoryContract(request.Contract, q.cfg.MaxOutputBytes))
	rendered, err := q.renderer.Render(prompts.AgentQA, "acceptance", prompts.QAPromptData{
		State: string(stateJSON), StoryContract: string(contractJSON),
		ValidationCommands: q.cfg.ValidationCommands, MaxScenarios: q.cfg.MaxScenarios,
	})
	if err != nil {
		return q.finish(result, domain.ReviewError, "invalid_model_output")
	}
	var scenarios []domain.QAScenario
	for turn := 0; turn < q.cfg.Iterations; turn++ {
		response, completeErr := q.llm.Complete(context.WithValue(ctx, AgentRoleKey, "qa"), rendered.Full())
		if completeErr != nil {
			if errors.Is(completeErr, domain.ErrBudgetExhausted) {
				return q.finish(result, domain.ReviewBudgetExhausted, "budget_exhausted")
			}
			if ctx.Err() != nil {
				return q.finish(result, domain.ReviewInterrupted, "context_cancelled")
			}
			continue
		}
		var duplicates int
		scenarios, duplicates, err = ParseQAScenarioProposal(response, request.Contract, q.cfg.MaxScenarios)
		result.DuplicatesSuppressed = duplicates
		if err == nil {
			break
		}
	}
	if err != nil || len(scenarios) == 0 {
		return q.finish(result, domain.ReviewError, "invalid_model_output")
	}
	for i := range scenarios {
		scenarios[i].ID = uuid.NewString()
		scenarios[i].ReviewPhaseID = result.Phase.ID
		status, finding, evidence := q.executeScenario(ctx, request.Task.ID, result.Phase, artifact, scenarios[i])
		scenarios[i].Status, scenarios[i].Evidence = status, evidence
		result.Scenarios = append(result.Scenarios, scenarios[i])
		if status == domain.ReviewInconclusive {
			return q.finish(result, status, "scenario_environment_failed")
		}
		if finding != nil {
			result.Findings = append(result.Findings, *finding)
		}
	}
	if err := q.builder.Verify(artifact); err != nil {
		result.Findings = nil
		return q.finish(result, domain.ReviewInconclusive, "artifact_changed")
	}
	if len(result.Findings) > 0 {
		return q.finish(result, domain.ReviewFindings, "public_contract_failed")
	}
	return q.finish(result, domain.ReviewPass, "")
}

func (q *QARuntimeCoordinator) executeScenario(ctx context.Context, taskID string, phase domain.ReviewPhase,
	artifact QAArtifact, scenario domain.QAScenario,
) (domain.ReviewStatus, *domain.QAFinding, string) {
	for _, step := range scenario.Steps {
		if err := q.builder.Verify(artifact); err != nil {
			return domain.ReviewInconclusive, nil, "artifact changed"
		}
		command := QACommand{Argv: step.Command, Stdin: step.Stdin,
			Timeout: time.Duration(q.cfg.MaxDuration), OutputLimit: q.cfg.MaxOutputBytes}
		observed, err := q.sandbox.Run(ctx, command)
		if err != nil || observed.TimedOut || observed.Truncated {
			return domain.ReviewInconclusive, nil, sanitizeQAText("scenario environment failed: "+errorText(err), q.cfg.MaxOutputBytes)
		}
		expected, actual, passed := compareQAStep(step, observed)
		evidence := sanitizeQAText(fmt.Sprintf("artifact=%s command=%q exit=%d stdout=%q stderr=%q",
			artifact.ID, step.Command, observed.ExitCode, observed.Stdout, observed.Stderr), q.cfg.MaxOutputBytes)
		if !passed {
			return domain.ReviewFindings, &domain.QAFinding{
				ID: uuid.NewString(), ReviewPhaseID: phase.ID, TaskID: taskID, ArtifactID: artifact.ID,
				ScenarioFingerprint: scenario.Fingerprint, PublicContractID: scenario.PublicContractID,
				Severity: "blocking", Expected: expected, Actual: actual, Evidence: evidence, Disposition: "OPEN",
			}, evidence
		}
	}
	return domain.ReviewPass, nil, "all expectations satisfied"
}

func compareQAStep(step domain.QAStep, result QACommandResult) (string, string, bool) {
	expected := fmt.Sprintf("exit=%d stdout_contains=%q stderr_prefix=%q", step.ExpectedExitCode, step.StdoutContains, step.StderrPrefix)
	actual := fmt.Sprintf("exit=%d stdout=%q stderr=%q", result.ExitCode, result.Stdout, result.Stderr)
	if result.ExitCode != step.ExpectedExitCode || (step.StderrPrefix != "" && !strings.HasPrefix(result.Stderr, step.StderrPrefix)) {
		return expected, actual, false
	}
	for _, value := range step.StdoutContains {
		if !strings.Contains(result.Stdout, value) {
			return expected, actual, false
		}
	}
	return expected, actual, true
}

func (q *QARuntimeCoordinator) finish(result QAReviewResult, status domain.ReviewStatus, reason string) QAReviewResult {
	now := time.Now()
	if q.clock != nil {
		now = q.clock.Now()
	}
	result.Phase.Status, result.Phase.TerminalReason, result.Phase.CompletedAt = status, reason, now
	return result
}

func statusForContext(ctx context.Context, fallback domain.ReviewStatus) domain.ReviewStatus {
	if ctx.Err() != nil {
		return domain.ReviewInterrupted
	}
	return fallback
}

func reasonForContext(ctx context.Context, fallback string) string {
	if ctx.Err() != nil {
		return "context_cancelled"
	}
	return fallback
}

func errorText(err error) string {
	if err == nil {
		return "unknown"
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return "cancelled"
	}
	return SanitizeLog(err.Error())
}

func artifactDomainManifest(entries []ArtifactManifestEntry) []domain.ArtifactManifestEntry {
	manifest := make([]domain.ArtifactManifestEntry, len(entries))
	for i, entry := range entries {
		manifest[i] = domain.ArtifactManifestEntry{Path: entry.Path, SHA256: entry.SHA256}
	}
	return manifest
}

func minimalQAState(state *domain.State, task domain.Task) any {
	type snapshot struct {
		StateID string   `json:"state_id"`
		TaskID  string   `json:"task_id"`
		Title   string   `json:"title"`
		Targets []string `json:"target_files,omitempty"`
	}
	stateID := ""
	if state != nil {
		stateID = state.ID
	}
	return snapshot{StateID: sanitizeQAText(stateID, 256), TaskID: sanitizeQAText(task.ID, 256),
		Title: sanitizeQAText(task.Title, 2048), Targets: sanitizeQAStrings(task.TargetFiles, 256)}
}

func sanitizeStoryContract(contract domain.StoryContract, limit int) domain.StoryContract {
	contract.StoryID = sanitizeQAText(contract.StoryID, 256)
	contract.SourcePath = sanitizeQAText(contract.SourcePath, 1024)
	for i := range contract.PublicContracts {
		item := &contract.PublicContracts[i]
		item.ID = sanitizeQAText(item.ID, 256)
		item.Interface = sanitizeQAText(item.Interface, limit)
		item.ApplicablePathPrefixes = sanitizeQAStrings(item.ApplicablePathPrefixes, 1024)
		item.AllowedExecutables = sanitizeQAStrings(item.AllowedExecutables, 1024)
		item.StdoutContains = sanitizeQAStrings(item.StdoutContains, limit)
		item.StderrPrefixes = sanitizeQAStrings(item.StderrPrefixes, limit)
	}
	return contract
}

func sanitizeQAStrings(values []string, limit int) []string {
	result := make([]string, len(values))
	for i, value := range values {
		result[i] = sanitizeQAText(value, limit)
	}
	return result
}

func sanitizeQAText(value string, limit int) string {
	return capText(SanitizeLog(value), limit)
}
