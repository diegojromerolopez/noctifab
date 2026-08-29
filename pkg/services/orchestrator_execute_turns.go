package services

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/diegojromerolopez/noctifab/pkg/domain"
)

// executeTaskCodeFirst orchestrates the standard iterative tester/generator turns for a task.
func (o *Orchestrator) executeTaskCodeFirst(
	ctx context.Context,
	task *domain.Task,
	taskState *domain.State,
	taskGit *GitClient,
	fileContexts []string,
	taskID string,
) string {
	qaBlocked := ""
	if task.Retries == 0 {
		execOrder := strings.ToLower(strings.TrimSpace(o.cfg.TaskExecutionOrder))
		if execOrder == "tester_first" {
			qaBlocked = o.executeTesterFirstTurn(ctx, task, taskState, taskGit, fileContexts, taskID)
		} else {
			qaBlocked = o.executeGeneratorFirstTurn(ctx, task, taskState, taskGit, fileContexts, taskID)
		}
	} else {
		qaBlocked = o.executeRetryTurn(ctx, task, taskState, taskGit, fileContexts, taskID)
	}
	return qaBlocked
}

func (o *Orchestrator) executeTesterFirstTurn(
	ctx context.Context,
	task *domain.Task,
	taskState *domain.State,
	taskGit *GitClient,
	fileContexts []string,
	taskID string,
) string {
	// Ensure minimal compilation stub files exist before running tester on Turn 1
	o.ensureTargetStubFilesExist(taskState.ProjectPath, task)

	// 1. Run Test Writer Agent (role "tester") to write tests first
	o.updateTaskProgress(ctx, taskID, 25)
	o.RunTesterAgent(ctx, *task, taskState, fileContexts, "write", "")
	_ = o.stageAndCommit(ctx, taskGit, taskID, "test(core): write tests for task %s - %s", task.Title)

	// Post-Tester Quality Gate: ensure test suite is non-tautological and free of error-masking
	if testViolations := o.auditTesterTestOutput(taskState.ProjectPath); len(testViolations) > 0 {
		fmt.Printf("⚠️  [Post-Tester Gate] Task %s: Tester produced %d vacuous/tautological test violation(s). Triggering remediation turn...\n", taskID, len(testViolations))
		remediationCtx := append(fileContexts, o.formatTesterAntiStubViolations(testViolations))
		o.RunTesterAgent(ctx, *task, taskState, remediationCtx, "fix", "")
		_ = o.stageAndCommit(ctx, taskGit, taskID, "test(core): remediate vacuous tests for task %s - %s", task.Title)
	}

	// 2. Run Generator Agent (role "generator") to implement feature functionality to pass tests
	o.updateTaskProgress(ctx, taskID, 50)
	o.RunGeneratorAgent(ctx, *task, taskState, fileContexts, "", "implement")
	_ = o.stageAndCommit(ctx, taskGit, taskID, "feat(core): implement minimal functionality for task %s - %s", task.Title)

	// Read recently written tests from git to pass to the Generator Agent for the Refactor phase
	recentTestsContext := o.collectRecentTestsContext(ctx, taskGit, taskState.ProjectPath)

	// 3. Run Generator Agent (role "generator") to refactor/improve the implementation to pass the tests
	o.updateTaskProgress(ctx, taskID, 75)
	generatorHeadBefore, generatorHeadErr := taskGit.Run(ctx, false, "rev-parse", "HEAD")
	o.RunGeneratorAgent(ctx, *task, taskState, fileContexts, recentTestsContext, "refactor")

	qaBlocked := ""
	if commitErr := o.stageAndCommit(ctx, taskGit, taskID, "feat(core): refactor implementation for task %s - %s", task.Title); commitErr != nil {
		qaBlocked = "generator_commit_failed"
	}
	generatorHeadAfter, generatorAfterErr := taskGit.Run(ctx, false, "rev-parse", "HEAD")
	if qaBlocked == "" {
		qaBlocked = o.evaluateGeneratorTurnResult(ctx, taskState, task, generatorHeadBefore, generatorHeadAfter, generatorHeadErr, generatorAfterErr)
	}
	return qaBlocked
}

