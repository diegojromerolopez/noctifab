package services

import (
	"context"
	"fmt"
	"os/exec"
	"strings"

	"github.com/diegojromerolopez/noctifab/pkg/infrastructure/telemetry"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

type DependencyManager struct {
	AllowedPkgManagers []string
}

type pkgEntry struct {
	Manager string
	Pkg     string
}

var toolPackageMap = map[string]pkgEntry{
	"cargo":         {"curl", "curl -sSf https://sh.rustup.rs | sh -s -- -y"},
	"pytest":        {"pip", "pip install pytest"},
	"golangci-lint": {"go", "go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest"},
	"node":          {"brew", "brew install node"},
	"npm":           {"brew", "brew install node"},
}

func NewDependencyManager(allowed []string) *DependencyManager {
	return &DependencyManager{AllowedPkgManagers: allowed}
}

func (dm *DependencyManager) DetectMissingTool(output string) (string, bool) {
	lower := strings.ToLower(output)
	patterns := []string{
		"executable file not found",
		"command not found",
		"no such file or directory",
		"exit status 127",
	}
	hasMissingPattern := false
	for _, p := range patterns {
		if strings.Contains(lower, p) {
			hasMissingPattern = true
			break
		}
	}
	if !hasMissingPattern {
		return "", false
	}

	for tool := range toolPackageMap {
		if strings.Contains(lower, tool) {
			return tool, true
		}
	}

	return "", false
}

func (dm *DependencyManager) IsAllowed(manager string) bool {
	for _, m := range dm.AllowedPkgManagers {
		if m == manager {
			return true
		}
	}
	return false
}

func (dm *DependencyManager) InstallTool(ctx context.Context, tool string) error {
	ctx, span := telemetry.Tracer().Start(ctx, "InstallTool",
		trace.WithAttributes(
			attribute.String("tool", tool),
			attribute.StringSlice("allowed_managers", dm.AllowedPkgManagers),
		))
	defer span.End()

	entry, ok := toolPackageMap[tool]
	if !ok {
		return fmt.Errorf("unknown tool %q: no package mapping available", tool)
	}
	if !dm.IsAllowed(entry.Manager) {
		return fmt.Errorf("package manager %q not in allowed list", entry.Manager)
	}

	parts := strings.Fields(entry.Pkg)
	cmd := exec.CommandContext(ctx, parts[0], parts[1:]...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to install %q via %q: %w\nOutput: %s", tool, entry.Pkg, err, string(output))
	}
	return nil
}
