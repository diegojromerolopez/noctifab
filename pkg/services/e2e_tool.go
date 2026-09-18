package services

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/diegojromerolopez/noctifab/pkg/domain"
)

// RunE2ETestsTool implements run_e2e_tests, allowing any agent (tester, generator,
// QA auditor, sovereign rescue) to execute containerized and black-box E2E test suites.
type RunE2ETestsTool struct {
	Runner  Sandbox
	Timeout time.Duration
	E2EMode string
	E2ECmd  string
}

// Name returns the identifier of the tool.
func (t *RunE2ETestsTool) Name() string { return "run_e2e_tests" }

// Description returns the tool usage guidance for LLM agents.
func (t *RunE2ETestsTool) Description() string {
	return "run_e2e_tests executes the containerized end-to-end (E2E) test suite (via 'make e2e' or detected docker compose) to verify client-server integration. Arguments: command (optional, string)."
}

// Execute runs the E2E verification command in the project workspace.
func (t *RunE2ETestsTool) Execute(ctx context.Context, state *domain.State, args map[string]any) (string, error) {
	if t.Runner == nil {
		return "", errors.New("no sandbox execution engine registered for run_e2e_tests")
	}
	if state == nil || strings.TrimSpace(state.ProjectPath) == "" {
		return "", errors.New("invalid state: missing project path")
	}

	command, _ := args["command"].(string)
	command = strings.TrimSpace(command)

	if command == "" {
		command = DetectE2ECommand(state.ProjectPath, t.E2EMode, t.E2ECmd)
	}
	if command == "" {
		return "No E2E test suite configured or detected (checked make e2e, docker-compose.e2e.yml, docker-compose.yml).", nil
	}

	timeout := t.Timeout
	if timeout <= 0 {
		timeout = 5 * time.Minute
	}
	runCtx, runCancel := context.WithTimeout(ctx, timeout)
	defer runCancel()

	fmt.Fprintf(os.Stderr, "🐳 [E2E Tool] Executing E2E test command: %q...\n", command)
	out, err := t.Runner.RunCommand(runCtx, state.ProjectPath, command, "")
	if runCtx.Err() != nil && errors.Is(runCtx.Err(), context.DeadlineExceeded) {
		timeoutMsg := fmt.Sprintf("TIMEOUT: E2E command %q timed out after %v (possible infinite loop or container hang).\nLast output:\n%s", command, timeout, out)
		return timeoutMsg, fmt.Errorf("E2E command timed out after %v", timeout)
	}

	return out, err
}