func (o *Orchestrator) executeGeneratorFirstTurn(
	ctx context.Context,
	task *domain.Task,
	taskState *domain.State,
	taskGit *GitClient,
	fileContexts []string,
	taskID string,
) string {
	// Ensure minimal compilation stub files exist before running generator on Turn 1
	o.ensureTargetStubFilesExist(taskState.ProjectPath, task)

	// 1. Run Generator Agent (role "generator") to implement minimal functionality
	o.updateTaskProgress(ctx, taskID, 25)
	o.RunGeneratorAgent(ctx, *task, taskState, fileContexts, "", "implement")
	_ = o.stageAndCommit(ctx, taskGit, taskID, "feat(core): implement minimal functionality for task %s - %s", task.Title)

	// Pre-Tester Quality Gate: ensure generator output is functional and contains no stubs
	if violations := o.auditGeneratorFunctionalOutput(taskState.ProjectPath, task.TargetFiles); len(violations) > 0 {
		fmt.Printf("⚠️  [Pre-Tester Gate] Task %s: Generator produced %d non-functional stub(s). Triggering remediation turn before testing...\n", taskID, len(violations))
		remediationCtx := append(fileContexts, o.formatAntiStubViolations(violations))
		o.RunGeneratorAgent(ctx, *task, taskState, remediationCtx, "", "fix")
		_ = o.stageAndCommit(ctx, taskGit, taskID, "feat(core): remediate non-functional stubs for task %s - %s", task.Title)
	}

	// 2. Run Test Writer Agent (role "tester") to write tests against the minimal implementation
	o.updateTaskProgress(ctx, taskID, 50)
	o.RunTesterAgent(ctx, *task, taskState, fileContexts, "write", "")
	_ = o.stageAndCommit(ctx, taskGit, taskID, "test(core): write tests for task %s - %s", task.Title)

	// Post-Tester Quality Gate: ensure test suite is non-tautological and free of error-masking
	if testViolations := o.auditTesterTestOutput(taskState.ProjectPath); len(testViolations) > 0 {
		fmt.Printf("⚠️  [Post-Tester Gate] Task %s: Tester produced %d vacuous/tautological test violation(s). Triggering remediation turn...\n", taskID, len(testViolations))
		remediationCtx := append(fileContexts, o.formatTesterAntiStubViolations(testViolations))
		o.RunTesterAgent(ctx, *task, taskState, remediationCtx, "fix", "")
		_ = o.stageAndCommit(ctx, taskGit, taskID, "test(core): remediate vacuous tests for task %s - %s", task.Title)
	}

	// Read recently written tests from git to pass to the Generator Agent for the Refactor phase
	recentTestsContext := o.collectRecentTestsContext(ctx, taskGit, taskState.ProjectPath)

	// 3. Run Generator Agent (role "generator") to refactor/improve the implementation to pass the tests
	o.updateTaskProgress(ctx, taskID, 75)
	generatorHeadBefore, generatorHeadErr := taskGit.Run(ctx, false, "rev-parse", "HEAD")
	o.RunGeneratorAgent(ctx, *task, taskState, fileContexts, recentTestsContext, "refactor")

	qaBlocked := ""
	if commitErr := o.stageAndCommit(ctx, taskGit, taskID, "feat(core): refactor implementation for task %s - %s", task.Title); commitErr != nil {
		qaBlocked = "generator_commit_failed"
	}
	generatorHeadAfter, generatorAfterErr := taskGit.Run(ctx, false, "rev-parse", "HEAD")
	if qaBlocked == "" {
		qaBlocked = o.evaluateGeneratorTurnResult(ctx, taskState, task, generatorHeadBefore, generatorHeadAfter, generatorHeadErr, generatorAfterErr)
	}
	return qaBlocked
}

