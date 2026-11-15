package cli

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/diegojromerolopez/noctifab/pkg/domain"
	"github.com/diegojromerolopez/noctifab/pkg/infrastructure/config"
	"github.com/diegojromerolopez/noctifab/pkg/infrastructure/telemetry"
	"github.com/diegojromerolopez/noctifab/pkg/services"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

type storyExecutorDeps struct {
	cfg               *config.Config
	targetDir         string
	storyFiles        []string
	gitClient         *services.GitClient
	rebaseQueue       *services.RebaseQueue
	repo              domain.StateRepository
	reg               *services.ToolRegistry
	llmClient         domain.LLMClient
	validator         services.Validator
	scheduler         *services.Scheduler
	evaluator         *services.TestValidator
	vcsClient         domain.VCSClient
	orchConfig        services.OrchestratorConfig
	mailbox           *services.CommandMailbox
	repairHandler     services.RepairHandler
	promptRenderer    services.PromptRenderer
	executionReporter domain.ExecutionReporter
}

func buildStoryExecutor(deps storyExecutorDeps) func(ctx context.Context, currentStoryFile string) error {
	return func(ctx context.Context, currentStoryFile string) error {
		specBytes, err := os.ReadFile(currentStoryFile)
		if err != nil {
			return err
		}
		featName := filepath.Base(currentStoryFile)
		storyID := services.ExtractStoryID(currentStoryFile)
		if storyID == "" {
			storyID = featName
		}

		ctx, span := telemetry.Tracer().Start(ctx, "executeStory",
			trace.WithAttributes(
				attribute.String("story.id", storyID),
				attribute.String("story.file", currentStoryFile),
			))
		defer span.End()
		configuredBranch := deps.cfg.VCS.GetIntegrationBranch()
		if strings.ToLower(deps.cfg.VCS.BranchStrategy) == "per_story_isolated" {
			configuredBranch = ""
		} else if configuredBranch == "" {
			prefix := deps.cfg.VCS.BranchPrefix
			if prefix == "" {
				prefix = "noctifab/"
			}
			configuredBranch = prefix + "implementation"
		}
		branchRes := services.ResolveBranches(ctx, deps.gitClient, deps.cfg.VCS.BaseBranch, configuredBranch, deps.cfg.VCS.BranchPrefix, featName)
		baseBranch := branchRes.BaseBranch
		integrationBranch := branchRes.IntegrationBranch

		var state *domain.State
		const maxOCCRetries = 10
		backoff := 50 * time.Millisecond
		for attempt := 0; attempt < maxOCCRetries; attempt++ {
			s, err := deps.repo.Load(ctx)
			if err != nil {
				if errors.Is(err, sql.ErrNoRows) {
					s = &domain.State{
						ID:          uuid.New().String(),
						ProjectPath: deps.targetDir,
						Version:     0,
						BuildStatus: domain.BuildUnknown,
						Metadata: domain.StateMetadata{
							InputSource:       "markdown",
							InputPath:         currentStoryFile,
							FeatureName:       featName,
							BaseBranch:        baseBranch,
							IntegrationBranch: integrationBranch,
						},
					}
				} else {
					return err
				}
			}
			s.Metadata.InputPath = currentStoryFile
			s.Metadata.FeatureName = featName
			s.Metadata.BaseBranch = baseBranch
			s.Metadata.IntegrationBranch = integrationBranch
			s.StoryStatus = domain.StoryRunning

			now := time.Now().UTC()
			foundStory := false
			for i, st := range s.Stories {
				if st.ID == featName || st.FilePath == currentStoryFile {
					s.Stories[i].Status = domain.StoryRunning
					if s.Stories[i].StartedAt == nil {
						s.Stories[i].StartedAt = &now
					}
					s.Stories[i].UpdatedAt = now
					foundStory = true
					break
				}
			}
			if !foundStory {
				s.Stories = append(s.Stories, domain.Story{
					ID:        featName,
					StateID:   s.ID,
					Title:     featName,
					FilePath:  currentStoryFile,
					Status:    domain.StoryRunning,
					StartedAt: &now,
					CreatedAt: now,
					UpdatedAt: now,
				})
			}

			storyPath := currentStoryFile
			if !filepath.IsAbs(storyPath) {
				storyPath = filepath.Join(deps.targetDir, storyPath)
			}
			if markdown, err := os.ReadFile(storyPath); err == nil {
				relPath := currentStoryFile
				if filepath.IsAbs(relPath) {
					if rel, err := filepath.Rel(deps.targetDir, relPath); err == nil {
						relPath = rel
					}
				}
				if contract, err := services.ParseStoryContract(relPath, string(markdown)); err == nil && contract.StoryID != "" {
					services.UpsertStoryContract(s, contract)
				}
			}
			if err := deps.repo.Save(ctx, s); err != nil {
				if errors.Is(err, domain.ErrVersionConflict) && attempt < maxOCCRetries-1 {
					jitter := time.Duration(float64(backoff) * (0.8 + 0.4*rand.Float64()))
					select {
					case <-ctx.Done():
						return ctx.Err()
					case <-time.After(jitter):
					}
					backoff *= 2
					if backoff > 2*time.Second {
						backoff = 2 * time.Second
					}
					continue
				}
				return fmt.Errorf("failed to save initial state: %w", err)
			}
			state = s
			break
		}

		var qaCoord *services.QARuntimeCoordinator
		if qaDeps := qaDependencies(deps.cfg); len(qaDeps) > 0 {
			d := qaDeps[0]
			qaCoord = services.NewQARuntimeCoordinator(
				deps.cfg.Agents.QA, deps.llmClient, deps.promptRenderer,
				d.WorkspaceFactory, d.ArtifactBuilder, d.Sandbox, d.FileSystem, d.Clock,
			)
		}

		orchRuntime := services.OrchestratorRuntimeDependencies{
			Mailbox:        deps.mailbox,
			WatchdogRepair: deps.repairHandler,
			PromptRenderer: deps.promptRenderer,
			QA:             qaCoord,
			Observer:       deps.executionReporter,
		}
		orchestrator := services.NewOrchestratorWithRuntime(
			deps.repo, deps.reg, deps.llmClient, deps.validator, deps.scheduler,
			deps.gitClient, deps.rebaseQueue, deps.evaluator, deps.vcsClient, deps.orchConfig, orchRuntime,
		)

		fallbackCfg := deps.cfg.GetFallback()
		if fallbackCfg.Enabled {
			fallbackAgent := services.NewFallbackAgent(
				deps.repo,
				deps.llmClient,
				deps.mailbox,
				time.Duration(fallbackCfg.PollInterval),
				fallbackCfg.MaxRetries,
				time.Duration(fallbackCfg.StallThreshold),
				time.Duration(fallbackCfg.ConflictThreshold),
				fallbackCfg.LLMAssessment,
			)
			fallbackAgent.SetBudgetCliff(fallbackCfg.BudgetCliffRatio, 0)
			fallbackAgent.SetStallCountThreshold(fallbackCfg.Triggers.StallCountThreshold)
			orchestrator.SetFallbackAgent(fallbackAgent)
			fallbackCtx, cancelFallback := context.WithCancel(ctx)
			defer cancelFallback()
			fallbackAgent.Start(fallbackCtx)
		}

		if err := orchestrator.PlanStory(ctx, state, string(specBytes)); err != nil {
			return err
		}

		if stLoaded, loadErr := deps.repo.Load(ctx); loadErr == nil && stLoaded != nil {
			state = stLoaded
		}
		storyTasks := getStoryTasks(state, featName, storyID)
		if len(storyTasks) > 0 && deps.evaluator != nil {
			allTasksGreen := true
			for _, t := range storyTasks {
				// A task cannot be considered already green if it has no declared target files,
				// or if any of its target files do not exist or are empty on disk.
				if len(t.TargetFiles) == 0 {
					allTasksGreen = false
					break
				}
				filesExist := true
				for _, tf := range t.TargetFiles {
					if tf == "" {
						continue
					}
					fullPath := tf
					if !filepath.IsAbs(fullPath) {
						fullPath = filepath.Join(state.ProjectPath, tf)
					}
					fi, err := os.Stat(fullPath)
					if err != nil || fi.Size() == 0 {
						filesExist = false
						break
					}
				}
				if !filesExist {
					allTasksGreen = false
					break
				}

				passed, _, valErr := deps.evaluator.ValidateTask(ctx, state, t)
				if !passed || valErr != nil {
					allTasksGreen = false
					break
				}
			}
			if allTasksGreen {
				fmt.Printf("🚀 [Fast Exit on Verified Green] All %d tasks in %s already compile cleanly and pass all test assertions. Marking %s as SUCCESS.\n", len(storyTasks), featName, featName)
				now := time.Now().UTC()
				for i := range state.Tasks {
					for _, st := range storyTasks {
						if state.Tasks[i].ID == st.ID {
							state.Tasks[i].Status = domain.TaskSuccess
							state.Tasks[i].Progress = 100
							state.Tasks[i].UpdatedAt = now
						}
					}
				}
				for i, s := range state.Stories {
					if s.ID == featName || s.ID == storyID || s.FilePath == currentStoryFile {
						state.Stories[i].Status = domain.StorySuccess
						state.Stories[i].UpdatedAt = now
					}
				}
				_ = deps.repo.Save(ctx, state)
				if deps.gitClient != nil {
					_, _ = deps.gitClient.Run(ctx, true, "add", "roadmap")
					if statusOut, err := deps.gitClient.Run(ctx, false, "status", "--porcelain"); err == nil && strings.TrimSpace(statusOut) != "" {
						_, _ = deps.gitClient.Run(ctx, true, "commit", "-m", fmt.Sprintf("chore: complete story %s", featName))
					}
				}
				if deps.executionReporter != nil {
					deps.executionReporter.Observe(ctx, domain.ExecutionEvent{
						Kind:    domain.EventStoryFinished,
						StoryID: featName,
						Outcome: domain.OutcomeSuccess,
						At:      now,
					})
				}
				return nil
			}
		}

		// Immediate dispatch of initial ready tasks without waiting for the first ticker tick.
		_, _ = orchestrator.RunOnce(ctx)

		ticker := time.NewTicker(storyExecInterval(deps.cfg))
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-orchestrator.TaskCompletedChan():
			case <-orchestrator.StoryCompletedChan():
			case <-ticker.C:
			}
			_, _ = orchestrator.RunOnce(ctx)
			var st *domain.State
			var loadErr error
			if state != nil && state.ID != "" {
				st, loadErr = deps.repo.LoadByID(ctx, state.ID)
			}
			if loadErr != nil || st == nil {
				st, loadErr = deps.repo.Load(ctx)
			}
			if loadErr != nil {
				return loadErr
			}
			storyTasks := getStoryTasks(st, featName, storyID)
			if len(storyTasks) > 0 && allStoryTasksFinished(storyTasks) {
				// If story status is still running or idle, invoke RunOnce to execute Story QA and finalization gates
				if st.StoryStatus == domain.StoryRunning || st.StoryStatus == domain.StoryIdle {
					_, _ = orchestrator.RunOnce(ctx)
					var stReloaded *domain.State
					if state != nil && state.ID != "" {
						stReloaded, _ = deps.repo.LoadByID(ctx, state.ID)
					}
					if stReloaded == nil {
						stReloaded, _ = deps.repo.Load(ctx)
					}
					if stReloaded != nil {
						st = stReloaded
						storyTasks = getStoryTasks(st, featName, storyID)
						// If Story QA queued a new remediation task, continue the execution loop
						if !allStoryTasksFinished(storyTasks) {
							continue
						}
					}
				}
				for _, t := range storyTasks {
					if t.Status == domain.TaskFailed {
						return fmt.Errorf("story execution failed: task %s (%s) failed", t.ID, t.Title)
					}
				}
				for _, s := range st.Stories {
					if (s.ID == featName || s.ID == storyID || s.FilePath == currentStoryFile) && s.Status == domain.StoryFailed {
						return fmt.Errorf("story execution failed: story finalization status marked as failed")
					}
				}
				return nil
			}
		}
	}
}

