package services

import (
	"fmt"
	"strings"

	"github.com/diegojromerolopez/noctifab/pkg/domain"
)

// buildGeneratorPromptContext constructs the initial prompt context slices for the generator agent.
func buildGeneratorPromptContext(
	task domain.Task,
	state *domain.State,
	cleanFileContexts, cleanReaderContexts []string,
	recentTestsContext string,
	failedDiff string,
) []string {
	var promptContext []string
	if len(cleanFileContexts) > 0 {
		promptContext = append(promptContext, fmt.Sprintf("Existing files context:\n%s", strings.Join(cleanFileContexts, "\n\n")))
	}
	if len(cleanReaderContexts) > 0 {
		promptContext = append(promptContext, fmt.Sprintf("Inspection context gathered:\n%s", strings.Join(cleanReaderContexts, "\n\n")))
	}
	if recentTestsContext != "" {
		promptContext = append(promptContext, recentTestsContext)
	}
	if task.RecoveryDirective != "" {
		promptContext = append(promptContext, fmt.Sprintf("### ⚠️ PREVIOUS ATTEMPT STALL RECOVERY DIRECTIVE\n%s", task.RecoveryDirective))
	}
	if len(task.UserDirectives) > 0 {
		promptContext = append(promptContext, fmt.Sprintf("### 🎯 [USER HUMAN-IN-THE-LOOP STEERING DIRECTIVES]\n%s", strings.Join(task.UserDirectives, "\n")))
	}
	isRemediation := strings.HasPrefix(task.ID, "qa-remediation-") || strings.HasPrefix(task.ID, "spec-remediation-")
	if task.Retries > 0 || isRemediation {
		if task.FailureEnvelope != nil {
			var sb strings.Builder
			fmt.Fprintf(&sb, "### ⚠️ PREVIOUS ATTEMPT FAILURE DIAGNOSTICS (Stage: %s)\n", task.FailureEnvelope.Stage)
			if task.FailureEnvelope.Command != "" {
				fmt.Fprintf(&sb, "- **Command:** `%s`\n", task.FailureEnvelope.Command)
			}
			if task.FailureEnvelope.ExitCode != 0 {
				fmt.Fprintf(&sb, "- **Exit Code:** `%d`\n", task.FailureEnvelope.ExitCode)
			}
			if len(task.FailureEnvelope.FailingFiles) > 0 {
				fmt.Fprintf(&sb, "- **Failing Files:** `%s`\n", strings.Join(task.FailureEnvelope.FailingFiles, "`, `"))
			}
			if task.FailureEnvelope.Stderr != "" || task.FailureEnvelope.Stdout != "" {
				sb.WriteString("\n#### Output Logs:\n```\n")
				if task.FailureEnvelope.Stdout != "" {
					sb.WriteString(task.FailureEnvelope.Stdout)
					if !strings.HasSuffix(task.FailureEnvelope.Stdout, "\n") {
						sb.WriteString("\n")
					}
				}
				if task.FailureEnvelope.Stderr != "" {
					sb.WriteString(task.FailureEnvelope.Stderr)
					if !strings.HasSuffix(task.FailureEnvelope.Stderr, "\n") {
						sb.WriteString("\n")
					}
				}
				sb.WriteString("```\n")
			}
			promptContext = append(promptContext, sb.String())
		} else if task.FailureLog != "" {
			warning := "WARNING: The implementation/refactoring changes from this story have caused test failures. You must inspect the code and test changes that brought these issues, identify the bugs, and modify the files to fix the failures."
			diagCtx := buildFailureDiagnosticsContext(task, state.ProjectPath)
			promptContext = append(promptContext, fmt.Sprintf("%s\n\n%s", warning, diagCtx))
		}
		if strings.TrimSpace(failedDiff) != "" {
			diffTitle := "FAILED ATTEMPT GIT DIFF"
			if isRemediation {
				diffTitle = "USER STORY CHANGES THAT BROUGHT ISSUES (GIT DIFF)"
			}
			promptContext = append(promptContext, fmt.Sprintf("### 🔍 %s\nThe following diff shows the exact changes made by this user story that brought these issues:\n```diff\n%s\n```", diffTitle, strings.TrimSpace(failedDiff)))
		}
	}
	return promptContext
}