func (o *Orchestrator) executeRetryTurn(
	ctx context.Context,
	task *domain.Task,
	taskState *domain.State,
	taskGit *GitClient,
	fileContexts []string,
	taskID string,
) string {
	// 1. Run Test Writer Agent to fix/refactor tests to align with updated implementation/failures
	o.updateTaskProgress(ctx, taskID, 40)
	o.RunTesterAgent(ctx, *task, taskState, fileContexts, "refactor", "")
	_ = o.stageAndCommit(ctx, taskGit, taskID, "test(core): fix/refactor tests for task %s - %s", task.Title)

	// Post-Tester Quality Gate: ensure test suite is non-tautological and free of error-masking
	if testViolations := o.auditTesterTestOutput(taskState.ProjectPath); len(testViolations) > 0 {
		fmt.Printf("⚠️  [Post-Tester Gate] Task %s: Tester produced %d vacuous/tautological test violation(s). Triggering remediation turn...\n", taskID, len(testViolations))
		remediationCtx := append(fileContexts, o.formatTesterAntiStubViolations(testViolations))
		o.RunTesterAgent(ctx, *task, taskState, remediationCtx, "fix", "")
		_ = o.stageAndCommit(ctx, taskGit, taskID, "test(core): remediate vacuous tests for task %s - %s", task.Title)
	}

	// Read recently written/fixed tests from git to pass to the Generator Agent
	recentTestsContext := o.collectRecentTestsContext(ctx, taskGit, taskState.ProjectPath)

	// 2. Run Generator Agent to fix/refactor implementation to pass the tests
	o.updateTaskProgress(ctx, taskID, 70)
	generatorHeadBefore, generatorHeadErr := taskGit.Run(ctx, false, "rev-parse", "HEAD")
	o.RunGeneratorAgent(ctx, *task, taskState, fileContexts, recentTestsContext, "fix")

	qaBlocked := ""
	if commitErr := o.stageAndCommit(ctx, taskGit, taskID, "feat(core): fix/refactor implementation for task %s - %s", task.Title); commitErr != nil {
		qaBlocked = "generator_commit_failed"
	}
	generatorHeadAfter, generatorAfterErr := taskGit.Run(ctx, false, "rev-parse", "HEAD")
	if qaBlocked == "" {
		qaBlocked = o.evaluateGeneratorTurnResult(ctx, taskState, task, generatorHeadBefore, generatorHeadAfter, generatorHeadErr, generatorAfterErr)
	}
	return qaBlocked
}

func (o *Orchestrator) evaluateGeneratorTurnResult(
	ctx context.Context,
	taskState *domain.State,
	task *domain.Task,
	beforeHead, afterHead string,
	beforeErr, afterErr error,
) string {
	if !o.cfg.QA.Enabled {
		return ""
	}
	if beforeErr != nil || afterErr != nil {
		return "generator_commit_failed"
	}
	if strings.TrimSpace(beforeHead) == strings.TrimSpace(afterHead) {
		// Generator made no new commit in this refactor phase.
		// Check if the current workspace already satisfies validation.
		if o.evaluator != nil {
			passed, valMsg, _ := o.evaluator.ValidateTask(ctx, taskState, *task)
			if passed {
				// Clean state: workspace already passes validation without additional edits.
				return ""
			}
			task.FailureLog = fmt.Sprintf("Generator produced no changes, but validation is FAILING:\n%s", valMsg)
			return "generator_no_op_validation_failed"
		}
		return "generator_commit_failed"
	}
	return ""
}

func (o *Orchestrator) executeSurgicalRepairTurn(
	ctx context.Context,
	task *domain.Task,
	taskState *domain.State,
	taskGit *GitClient,
	failureLog string,
) {
	o.updateTaskProgress(ctx, task.ID, 85)
	summary := summarizeFailureLog(failureLog)
	errorContext := fmt.Sprintf("### 🎯 TARGET FAILURE LOG TRACE FOR SURGICAL REPAIR:\n```\n%s\n```\nFix this specific error using minimal edits in 'edit_file'. Do not rewrite working code.", summary)
	o.RunGeneratorAgent(ctx, *task, taskState, nil, errorContext, "surgical_repair")
	_ = o.stageAndCommit(ctx, taskGit, task.ID, "fix(core): surgical repair for task %s - %s", task.Title)
}

