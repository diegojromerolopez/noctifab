package services

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/diegojromerolopez/noctifab/pkg/domain"
)

// DoDValidationReport captures deterministic results from executing public contracts.
type DoDValidationReport struct {
	StoryID           string   `json:"story_id"`
	Passed            bool     `json:"passed"`
	ExecutedContracts int      `json:"executed_contracts"`
	Failures          []string `json:"failures,omitempty"`
}

// StoryDoDValidator executes machine-readable public contracts and verifies
// observable invariants deterministically without relying on LLM evaluation.
type StoryDoDValidator struct {
	runner  Sandbox
	timeout time.Duration
}

// NewStoryDoDValidator creates a new validator.
func NewStoryDoDValidator(runner Sandbox, timeout time.Duration) *StoryDoDValidator {
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	return &StoryDoDValidator{
		runner:  runner,
		timeout: timeout,
	}
}

// ValidateStoryContracts executes all PublicContract specifications defined for a story.
func (v *StoryDoDValidator) ValidateStoryContracts(ctx context.Context, projectPath string, contract domain.StoryContract) (*DoDValidationReport, error) {
	report := &DoDValidationReport{
		StoryID: contract.StoryID,
		Passed:  true,
	}

	if len(contract.PublicContracts) == 0 {
		return report, nil
	}

	for _, pc := range contract.PublicContracts {
		report.ExecutedContracts++
		if len(pc.AllowedExecutables) == 0 {
			continue
		}

		// Try the primary executable
		execCmd := pc.AllowedExecutables[0]
		execCtx, cancel := context.WithTimeout(ctx, v.timeout)
		out, err := v.runner.RunCommand(execCtx, projectPath, execCmd, "")
		cancel()

		// 1. Verify Exit Code
		if len(pc.ExitCodes) > 0 {
			exitCode := 0
			if err != nil {
				exitCode = 1
			}
			matched := false
			for _, expected := range pc.ExitCodes {
				if exitCode == expected {
					matched = true
					break
				}
			}
			if !matched {
				report.Passed = false
				report.Failures = append(report.Failures,
					fmt.Sprintf("Contract %q [%s]: expected exit codes %v, got %d (err: %v)", pc.ID, pc.Interface, pc.ExitCodes, exitCode, err))
			}
		}

		// 2. Verify Stdout Substrings
		for _, substr := range pc.StdoutContains {
			if !strings.Contains(out, substr) {
				report.Passed = false
				report.Failures = append(report.Failures,
					fmt.Sprintf("Contract %q [%s]: stdout missing required pattern %q. Output was: %s", pc.ID, pc.Interface, substr, capText(out, 500)))
			}
		}

		// 3. Verify Stderr Prefixes (if error occurred)
		if err != nil && len(pc.StderrPrefixes) > 0 {
			errStr := err.Error()
			matched := false
			for _, prefix := range pc.StderrPrefixes {
				if strings.HasPrefix(strings.TrimSpace(errStr), prefix) || strings.HasPrefix(strings.TrimSpace(out), prefix) {
					matched = true
					break
				}
			}
			if !matched {
				report.Passed = false
				report.Failures = append(report.Failures,
					fmt.Sprintf("Contract %q [%s]: error output missing expected stderr prefix from %v", pc.ID, pc.Interface, pc.StderrPrefixes))
			}
		}
	}

	return report, nil
}

// checkTaskStoryContracts runs a pre-merge black-box contract smoke check for any
// public contract whose target executable binary already exists on disk in the task worktree.
func checkTaskStoryContracts(ctx context.Context, evaluator *TestValidator, taskState *domain.State, task *domain.Task, passed bool, logMsg string) (bool, string) {
	if !passed || evaluator == nil || evaluator.Runner == nil || taskState == nil || task == nil {
		return passed, logMsg
	}

	var contract domain.StoryContract
	for _, sc := range taskState.StoryContracts {
		if task.StoryID != "" && sc.StoryID == task.StoryID {
			contract = sc
			break
		}
		contract = sc
	}
	if len(contract.PublicContracts) == 0 {
		return passed, logMsg
	}

	ready := filterReadyTaskContracts(taskState.ProjectPath, contract.PublicContracts, task.TargetFiles)
	if len(ready) == 0 {
		return passed, logMsg
	}

	validator := NewStoryDoDValidator(evaluator.Runner, 15*time.Second)
	report, err := validator.ValidateStoryContracts(ctx, taskState.ProjectPath, domain.StoryContract{
		StoryID:         contract.StoryID,
		PublicContracts: ready,
	})
	if err != nil || report == nil || report.Passed {
		return passed, logMsg
	}

	failureMsg := fmt.Sprintf("Task pre-merge black-box contract smoke check failed:\n%s", strings.Join(report.Failures, "\n"))
	if strings.TrimSpace(logMsg) != "" {
		failureMsg = logMsg + "\n\n" + failureMsg
	}
	return false, failureMsg
}

func filterReadyTaskContracts(projectPath string, contracts []domain.PublicContract, targetFiles []string) []domain.PublicContract {
	if projectPath == "" {
		return nil
	}
	var ready []domain.PublicContract
	for _, pc := range contracts {
		if len(pc.AllowedExecutables) == 0 {
			continue
		}
		fields := strings.Fields(pc.AllowedExecutables[0])
		if len(fields) == 0 {
			continue
		}
		execToken := fields[0]
		if !strings.HasPrefix(execToken, "./") && !strings.HasPrefix(execToken, "bin/") && !strings.Contains(execToken, "/") {
			continue
		}
		binPath := filepath.Join(projectPath, filepath.Clean(execToken))
		info, err := os.Stat(binPath)
		if err != nil || info.IsDir() {
			continue
		}
		if len(pc.ApplicablePathPrefixes) > 0 && len(targetFiles) > 0 {
			matchedPrefix := false
			for _, tf := range targetFiles {
				cleanTF := strings.TrimPrefix(filepath.ToSlash(tf), "./")
				for _, prefix := range pc.ApplicablePathPrefixes {
					cleanPrefix := strings.TrimPrefix(filepath.ToSlash(prefix), "./")
					if strings.HasPrefix(cleanTF, cleanPrefix) {
						matchedPrefix = true
						break
					}
				}
				if matchedPrefix {
					break
				}
			}
			if !matchedPrefix {
				continue
			}
		}
		ready = append(ready, pc)
	}
	return ready
}
