package services

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/diegojromerolopez/noctifab/pkg/domain"
)

const maxConvergenceTurns = 4

// executeConvergenceRepairLoop executes an autonomous closed-loop repair cycle.
// In every turn, deterministic auto-fixes (formatters, stub seeding, imports) are applied first.
// If deterministic fixes do not satisfy validation, the exact compiler/test error diagnostics
// along with the offending source code snippets from disk (with visual pointers marking error lines)
// are supplied to the Generator agent for surgical repair, iterating until convergence.
func (o *Orchestrator) executeConvergenceRepairLoop(
	ctx context.Context,
	task *domain.Task,
	taskState *domain.State,
	taskGit *GitClient,
	initialFailureLog string,
	integrationBranch string,
) (bool, string) {
	workDir := taskState.ProjectPath
	if taskGit != nil && taskGit.Dir() != "" {
		workDir = taskGit.Dir()
	}

	currentLog := initialFailureLog
	var passed bool

	for turn := 1; turn <= maxConvergenceTurns; turn++ {
		fmt.Printf("Orchestrator: [Convergence Loop] Task %s starting Turn %d/%d...\n", task.ID, turn, maxConvergenceTurns)
		o.updateTaskProgress(ctx, task.ID, 70+turn*5)

		// 1. Deterministic Auto-Fix Step: run formatters, stubs, and imports first
		o.runDeterministicAutoFixes(ctx, task, workDir)

		// Check if deterministic fixes resolved the failure
		passed, currentLog, _ = o.evaluator.ValidateTask(ctx, taskState, *task)
		passed, currentLog = checkTaskMutations(ctx, task, taskGit, integrationBranch, passed, currentLog)
		passed, currentLog = checkTaskStoryContracts(ctx, o.evaluator, taskState, task, passed, currentLog)
		if passed {
			fmt.Printf("✅ [Convergence Loop] Task %s successfully converged via deterministic fixes on Turn %d!\n", task.ID, turn)
			return true, currentLog
		}

		// 2. Deterministic fix did not work -> Extract diagnostics and offending code for Agent
		diagnostics := extractDiagnostics(currentLog)
		offendingSnippets := ExtractOffendingCodeSnippets(workDir, diagnostics, 5)

		fileContexts := o.gatherRepairFileContexts(workDir, diagnostics, task.TargetFiles)
		repairPrompt := o.formatConvergenceRepairPrompt(currentLog, offendingSnippets, turn, maxConvergenceTurns)

		// Run generator in surgical repair mode with the error and offending code
		o.RunGeneratorAgent(ctx, *task, taskState, fileContexts, repairPrompt, "surgical_repair")

		// Traceback-to-Diff Alignment Guard: Prevent sycophantic repairs that touch unrelated files
		if taskGit != nil {
			modifiedFilesOut, _ := taskGit.Run(ctx, false, "diff", "--name-only")
			var modifiedFiles []string
			for _, f := range strings.Split(modifiedFilesOut, "\n") {
				if trimmed := strings.TrimSpace(f); trimmed != "" {
					modifiedFiles = append(modifiedFiles, trimmed)
				}
			}
			if len(modifiedFiles) > 0 {
				guard := NewRepairAlignmentGuard()
				tracebackFiles := guard.ExtractTracebackFiles(workDir, currentLog)
				alignment := guard.ValidateRepairAlignment(modifiedFiles, tracebackFiles, task.TargetFiles, "", nil)
				if !alignment.Allowed {
					fmt.Fprintf(os.Stderr, "⚠️ [Repair Alignment Guard Rejected] %s\n", alignment.Reason)
					_, _ = taskGit.Run(ctx, true, "checkout", "--", ".")
					continue
				}
			}
		}

		_ = o.stageAndCommit(ctx, taskGit, task.ID, "fix(core): convergence repair turn %d for task %s - %s", turn, task.ID, task.Title)

		// Re-validate after agent repair
		passed, currentLog, _ = o.evaluator.ValidateTask(ctx, taskState, *task)
		passed, currentLog = checkTaskMutations(ctx, task, taskGit, integrationBranch, passed, currentLog)
		passed, currentLog = checkTaskStoryContracts(ctx, o.evaluator, taskState, task, passed, currentLog)
		if passed {
			fmt.Printf("✅ [Convergence Loop] Task %s successfully converged on Turn %d!\n", task.ID, turn)
			return true, currentLog
		}
	}

	// 3. Directive 7: Resilience & Forced Compilation Mandate (Fallback Simplification Turn)
	fmt.Printf("⚠️ [Convergence Loop] Task %s exhausted %d turns. Attempting fallback simplification...\n", task.ID, maxConvergenceTurns)
	diagnostics := extractDiagnostics(currentLog)
	fileContexts := o.gatherRepairFileContexts(workDir, diagnostics, task.TargetFiles)
	simplificationPrompt := o.formatSimplificationPrompt(currentLog, task)

	o.RunGeneratorAgent(ctx, *task, taskState, fileContexts, simplificationPrompt, "surgical_repair")
	_ = o.stageAndCommit(ctx, taskGit, task.ID, "fix(core): fallback simplification for task %s - %s", task.ID, task.Title)

	o.runDeterministicAutoFixes(ctx, task, workDir)
	passed, currentLog, _ = o.evaluator.ValidateTask(ctx, taskState, *task)
	passed, currentLog = checkTaskMutations(ctx, task, taskGit, integrationBranch, passed, currentLog)
	passed, currentLog = checkTaskStoryContracts(ctx, o.evaluator, taskState, task, passed, currentLog)
	if passed {
		fmt.Printf("✅ [Convergence Loop] Task %s converged via fallback simplification!\n", task.ID)
		return true, currentLog
	}

	// 4. Fallback Agent Escalation (if enabled)
	fbCfg := o.cfg.GetFallback()
	if fbCfg.Enabled {
		fmt.Printf("Orchestrator: Task %s escalating to Fallback Agent...\n", task.ID)
		fbPassed, fbLog := o.RunFallbackAgent(ctx, task, taskState, taskGit, currentLog, "convergence_loop_exhausted")
		if fbPassed {
			return true, fbLog
		}
		currentLog = fbLog
	}

	return false, currentLog
}

