package services

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/diegojromerolopez/noctifab/pkg/domain"
	"github.com/google/uuid"
)

func (o *Orchestrator) prepareQA(ctx context.Context, state *domain.State, task domain.Task) (domain.StoryContract, *QAReviewResult) {
	if !o.cfg.QA.Enabled {
		result := o.skippedQA(task.ID, "disabled")
		return domain.StoryContract{}, &result
	}
	storyPath := state.Metadata.InputPath
	if !filepath.IsAbs(storyPath) {
		storyPath = filepath.Join(state.ProjectPath, storyPath)
	}
	markdown, err := os.ReadFile(storyPath)
	if err != nil {
		result := o.skippedQA(task.ID, "missing_story_contract")
		return domain.StoryContract{}, &result
	}
	contract, err := ParseStoryContract(relativeStoryPath(state.ProjectPath, storyPath), string(markdown))
	if err != nil || len(contract.PublicContracts) == 0 {
		result := o.skippedQA(task.ID, "missing_story_contract")
		return domain.StoryContract{}, &result
	}
	if !qaApplies(contract, task.TargetFiles, o.cfg.QA.ValidationCommands) {
		result := o.skippedQAWithStory(task.ID, contract.StoryID, "not_applicable")
		return contract, &result
	}
	if o.qa == nil || len(o.cfg.QA.BuildCommand) == 0 || len(o.cfg.QA.ValidationCommands) == 0 {
		result := o.skippedQAWithStory(task.ID, contract.StoryID, "validation_surface_unavailable")
		result.Phase.Status = domain.ReviewInconclusive
		return contract, &result
	}
	return contract, nil
}

func (o *Orchestrator) runInitialQA(ctx context.Context, state *domain.State, task domain.Task,
	taskGit *GitClient, sourceCommit string, contract domain.StoryContract, fileContexts []string,
) QAReviewResult {
	request := QAReviewRequest{
		State: state.Clone(), Task: task, Contract: contract, SourceCommit: sourceCommit,
		RepositoryPath: state.ProjectPath, Attempt: 1,
		VerifySource: storySHARecheck(state.ProjectPath, contract),
		Tester: func(testerCtx context.Context, workspace ReviewWorkspace) (string, error) {
			testerState := state.Clone()
			testerState.ProjectPath = workspace.Path
			o.RunTesterAgent(testerCtx, task, testerState, fileContexts, "write", "")
			git := NewGitClient(workspace.Path)
			paths, err := git.Run(testerCtx, false, "diff", "--name-only", sourceCommit)
			if err != nil {
				return "", err
			}
			if err := validateTesterPaths(paths, o.cfg.QA.TesterPathPrefixes); err != nil {
				return "", err
			}
			return git.Run(testerCtx, false, "diff", "--binary", sourceCommit)
		},
	}
	result, err := o.executeQAReview(ctx, contract, request)
	if err != nil {
		return result
	}
	if result.TesterPatch != "" && (result.Phase.Status == domain.ReviewPass || result.Phase.Status == domain.ReviewFindings) {
		if err := applyTesterPatch(ctx, taskGit, state.ProjectPath, result.TesterPatch); err != nil {
			result.Phase.Status, result.Phase.TerminalReason = domain.ReviewError, "tester_patch_failed"
			result.Findings = nil
			_ = o.persistQAResult(ctx, contract, result)
		}
	}
	return result
}

func (o *Orchestrator) rerunQAAfterFix(ctx context.Context, state *domain.State, task domain.Task,
	taskGit *GitClient, contract domain.StoryContract, fileContexts []string, findings []domain.QAFinding,
) string {
	maxRounds := o.cfg.QA.MaxReviewRounds
	for attempt := 2; attempt <= maxRounds+1; attempt++ {
		o.metrics().RecordQAFixRound()
		before, err := taskGit.Run(ctx, false, "rev-parse", "HEAD")
		if err != nil {
			return "artifact_changed"
		}
		o.RunGeneratorAgent(ctx, task, state, fileContexts, qaFindingsFeedback(findings), "fix")
		if err := commitTaskChanges(ctx, taskGit, fmt.Sprintf("fix(core): address QA findings for task %s", task.ID)); err != nil {
			return "artifact_changed"
		}
		commit, err := taskGit.Run(ctx, false, "rev-parse", "HEAD")
		if err != nil || strings.TrimSpace(commit) == strings.TrimSpace(before) {
			return "artifact_changed"
		}
		passed, _, _ := o.evaluator.ValidateTask(ctx, state, task)
		if !passed {
			return "validation_failed"
		}
		request := QAReviewRequest{
			State: state.Clone(), Task: task, Contract: contract, SourceCommit: strings.TrimSpace(commit),
			RepositoryPath: state.ProjectPath, Attempt: attempt, VerifySource: storySHARecheck(state.ProjectPath, contract),
		}
		result, reviewErr := o.executeQAReview(ctx, contract, request)
		if reviewErr != nil {
			return "persistence_failed"
		}
		if result.Phase.Status == domain.ReviewPass || result.Phase.Status == domain.ReviewSkipped {
			return ""
		}
		if result.Phase.Status != domain.ReviewFindings || attempt > maxRounds {
			return result.Phase.TerminalReason
		}
		findings = result.Findings
	}
	return "public_contract_failed"
}

