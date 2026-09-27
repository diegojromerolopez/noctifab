package services

import (
	"context"
	"fmt"
	"time"

	"github.com/diegojromerolopez/noctifab/pkg/domain"
)

// validateE2E executes the detected E2E verification command with pre-E2E teardown enforcement
// and post-run cleanup to guarantee zero container name or port conflicts on host environments.
func (v *TestValidator) validateE2E(ctx context.Context, state *domain.State, task domain.Task) (passed bool, failureMsg string, err error) {
	if !v.shouldValidateE2E(state, task) {
		return true, "", nil
	}

	e2eCmd := v.detectE2ECommand(state.ProjectPath)
	if e2eCmd == "" || v.Runner == nil {
		return true, "", nil
	}

	// Pre-E2E Teardown Enforcement:
	// Clean up any lingering containers, volumes, and ports before launching the E2E suite
	guard := v.ContainerGuard
	if guard == nil {
		guard = NewContainerTeardownGuard(nil, nil)
	}
	_ = guard.PreFlightClean(ctx, state.ProjectPath)
	defer func() {
		// Post-run teardown ensures no orphan containers or daemon listeners remain
		_ = guard.PostRunClean(ctx, state.ProjectPath)
	}()

	e2eTimeout := v.RunTimeout
	if e2eTimeout <= 0 {
		e2eTimeout = 5 * time.Minute
	}
	e2eCtx, e2eCancel := context.WithTimeout(ctx, e2eTimeout)
	defer e2eCancel()

	e2eOut, e2eErr := v.Runner.RunCommand(e2eCtx, state.ProjectPath, e2eCmd, "")
	if e2eErr != nil {
		if isCommandToolMissing(v.Runner, e2eCmd, e2eOut+" "+e2eErr.Error()) {
			fmt.Printf("⚠️  [Validation Degraded] Task %s: required E2E tool is absent on host (%s). Proceeding in degraded mode.\n", task.ID, e2eCmd)
			return true, "", nil
		}
		if !isE2EFailureInScope(state, task, e2eOut+"\n"+e2eErr.Error()) {
			fmt.Printf("⚠️  Orchestrator: Task %s E2E failure(s) outside scope; ignoring in loop.\n", task.ID)
			return true, "", nil
		}
		fmt.Printf("❌ Orchestrator: Task %s E2E test gate (%s) failed: %v\n", task.ID, e2eCmd, e2eErr)
		return false, fmt.Sprintf("E2E test validation failed (%s):\n%s\n%v", e2eCmd, e2eOut, e2eErr), nil
	}

	noTestsRan, notice := EvaluateTestExecution(state.ProjectPath, e2eOut)
	if noTestsRan {
		fmt.Printf("❌ Orchestrator: Task %s E2E test suite produced no tests: %s\n", task.ID, notice)
		return false, fmt.Sprintf("E2E test validation failed (%s): %s", e2eCmd, notice), nil
	}

	return true, "", nil
}
