package services

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// ComposeRunner runs docker compose teardown commands in a directory.
type ComposeRunner func(ctx context.Context, dir string, name string, args ...string) ([]byte, error)

// defaultComposeRunner runs commands via os/exec.
func defaultComposeRunner(ctx context.Context, dir string, name string, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Dir = dir
	return cmd.CombinedOutput()
}

// ContainerTeardownGuard deterministically stops and cleans up orphan containers,
// volumes, and network socket bindings associated with a project.
type ContainerTeardownGuard struct {
	composeRunner ComposeRunner
	portReaper    *PortReaper
	timeout       time.Duration
}

// NewContainerTeardownGuard constructs a new ContainerTeardownGuard.
func NewContainerTeardownGuard(runner ComposeRunner, reaper *PortReaper) *ContainerTeardownGuard {
	if runner == nil {
		runner = defaultComposeRunner
	}
	if reaper == nil {
		reaper = NewPortReaper()
	}
	return &ContainerTeardownGuard{
		composeRunner: runner,
		portReaper:    reaper,
		timeout:       10 * time.Second,
	}
}

// DiscoverComposeFiles finds all docker compose manifests in projectPath.
func (g *ContainerTeardownGuard) DiscoverComposeFiles(projectPath string) []string {
	if projectPath == "" {
		return nil
	}

	candidates := []string{
		"docker-compose.yml", "docker-compose.yaml",
		"docker-compose.e2e.yml", "docker-compose.e2e.yaml",
		"docker-compose.test.yml", "docker-compose.test.yaml",
	}

	var found []string
	for _, c := range candidates {
		full := filepath.Join(projectPath, c)
		if info, err := os.Stat(full); err == nil && !info.IsDir() {
			found = append(found, c)
		}
	}
	return found
}

// TeardownProjectEnvironment cleans up all containers and unbound ports for projectPath.
func (g *ContainerTeardownGuard) TeardownProjectEnvironment(ctx context.Context, projectPath string) error {
	if projectPath == "" {
		return nil
	}

	composeFiles := g.DiscoverComposeFiles(projectPath)
	for _, cf := range composeFiles {
		runCtx, cancel := context.WithTimeout(ctx, g.timeout)
		// Run docker compose down with volume purge and orphan removal
		out, err := g.composeRunner(runCtx, projectPath, "docker", "compose", "-f", cf, "down", "-v", "--remove-orphans", "--timeout", "3")
		cancel()
		if err != nil {
			// Non-fatal if docker is not running or compose file was not active
			_ = out
		}
	}

	// Detect ports and reap any remaining host or container listening processes
	ports, _ := DetectProjectPorts(projectPath)
	if len(ports) > 0 && g.portReaper != nil {
		reapCtx, cancel := context.WithTimeout(ctx, g.timeout)
		defer cancel()
		_, _, _ = g.portReaper.ReapOrphanPortHolders(reapCtx, ports)

		// Verify ports are free
		var stillBound []int
		for _, p := range ports {
			if g.portReaper.IsPortInUse(p) {
				stillBound = append(stillBound, p)
			}
		}
		if len(stillBound) > 0 {
			return fmt.Errorf("project port(s) %v still bound after container and socket teardown", stillBound)
		}
	}

	return nil
}

// PreFlightClean checks and frees any lingering containers or sockets before a task starts.
func (g *ContainerTeardownGuard) PreFlightClean(ctx context.Context, projectPath string) error {
	return g.TeardownProjectEnvironment(ctx, projectPath)
}

// PostRunClean ensures no orphan containers or daemon listeners remain after a task completes.
func (g *ContainerTeardownGuard) PostRunClean(ctx context.Context, projectPath string) error {
	return g.TeardownProjectEnvironment(ctx, projectPath)
}

// FormatTeardownSummary provides a user-facing report of cleaned artifacts.
func (g *ContainerTeardownGuard) FormatTeardownSummary(composeFiles []string, ports []int) string {
	var parts []string
	if len(composeFiles) > 0 {
		parts = append(parts, fmt.Sprintf("tore down compose files: %s", strings.Join(composeFiles, ", ")))
	}
	if len(ports) > 0 {
		parts = append(parts, fmt.Sprintf("unbound port(s): %v", ports))
	}
	if len(parts) == 0 {
		return "clean environment (no containers or ports detected)"
	}
	return strings.Join(parts, "; ")
}
