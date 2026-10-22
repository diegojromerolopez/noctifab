package services

import (
	"context"
	"fmt"
	"strings"

	"github.com/diegojromerolopez/noctifab/pkg/domain"
	"github.com/diegojromerolopez/noctifab/pkg/infrastructure/config"
	"github.com/diegojromerolopez/noctifab/pkg/infrastructure/llm"
)

// EscalationDecision encapsulates the dynamic escalation actions chosen for a task execution turn.
type EscalationDecision struct {
	EnableThinking bool
	UpgradeModel   bool
	TargetModel    string
	ShouldSplit    bool
	CleanWorktree  bool
	Reason         string
}

// TaskEscalator defines the isolated contract for evaluating task complexity,
// dynamic thinking escalation, progressive model upgrades, and last-resort task splitting.
type TaskEscalator interface {
	EvaluateEscalation(ctx context.Context, task *domain.Task, turn int) EscalationDecision
	SplitTask(ctx context.Context, task *domain.Task, state *domain.State) ([]domain.Task, error)
}

// ProgressiveTaskEscalator implements the progressive thinking and model upgrade ladder.
type ProgressiveTaskEscalator struct {
	cfg                     config.TaskEscalationConfig
	availableUpgradedModels []string
}

// NewProgressiveTaskEscalator constructs a new ProgressiveTaskEscalator.
func NewProgressiveTaskEscalator(cfg config.TaskEscalationConfig, availableUpgradedModels []string) *ProgressiveTaskEscalator {
	upgrades := cfg.GetUpgradedModels()
	if len(upgrades) == 0 {
		upgrades = availableUpgradedModels
	}
	return &ProgressiveTaskEscalator{
		cfg:                     cfg,
		availableUpgradedModels: upgrades,
	}
}

// EvaluateEscalation evaluates the current state of a task and determines the next escalation step.
func (e *ProgressiveTaskEscalator) EvaluateEscalation(ctx context.Context, task *domain.Task, turn int) EscalationDecision {
	if task == nil || !e.cfg.IsEnabled() {
		return EscalationDecision{
			EnableThinking: false,
			UpgradeModel:   false,
			CleanWorktree:  false,
		}
	}

	complexity := strings.ToUpper(strings.TrimSpace(task.Complexity))
	if complexity != "LOW" && complexity != "HIGH" {
		complexity = "MEDIUM"
	}

	thinkingThreshold := e.cfg.GetThinkingThresholdTurns()
	if complexity == "HIGH" && thinkingThreshold > 1 {
		// High complexity tasks escalate to thinking mode faster (turn 1)
		thinkingThreshold = 1
	}

	modelUpgradeThreshold := e.cfg.GetModelUpgradeThresholdTurns()
	maxUpgrades := e.cfg.GetMaxModelUpgrades()

	// Check if task has exhausted maximum model upgrades -> Trigger last-resort task splitting
	if task.ModelUpgrades >= maxUpgrades && e.cfg.IsSplitOnMaxUpgradesEnabled() {
		return EscalationDecision{
			EnableThinking: true,
			UpgradeModel:   false,
			ShouldSplit:    true,
			CleanWorktree:  true,
			Reason:         fmt.Sprintf("task exhausted max model upgrades (%d/%d); splitting high-complexity task", task.ModelUpgrades, maxUpgrades),
		}
	}

	// Model upgrade threshold is met when retries exceed thinking threshold + upgrade threshold
	totalFails := task.Retries
	if totalFails >= (thinkingThreshold + modelUpgradeThreshold) {
		targetModel := e.selectUpgradedModel(task.ModelUpgrades)
		isThinkingOnly := llm.IsThinkingOnlyModel(targetModel)
		// For normal models, start upgraded model with thinking disabled (Step 3).
		// For thinking-only models, skip Step 3 and enable thinking immediately (Step 4).
		enableThinking := isThinkingOnly
		if totalFails >= (thinkingThreshold + modelUpgradeThreshold + 1) {
			enableThinking = true
		}

		return EscalationDecision{
			EnableThinking: enableThinking,
			UpgradeModel:   targetModel != "",
			TargetModel:    targetModel,
			CleanWorktree:  e.cfg.IsCleanWorktreeOnEscalateEnabled(),
			Reason:         fmt.Sprintf("failing task escalated to upgraded model '%s' (thinking=%v)", targetModel, enableThinking),
		}
	}

	// Thinking activation threshold
	if totalFails >= thinkingThreshold || turn >= thinkingThreshold {
		return EscalationDecision{
			EnableThinking: true,
			UpgradeModel:   false,
			CleanWorktree:  false,
			Reason:         fmt.Sprintf("task reached failure threshold (%d retries, turn %d); enabled thinking mode", totalFails, turn),
		}
	}

	// Step 1: Default fast path with thinking disabled
	return EscalationDecision{
		EnableThinking: false,
		UpgradeModel:   false,
		CleanWorktree:  false,
		Reason:         "baseline fast execution with thinking disabled",
	}
}

func (e *ProgressiveTaskEscalator) selectUpgradedModel(currentUpgrades int) string {
	if len(e.availableUpgradedModels) == 0 {
		return ""
	}
	idx := currentUpgrades
	if idx >= len(e.availableUpgradedModels) {
		idx = len(e.availableUpgradedModels) - 1
	}
	return e.availableUpgradedModels[idx]
}

// SplitTask decomposes a high-complexity task that failed maximum model upgrades into 2-3 focused subtasks.
func (e *ProgressiveTaskEscalator) SplitTask(ctx context.Context, task *domain.Task, state *domain.State) ([]domain.Task, error) {
	if task == nil {
		return nil, fmt.Errorf("cannot split nil task")
	}

	targetFiles := task.TargetFiles
	splitCount := 2
	if len(targetFiles) >= 4 {
		splitCount = 3
	}

	var subtasks []domain.Task
	var prevSubtaskID string

	// Partition target files across subtasks
	filesPerSubtask := (len(targetFiles) + splitCount - 1) / splitCount
	if filesPerSubtask <= 0 {
		filesPerSubtask = 1
	}

	suffixes := []string{"-A", "-B", "-C"}
	phases := []string{"Core Models & Interfaces", "Business Logic Implementation", "Integrations & Verification"}

	for i := 0; i < splitCount; i++ {
		subtaskID := task.ID + suffixes[i]
		title := fmt.Sprintf("%s (Part %d: %s)", task.Title, i+1, phases[i])
		desc := fmt.Sprintf("Decomposed subtask from high-complexity parent %s.\nPhase: %s.\nOriginal objective: %s", task.ID, phases[i], task.Description)

		startIdx := i * filesPerSubtask
		endIdx := startIdx + filesPerSubtask
		if startIdx > len(targetFiles) {
			startIdx = len(targetFiles)
		}
		if endIdx > len(targetFiles) {
			endIdx = len(targetFiles)
		}
		subTargetFiles := targetFiles[startIdx:endIdx]

		var deps []string
		if i == 0 {
			deps = append(deps, task.DependsOn...)
		} else {
			deps = append(deps, prevSubtaskID)
		}

		subtask := domain.Task{
			ID:           subtaskID,
			Title:        title,
			Description:  desc,
			Status:       domain.TaskPending,
			ChangeType:   task.ChangeType,
			DependsOn:    deps,
			TargetFiles:  subTargetFiles,
			StoryID:      task.StoryID,
			Complexity:   "MEDIUM",
			ParentTaskID: task.ID,
			Retries:      0,
			MaxRetries:   task.MaxRetries,
		}
		subtasks = append(subtasks, subtask)
		prevSubtaskID = subtaskID
	}

	return subtasks, nil
}
