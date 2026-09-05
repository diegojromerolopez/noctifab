package cli

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/diegojromerolopez/noctifab/pkg/domain"
	"github.com/diegojromerolopez/noctifab/pkg/infrastructure/config"
	"github.com/diegojromerolopez/noctifab/pkg/services"
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
		configuredBranch := deps.cfg.VCS.GetIntegrationBranch()
		if strings.ToLower(deps.cfg.VCS.BranchStrategy) == "per_story_isolated" {
			configuredBranch = ""
		} else if configuredBranch == "" && len(deps.storyFiles) > 1 {
			prefix := deps.cfg.VCS.BranchPrefix
			if prefix == "" {
				prefix = "noctifab/"
			}
			configuredBranch = prefix + "implementation"
		}
		branchRes := services.ResolveBranches(ctx, deps.gitClient, deps.cfg.VCS.BaseBranch, configuredBranch, deps.cfg.VCS.BranchPrefix, featName)
		baseBranch := branchRes.BaseBranch
		integrationBranch := branchRes.IntegrationBranch

		state, err := deps.repo.Load(ctx)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				state = &domain.State{
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
		state.Metadata.InputPath = currentStoryFile
		state.Metadata.FeatureName = featName
		state.Metadata.BaseBranch = baseBranch
		state.Metadata.IntegrationBranch = integrationBranch
		state.StoryStatus = domain.StoryRunning

		now := time.Now().UTC()
		foundStory := false
		for i, st := range state.Stories {
			if st.ID == featName || st.FilePath == currentStoryFile {
				state.Stories[i].Status = domain.StoryRunning
				if state.Stories[i].StartedAt == nil {
					state.Stories[i].StartedAt = &now
				}
				state.Stories[i].UpdatedAt = now
				foundStory = true
				break
			}
		}
		if !foundStory {
			state.Stories = append(state.Stories, domain.Story{
				ID:        featName,
				StateID:   state.ID,
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
				services.UpsertStoryContract(state, contract)
			}
		}
		if err := deps.repo.Save(ctx, state); err != nil {
			return fmt.Errorf("failed to save initial state: %w", err)
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

		// Immediate dispatch of initial ready tasks without waiting for the first ticker tick.
		_, _ = orchestrator.RunOnce(ctx)

		ticker := time.NewTicker(storyExecInterval(deps.cfg))
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-ticker.C:
				_, _ = orchestrator.RunOnce(ctx)
				st, err := deps.repo.Load(ctx)
				if err != nil {
					return err
				}
				storyTasks := getStoryTasks(st, featName, storyID)
				if len(storyTasks) > 0 && allStoryTasksFinished(storyTasks) {
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
}

func getStoryTasks(state *domain.State, featName, storyID string) []domain.Task {
	if state == nil {
		return nil
	}
	if len(state.Stories) <= 1 && len(state.Tasks) > 0 {
		return state.Tasks
	}
	var tasks []domain.Task
	for _, t := range state.Tasks {
		if (storyID != "" && t.StoryID == storyID) || (featName != "" && t.StoryID == featName) ||
			(storyID != "" && strings.HasPrefix(t.ID, storyID+"-")) || (featName != "" && strings.HasPrefix(t.ID, featName+"-")) {
			tasks = append(tasks, t)
		}
	}
	if len(tasks) == 0 {
		return state.Tasks
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