func (o *Orchestrator) runQAGate(ctx context.Context, state *domain.State, task domain.Task,
	taskGit *GitClient, fileContexts []string,
) string {
	contract, precomputed := o.prepareQA(ctx, state, task)
	if precomputed != nil {
		if err := o.persistQAResult(ctx, contract, *precomputed); err != nil {
			return "persistence_failed"
		}
		if precomputed.Phase.Status == domain.ReviewSkipped {
			return ""
		}
		return precomputed.Phase.TerminalReason
	}
	commit, err := taskGit.Run(ctx, false, "rev-parse", "HEAD")
	if err != nil {
		return "artifact_changed"
	}
	clean, err := taskGit.Run(ctx, false, "status", "--porcelain")
	if err != nil || strings.TrimSpace(clean) != "" {
		return "artifact_changed"
	}
	result := o.runInitialQA(ctx, state, task, taskGit, strings.TrimSpace(commit), contract, fileContexts)
	if result.Phase.Status == domain.ReviewFindings {
		return o.rerunQAAfterFix(ctx, state, task, taskGit, contract, fileContexts, result.Findings)
	}
	if result.Phase.Status != domain.ReviewPass && result.Phase.Status != domain.ReviewSkipped {
		return result.Phase.TerminalReason
	}
	if result.TesterPatch != "" {
		if err := commitTaskChanges(ctx, taskGit, fmt.Sprintf("test(core): add QA acceptance tests for task %s", task.ID)); err != nil {
			return "tester_patch_failed"
		}
		passed, _, _ := o.evaluator.ValidateTask(ctx, state, task)
		if !passed {
			return "validation_failed"
		}
	}
	return ""
}

func (o *Orchestrator) executeQAReview(ctx context.Context, contract domain.StoryContract, request QAReviewRequest) (QAReviewResult, error) {
	working := o.qa.Begin(request)
	if err := o.persistQAWorking(ctx, contract, working.Phase); err != nil {
		working.Phase = terminalQAError(working.Phase, "persistence_failed")
		return working, err
	}
	session := o.qa.Review(ctx, request, working)
	result := sanitizeQAResult(session.Result, o.cfg.QA.MaxOutputBytes)
	if err := o.persistQAResult(ctx, contract, result); err != nil {
		return result, err
	}
	if err := session.Cleanup(context.WithoutCancel(ctx)); err != nil {
		result.Phase = terminalQAError(result.Phase, "isolation_failed")
		result.Findings = nil
		if persistErr := o.persistQAResult(ctx, contract, result); persistErr != nil {
			return result, errors.Join(err, persistErr)
		}
		// Cleanup ownership remains in the session after failure, so a retry can finish partial cleanup.
		_ = session.Cleanup(context.WithoutCancel(ctx))
	}
	return result, nil
}

func (o *Orchestrator) persistQAWorking(ctx context.Context, contract domain.StoryContract, phase domain.ReviewPhase) error {
	return o.updateStateWithRetry(ctx, func(state *domain.State) error {
		if contract.StoryID != "" {
			upsertStoryContract(state, contract)
		}
		state.ReviewPhases = upsertReviewPhase(state.ReviewPhases, phase)
		upsertWorkingQAAgent(state, phase)
		return nil
	})
}

func upsertWorkingQAAgent(state *domain.State, phase domain.ReviewPhase) {
	agent := domain.Agent{ID: phase.ID, Name: "qa", Role: domain.AgentRoleQA, Status: domain.AgentWorking,
		TaskID: phase.TaskID, StartedAt: phase.StartedAt}
	for i := range state.ActiveAgents {
		if state.ActiveAgents[i].ID == phase.ID {
			state.ActiveAgents[i] = agent
			return
		}
	}
	state.ActiveAgents = append(state.ActiveAgents, agent)
}

