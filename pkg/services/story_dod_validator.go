package services

import (
	"context"
	"fmt"
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
