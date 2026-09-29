package services

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// PreflightReport summarizes the deterministic pre-flight readiness checks of a project.
type PreflightReport struct {
	ProjectPath           string   `json:"project_path"`
	DetectedRuntimes      []string `json:"detected_runtimes"`
	MissingBinaries       []string `json:"missing_binaries"`
	InvalidCommands       []string `json:"invalid_commands"`
	RecommendedStrategy   string   `json:"recommended_strategy"` // "local", "docker", or "blocked"
	ReadyForLLMGeneration bool     `json:"ready_for_llm_generation"`
	IsDaemon              bool     `json:"is_daemon"`
	DetectedPorts         []int    `json:"detected_ports,omitempty"`
	PreemptedPorts        []int    `json:"preempted_ports,omitempty"`
	ReapedPIDs            []int    `json:"reaped_pids,omitempty"`
	PortPreflightError    string   `json:"port_preflight_error,omitempty"`
}

// PreflightSystem performs deterministic environment, toolchain, and command verification
// before passing execution or prompts to LLMs.
type PreflightSystem struct {
	lookPath       func(file string) (string, error)
	portReaper     *PortReaper
	containerGuard *ContainerTeardownGuard
}

// NewPreflightSystem initializes a new PreflightSystem.
func NewPreflightSystem() *PreflightSystem {
	reaper := NewPortReaper()
	return &PreflightSystem{
		lookPath:       exec.LookPath,
		portReaper:     reaper,
		containerGuard: NewContainerTeardownGuard(nil, reaper),
	}
}

// PreflightWorkspace inspects workspace manifests and validates required toolchains deterministically.
func (p *PreflightSystem) PreflightWorkspace(ctx context.Context, projectPath string, configuredCommands []string) (*PreflightReport, error) {
	report := &PreflightReport{
		ProjectPath:           projectPath,
		ReadyForLLMGeneration: true,
		RecommendedStrategy:   "local",
	}

	// For projects that run servers/daemons or have container definitions, container & port teardown is a MUST.
	if p.containerGuard == nil {
		p.containerGuard = NewContainerTeardownGuard(nil, p.portReaper)
	}
	composeFiles := p.containerGuard.DiscoverComposeFiles(projectPath)
	if IsDaemonProject(projectPath) || len(composeFiles) > 0 {
		report.IsDaemon = IsDaemonProject(projectPath)
		_ = p.containerGuard.PreFlightClean(ctx, projectPath)
		ports, _ := DetectProjectPorts(projectPath)
		if len(ports) > 0 {
			report.DetectedPorts = ports
			reaper := p.portReaper
			if reaper == nil {
				reaper = NewPortReaper()
			}
			preempted, reaped, pErr := reaper.ReapOrphanPortHolders(ctx, ports)
			report.PreemptedPorts = preempted
			report.ReapedPIDs = reaped
			if pErr != nil {
				report.PortPreflightError = pErr.Error()
				report.ReadyForLLMGeneration = false
			}
		}
	}

	requiredTools := make(map[string]bool)

	// Detect project manifests
	if p.hasFile(projectPath, "go.mod") {
		report.DetectedRuntimes = append(report.DetectedRuntimes, "go")
		requiredTools["go"] = true
	}
	if p.hasFile(projectPath, "Cargo.toml") {
		report.DetectedRuntimes = append(report.DetectedRuntimes, "rust")
		requiredTools["cargo"] = true
	}
	if p.hasFile(projectPath, "package.json") {
		report.DetectedRuntimes = append(report.DetectedRuntimes, "node")
		requiredTools["node"] = true
		requiredTools["npm"] = true
	}
	if p.hasFile(projectPath, "pyproject.toml") || p.hasFile(projectPath, "requirements.txt") || p.hasFile(projectPath, "setup.py") {
		report.DetectedRuntimes = append(report.DetectedRuntimes, "python")
		requiredTools["python3"] = true
	}
	if p.hasFile(projectPath, "Makefile") {
		requiredTools["make"] = true
	}

	// Verify binary presence
	for tool := range requiredTools {
		if _, err := p.lookPath(tool); err != nil {
			report.MissingBinaries = append(report.MissingBinaries, tool)
		}
	}

	// Preflight configured commands (e.g., make targets)
	for _, cmd := range configuredCommands {
		trimmed := strings.TrimSpace(cmd)
		if strings.HasPrefix(trimmed, "make ") {
			target := strings.TrimSpace(strings.TrimPrefix(trimmed, "make "))
			if target != "" && !p.hasMakefileTarget(projectPath, target) {
				report.InvalidCommands = append(report.InvalidCommands, fmt.Sprintf("command %q: target %q does not exist in Makefile", cmd, target))
			}
		}
	}

	if len(report.MissingBinaries) > 0 {
		// Check if docker is available as a fallback strategy
		if _, err := p.lookPath("docker"); err == nil {
			report.RecommendedStrategy = "docker"
		} else {
			report.RecommendedStrategy = "blocked"
			report.ReadyForLLMGeneration = false
		}
	}

	return report, nil
}

func (p *PreflightSystem) hasFile(projectPath, name string) bool {
	info, err := os.Stat(filepath.Join(projectPath, name))
	return err == nil && !info.IsDir()
}

func (p *PreflightSystem) hasMakefileTarget(projectPath, target string) bool {
	content, err := os.ReadFile(filepath.Join(projectPath, "Makefile"))
	if err != nil {
		return false
	}
	lines := strings.Split(string(content), "\n")
	prefix1 := target + ":"
	prefix2 := target + " :"
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, prefix1) || strings.HasPrefix(trimmed, prefix2) {
			return true
		}
	}
	return false
}