func terminalQAError(phase domain.ReviewPhase, reason string) domain.ReviewPhase {
	phase.Status = domain.ReviewInconclusive
	phase.TerminalReason = reason
	phase.CompletedAt = time.Now()
	return phase
}

func storySHARecheck(projectPath string, contract domain.StoryContract) func() error {
	return func() error {
		path := contract.SourcePath
		if !filepath.IsAbs(path) {
			path = filepath.Join(projectPath, filepath.FromSlash(path))
		}
		content, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		sum := sha256.Sum256(content)
		if hex.EncodeToString(sum[:]) != contract.SourceSHA256 {
			return errors.New("story source changed")
		}
		return nil
	}
}

func commitTaskChanges(ctx context.Context, git *GitClient, message string) error {
	if _, err := git.Run(ctx, true, "add", "--all", "--", ":!.noctifab"); err != nil {
		return err
	}
	staged, err := git.Run(ctx, false, "diff", "--cached", "--name-only")
	if err != nil {
		return err
	}
	if strings.TrimSpace(staged) == "" {
		return fmt.Errorf("no changes to commit")
	}
	_, err = git.Run(ctx, true, "commit", "-m", message)
	return err
}

func (o *Orchestrator) persistQAResult(ctx context.Context, contract domain.StoryContract, result QAReviewResult) error {
	err := o.updateStateWithRetry(ctx, func(state *domain.State) error {
		if contract.StoryID != "" {
			upsertStoryContract(state, contract)
		}
		result = sanitizeQAResult(result, o.cfg.QA.MaxOutputBytes)
		state.ReviewPhases = upsertReviewPhase(state.ReviewPhases, result.Phase)
		for _, scenario := range result.Scenarios {
			if !containsScenario(state.QAScenarios, scenario.ReviewPhaseID, scenario.Fingerprint) {
				state.QAScenarios = append(state.QAScenarios, scenario)
			}
		}
		for _, finding := range result.Findings {
			if !containsFinding(state.QAFindings, finding.ArtifactID, finding.ScenarioFingerprint) {
				state.QAFindings = append(state.QAFindings, finding)
			}
		}
		if result.Phase.TerminalReason != "disabled" {
			upsertCompletedQAAgent(state, result.Phase)
		}
		return nil
	})
	if err != nil {
		return err
	}
	o.recordQAResultMetrics(result)
	return nil
}

func (o *Orchestrator) recordQAResultMetrics(result QAReviewResult) {
	metrics := o.metrics()
	metrics.RecordQAPhase(result.Phase.CompletedAt.Sub(result.Phase.StartedAt), result.Phase.TokensUsed)
	if result.Phase.Status == domain.ReviewSkipped {
		metrics.RecordQASkipped(result.Phase.TerminalReason)
	}
	for range result.DuplicatesSuppressed {
		metrics.RecordQADuplicateSuppressed()
	}
	for _, finding := range result.Findings {
		metrics.RecordQARegression()
		metrics.RecordQAFindingDisposition(finding.Disposition)
	}
}

func upsertCompletedQAAgent(state *domain.State, phase domain.ReviewPhase) {
	agent := domain.Agent{
		ID: phase.ID, Name: "qa", Role: domain.AgentRoleQA, Status: domain.AgentCompleted,
		TaskID: phase.TaskID, StartedAt: phase.StartedAt, CompletedAt: phase.CompletedAt,
		TokensUsed: phase.TokensUsed,
	}
	if phase.Status != domain.ReviewPass && phase.Status != domain.ReviewSkipped {
		agent.LastError = phase.TerminalReason
	}
	for i := range state.ActiveAgents {
		if state.ActiveAgents[i].ID == agent.ID {
			state.ActiveAgents[i] = agent
			return
		}
	}
	state.ActiveAgents = append(state.ActiveAgents, agent)
}

func (o *Orchestrator) skippedQA(taskID, reason string) QAReviewResult {
	return o.skippedQAWithStory(taskID, "", reason)
}

func (o *Orchestrator) skippedQAWithStory(taskID, storyID, reason string) QAReviewResult {
	now := time.Now()
	return QAReviewResult{Phase: domain.ReviewPhase{
		ID: uuid.NewString(), StoryID: storyID, TaskID: taskID, Role: "qa", Attempt: 1,
		Status: domain.ReviewSkipped, TerminalReason: reason, StartedAt: now, CompletedAt: now,
	}}
}

