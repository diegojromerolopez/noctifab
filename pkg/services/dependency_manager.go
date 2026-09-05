package services

import (
	"context"
	"fmt"
	"os/exec"
	"sort"
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
	"pytest-cov":    {"pip", "pip install pytest-cov"},
	"pytest-django": {"pip", "pip install pytest-django"},
	"coverage":      {"pip", "pip install coverage"},
	"vitest":        {"npm", "npm install -g vitest"},
	"jest":          {"npm", "npm install -g jest"},
	"ts-jest":       {"npm", "npm install -g ts-jest"},
	"ts-node":       {"npm", "npm install -g ts-node"},
	"typescript":    {"npm", "npm install -g typescript"},
	"tsc":           {"npm", "npm install -g typescript"},
	"supertest":     {"npm", "npm install -g supertest"},
	"rspec":         {"gem", "gem install rspec"},
	"rubocop":       {"gem", "gem install rubocop"},
	"gradle":        {"apk", "apk add --no-cache gradle"},
	"clang-format":  {"apk", "apk add --no-cache clang-extra-tools"},
	"dune":          {"opam", "opam install -y dune"},
	"golangci-lint": {"go", "go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest"},
	"node":          {"brew", "brew install node"},
	"npm":           {"brew", "brew install node"},
}

// sortedToolNames is ordered longest-first so compound tool names (e.g. ts-node)
// take precedence over shorter substrings (e.g. node).
var sortedToolNames = func() []string {
	tools := make([]string, 0, len(toolPackageMap))
	for tool := range toolPackageMap {
		tools = append(tools, tool)
	}
	sort.Slice(tools, func(i, j int) bool {
		if len(tools[i]) != len(tools[j]) {
			return len(tools[i]) > len(tools[j])
		}
		return tools[i] < tools[j]
	})
	return tools
}()

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
		": not found",
		"not found",
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

	for _, tool := range sortedToolNames {
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