func getStoryTasks(state *domain.State, featName, storyID string) []domain.Task {
	if state == nil {
		return nil
	}
	if len(state.Stories) <= 1 && len(state.Tasks) > 0 {
		return state.Tasks
	}
	var tasks []domain.Task
	storyLower := strings.ToLower(storyID)
	featLower := strings.ToLower(featName)
	normStoryID := services.ExtractStoryID(storyID)
	normFeatID := services.ExtractStoryID(featName)
	for _, t := range state.Tasks {
		idLower := strings.ToLower(t.ID)
		isRemediation := (storyLower != "" && strings.HasPrefix(idLower, "qa-remediation-"+storyLower)) ||
			(featLower != "" && strings.HasPrefix(idLower, "qa-remediation-"+featLower))

		normTaskStoryID := services.ExtractStoryID(t.StoryID)
		normTaskID := services.ExtractStoryID(t.ID)

		matchesStory := (storyID != "" && t.StoryID == storyID) ||
			(featName != "" && t.StoryID == featName) ||
			(storyID != "" && strings.HasPrefix(t.ID, storyID+"-")) ||
			(featName != "" && strings.HasPrefix(t.ID, featName+"-")) ||
			(normStoryID != "" && (normTaskStoryID == normStoryID || normTaskID == normStoryID)) ||
			(normFeatID != "" && (normTaskStoryID == normFeatID || normTaskID == normFeatID))

		if matchesStory || isRemediation {
			tasks = append(tasks, t)
		}
	}
	return tasks
}