func qaApplies(contract domain.StoryContract, targetFiles, validationCommands []string) bool {
	commands := make(map[string]struct{}, len(validationCommands))
	for _, command := range validationCommands {
		commands[command] = struct{}{}
	}
	for _, public := range contract.PublicContracts {
		commandMatches := false
		for _, executable := range public.AllowedExecutables {
			if _, ok := commands[executable]; ok {
				commandMatches = true
				break
			}
		}
		if !commandMatches {
			continue
		}
		if len(public.ApplicablePathPrefixes) == 0 {
			return true
		}
		for _, target := range targetFiles {
			clean := filepath.ToSlash(filepath.Clean(target))
			for _, prefix := range public.ApplicablePathPrefixes {
				if strings.HasPrefix(clean, strings.TrimPrefix(prefix, "./")) {
					return true
				}
			}
		}
	}
	return false
}

func validateTesterPaths(output string, prefixes []string) error {
	for _, path := range strings.Fields(output) {
		clean := filepath.ToSlash(filepath.Clean(path))
		allowed := false
		for _, prefix := range prefixes {
			if strings.HasPrefix(clean, strings.TrimPrefix(filepath.ToSlash(prefix), "./")) {
				allowed = true
				break
			}
		}
		if !allowed {
			return fmt.Errorf("tester patch path %q is outside configured prefixes", path)
		}
	}
	return nil
}

func applyTesterPatch(ctx context.Context, git *GitClient, projectPath, patch string) error {
	sum := sha256.Sum256([]byte(patch))
	path := filepath.Join(projectPath, ".noctifab", "data", "qa-test-"+hex.EncodeToString(sum[:8])+".patch")
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	if err := os.WriteFile(path, []byte(patch), 0o600); err != nil {
		return err
	}
	defer func() { _ = os.Remove(path) }()
	_, err := git.Run(ctx, true, "apply", "--index", "--binary", path)
	return err
}

func relativeStoryPath(projectPath, sourcePath string) string {
	relative, err := filepath.Rel(projectPath, sourcePath)
	if err == nil && relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return filepath.ToSlash(relative)
	}
	return filepath.Base(sourcePath)
}

func upsertStoryContract(state *domain.State, contract domain.StoryContract) {
	for i := range state.StoryContracts {
		if state.StoryContracts[i].StoryID == contract.StoryID {
			state.StoryContracts[i] = contract
			return
		}
	}
	state.StoryContracts = append(state.StoryContracts, contract)
}

func upsertReviewPhase(phases []domain.ReviewPhase, phase domain.ReviewPhase) []domain.ReviewPhase {
	for i := range phases {
		if phases[i].ID == phase.ID {
			phases[i] = phase
			return phases
		}
	}
	return append(phases, phase)
}

func containsScenario(scenarios []domain.QAScenario, phaseID, fingerprint string) bool {
	for _, scenario := range scenarios {
		if scenario.ReviewPhaseID == phaseID && scenario.Fingerprint == fingerprint {
			return true
		}
	}
	return false
}

func containsFinding(findings []domain.QAFinding, artifactID, fingerprint string) bool {
	for _, finding := range findings {
		if finding.ArtifactID == artifactID && finding.ScenarioFingerprint == fingerprint {
			return true
		}
	}
	return false
}

func qaFindingsFeedback(findings []domain.QAFinding) string {
	var feedback strings.Builder
	feedback.WriteString("\n\nQA public-contract findings. Fix only the current task, then rebuild and rerun validation:\n")
	for _, finding := range findings {
		_, _ = fmt.Fprintf(&feedback, "contract=%s fingerprint=%s artifact=%s expected=%s actual=%s evidence=%s\n",
			finding.PublicContractID, finding.ScenarioFingerprint, finding.ArtifactID,
			sanitizeQAText(finding.Expected, 4096), sanitizeQAText(finding.Actual, 4096), sanitizeQAText(finding.Evidence, 4096))
	}
	return sanitizeQAText(feedback.String(), 16384)
}

func sanitizeQAResult(result QAReviewResult, limit int) QAReviewResult {
	result.Phase.TerminalReason = sanitizeQAText(result.Phase.TerminalReason, 1024)
	result.TesterPatch = capText(result.TesterPatch, limit)
	for i := range result.Scenarios {
		result.Scenarios[i].Name = sanitizeQAText(result.Scenarios[i].Name, 1024)
		result.Scenarios[i].Evidence = sanitizeQAText(result.Scenarios[i].Evidence, limit)
	}
	for i := range result.Findings {
		finding := &result.Findings[i]
		finding.Expected = sanitizeQAText(finding.Expected, limit)
		finding.Actual = sanitizeQAText(finding.Actual, limit)
		finding.Evidence = sanitizeQAText(finding.Evidence, limit)
	}
	return result
}