func (o *Orchestrator) runDeterministicAutoFixes(ctx context.Context, task *domain.Task, workDir string) {
	if o.evaluator != nil && o.evaluator.Runner != nil {
		RunDeterministicAutoFormat(ctx, o.evaluator.Runner, workDir)
	}
}

func (o *Orchestrator) gatherRepairFileContexts(workDir string, diagnostics []DiagnosticError, targetFiles []string) []string {
	var fileContexts []string
	seenFiles := make(map[string]bool)

	// Pre-read failing files from diagnostic log
	for _, d := range diagnostics {
		if d.FilePath != "" && !seenFiles[d.FilePath] {
			seenFiles[d.FilePath] = true
			fullPath := findSourceFile(workDir, d.FilePath)
			if fullPath != "" {
				if content, err := os.ReadFile(fullPath); err == nil && len(content) > 0 {
					relPath, _ := filepath.Rel(workDir, fullPath)
					if relPath == "" {
						relPath = d.FilePath
					}
					fileContexts = append(fileContexts, fmt.Sprintf("File %s:\n```\n%s\n```", relPath, string(content)))
				}
			}
		}
	}

	// Pre-read task target files
	for _, target := range targetFiles {
		if target != "" && !seenFiles[target] {
			seenFiles[target] = true
			fullPath := findSourceFile(workDir, target)
			if fullPath != "" {
				if content, err := os.ReadFile(fullPath); err == nil && len(content) > 0 {
					relPath, _ := filepath.Rel(workDir, fullPath)
					if relPath == "" {
						relPath = target
					}
					fileContexts = append(fileContexts, fmt.Sprintf("File %s:\n```\n%s\n```", relPath, string(content)))
				}
			}
		}
	}

	return fileContexts
}

func (o *Orchestrator) formatConvergenceRepairPrompt(failureLog, offendingSnippets string, turn, maxTurns int) string {
	summary := summarizeFailureLog(failureLog)
	var sb strings.Builder

	fmt.Fprintf(&sb, "### 🎯 CONVERGENCE REPAIR ITERATION (Turn %d of %d):\n", turn, maxTurns)
	sb.WriteString("Validation failed with the following diagnostic error:\n")
	sb.WriteString("```\n")
	sb.WriteString(summary)
	sb.WriteString("\n```\n\n")

	if offendingSnippets != "" {
		sb.WriteString(offendingSnippets)
		sb.WriteString("\n")
	}

	sb.WriteString("### ⚡ ACTIONABLE REPAIR DIRECTIVES:\n")
	sb.WriteString("1. Inspect the offending code line(s) marked with '>>' and surrounding context.\n")
	sb.WriteString("2. Use 'edit_file' to surgically correct the syntax, missing type/import, or invalid parameter at that exact location.\n")
	sb.WriteString("3. If this is a build manifest (Makefile, Cargo.toml), add the missing target or fix the dependency specification.\n")
	sb.WriteString("4. Do not remove or stub out functional code. Implement the correct language constructs.\n")

	return sb.String()
}

func (o *Orchestrator) formatSimplificationPrompt(failureLog string, task *domain.Task) string {
	summary := summarizeFailureLog(failureLog)
	var sb strings.Builder

	sb.WriteString("### 🚨 RESILIENCE & COMPILATION MANDATE (Directive 7):\n")
	sb.WriteString("All previous surgical repairs failed to converge. The current error is:\n")
	sb.WriteString("```\n")
	sb.WriteString(summary)
	sb.WriteString("\n```\n\n")
	sb.WriteString("Mandate: Provide a clean, minimal, cleanly compiling implementation for the target functionality.\n")
	sb.WriteString("Simplify complex type constructs, eliminate circular references, or resolve conflicting signatures.\n")
	sb.WriteString("The code MUST compile cleanly without any syntax or type errors.\n")

	return sb.String()
}
