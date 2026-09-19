package cli

import (
	"context"
	"fmt"
	"strings"

	"github.com/diegojromerolopez/noctifab/pkg/infrastructure/config"
	"github.com/diegojromerolopez/noctifab/pkg/services"
)

func verifySovereignE2E(ctx context.Context, targetDir string, runner services.Sandbox, cfg *config.Config) (bool, string) {
	if runner == nil {
		return true, ""
	}
	e2eMode := ""
	e2eCmd := ""
	if cfg != nil {
		e2eMode = cfg.Sandbox.GetE2EMode()
		e2eCmd = cfg.Sandbox.GetE2ECommand()
	}
	detectedCmd := services.DetectE2ECommand(targetDir, e2eMode, e2eCmd)
	if detectedCmd == "" {
		return true, ""
	}

	fmt.Printf("🔍 [Sovereign Rescue] Running E2E verification gate: %q...\n", detectedCmd)
	e2eOut, e2eErr := runner.RunCommand(ctx, targetDir, detectedCmd, "")
	if e2eErr != nil {
		out := e2eOut
		if len(out) > 2000 {
			out = out[:2000] + "\n... [truncated]"
		}
		return false, fmt.Sprintf("❌ E2E test execution FAILED (%s):\n%s\nError: %v", detectedCmd, out, e2eErr)
	}
	fmt.Printf("✅ [Sovereign Rescue] E2E verification gate PASSED (%s)!\n", detectedCmd)
	return true, ""
}

func auditSovereignAntiStub(targetDir string) (bool, string) {
	antiStub := services.NewAntiStubValidator()
	violations, _ := antiStub.ValidateWorkspace(targetDir, nil)
	if len(violations) == 0 {
		return true, ""
	}

	var sb strings.Builder
	fmt.Fprintf(&sb, "⚠️ Anti-Stub & Non-Tautological Quality Gate FAILED (%d violation(s)):\n", len(violations))
	sb.WriteString("Tests and recipes must not be tautological, vacuous, or mask errors with shell tricks. You MUST write genuine behavioral assertions:\n")
	for _, v := range violations {
		fmt.Fprintf(&sb, "- %s:%d: [%s] %s\n", v.Path, v.Line, v.Rule, v.Snippet)
	}
	return false, sb.String()
}