func allStoryTasksFinished(tasks []domain.Task) bool {
	if len(tasks) == 0 {
		return false
	}
	for _, t := range tasks {
		if t.Status != domain.TaskSuccess && t.Status != domain.TaskFailed {
			return false
		}
	}
	return true
}

func buildStoryPlanner(deps storyExecutorDeps) func(ctx context.Context, currentStoryFile string) error {
	return func(ctx context.Context, currentStoryFile string) error {
		specBytes, err := os.ReadFile(currentStoryFile)
		if err != nil {
			return err
		}
		featName := filepath.Base(currentStoryFile)
		storyID := services.ExtractStoryID(currentStoryFile)
		if storyID == "" {
			storyID = featName
		}
		state, err := deps.repo.Load(ctx)
		if err != nil {
			return err
		}
		if state == nil {
			return errors.New("nil state loaded")
		}
		for _, t := range state.Tasks {
			if (storyID != "" && t.StoryID == storyID) || (featName != "" && t.StoryID == featName) {
				return nil
			}
		}

		planState := *state
		planState.Metadata.InputPath = currentStoryFile
		planState.Metadata.FeatureName = featName

		orchRuntime := services.OrchestratorRuntimeDependencies{
			Mailbox:        deps.mailbox,
			WatchdogRepair: deps.repairHandler,
			PromptRenderer: deps.promptRenderer,
			Observer:       deps.executionReporter,
		}
		orchestrator := services.NewOrchestratorWithRuntime(
			deps.repo, deps.reg, deps.llmClient, deps.validator, deps.scheduler,
			deps.gitClient, deps.rebaseQueue, deps.evaluator, deps.vcsClient, deps.orchConfig, orchRuntime,
		)
		return orchestrator.PlanStory(ctx, &planState, string(specBytes))
	}
}