func (o *Orchestrator) stageAndCommit(ctx context.Context, taskGit *GitClient, taskID string, format string, args ...interface{}) error {
	statusOut, _ := taskGit.Run(ctx, false, "status", "--porcelain")
	if strings.TrimSpace(statusOut) == "" {
		return nil
	}

	// Issue 6: Zero-token auto-formatting before git commit
	if o.evaluator != nil && o.evaluator.FormatterCommand != "" && o.evaluator.Runner != nil && taskGit != nil && taskGit.Dir() != "" {
		_, _ = o.evaluator.Runner.RunCommand(ctx, taskGit.Dir(), o.evaluator.FormatterCommand, "")
	}

	_, _ = taskGit.Run(ctx, true, "add", "--all", "--", ":!.noctifab")
	stagedOut, _ := taskGit.Run(ctx, false, "diff", "--cached", "--name-only")
	if strings.TrimSpace(stagedOut) == "" {
		return nil
	}
	commitMsg := fmt.Sprintf(format, append([]interface{}{taskID}, args...)...)
	_, commitErr := taskGit.Run(ctx, true, "commit", "-m", commitMsg)
	if commitErr != nil {
		fmt.Fprintf(os.Stderr, "Orchestrator: Git commit failed for task %s: %v\n", taskID, commitErr)
		return commitErr
	}
	return nil
}

func (o *Orchestrator) collectRecentTestsContext(ctx context.Context, taskGit *GitClient, projectPath string) string {
	diffOut, diffErr := taskGit.Run(ctx, false, "show", "--name-only", "--format=", "HEAD")
	if diffErr != nil {
		return ""
	}
	var testFileContexts []string
	for _, file := range strings.Split(diffOut, "\n") {
		file = strings.TrimSpace(file)
		if file != "" && (strings.Contains(file, "tests/") || strings.Contains(file, "spec/")) {
			fullPath, err := resolveSandboxPath(projectPath, file)
			if err == nil {
				if content, err := os.ReadFile(fullPath); err == nil {
					testFileContexts = append(testFileContexts, fmt.Sprintf("Test File %s:\n```\n%s\n```", file, capText(string(content), fileContextCapChars)))
				}
			}
		}
	}
	if len(testFileContexts) > 0 {
		return "\n\nWritten/Fixed tests context:\n" + strings.Join(testFileContexts, "\n\n")
	}
	return ""
}

func (o *Orchestrator) auditGeneratorFunctionalOutput(projectPath string, targetFiles []string) []AntiStubViolation {
	antiStub := NewAntiStubValidator()
	violations, _ := antiStub.ValidateWorkspace(projectPath, targetFiles)
	return violations
}

func (o *Orchestrator) formatAntiStubViolations(violations []AntiStubViolation) string {
	var sb strings.Builder
	fmt.Fprintf(&sb, "\n\n⚠️ PRE-TEST FUNCTIONAL AUDIT REJECTION (%d violation(s)):\n", len(violations))
	sb.WriteString("Your implementation was rejected because it contains placeholder stubs or non-functional dummies. You MUST implement real, working functional logic:\n")
	for _, v := range violations {
		fmt.Fprintf(&sb, "- %s:%d: [%s] %s\n", v.Path, v.Line, v.Rule, v.Snippet)
	}
	return sb.String()
}

func (o *Orchestrator) auditTesterTestOutput(projectPath string) []AntiStubViolation {
	antiStub := NewAntiStubValidator()
	violations, _ := antiStub.ValidateWorkspace(projectPath, nil)
	return violations
}

func (o *Orchestrator) formatTesterAntiStubViolations(violations []AntiStubViolation) string {
	var sb strings.Builder
	fmt.Fprintf(&sb, "\n\n⚠️ TEST SUITE AUDIT REJECTION (%d violation(s)):\n", len(violations))
	sb.WriteString("Your test suite was rejected because it contains vacuous/tautological assertions or shell error masking. You MUST write real, behavioral tests:\n")
	for _, v := range violations {
		fmt.Fprintf(&sb, "- %s:%d: [%s] %s\n", v.Path, v.Line, v.Rule, v.Snippet)
	}
	return sb.String()
}
