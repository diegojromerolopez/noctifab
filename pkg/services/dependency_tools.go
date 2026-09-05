package services

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/diegojromerolopez/noctifab/pkg/domain"
)

// InstallPackageTool provides package installation capabilities for agents (e.g. Last-Resort Agent).
type InstallPackageTool struct {
	DepMgr *DependencyManager
	Runner Sandbox
}

func (t *InstallPackageTool) Name() string { return "install_package" }
func (t *InstallPackageTool) Description() string {
	return "install_package installs a package or toolchain dependency. Arguments: package (string), manager (optional string)."
}
func (t *InstallPackageTool) Execute(ctx context.Context, state *domain.State, args map[string]any) (string, error) {
	pkg, ok := args["package"].(string)
	if !ok || strings.TrimSpace(pkg) == "" {
		return "", errors.New("missing or invalid 'package' argument")
	}
	pkg = strings.TrimSpace(pkg)
	manager, _ := args["manager"].(string)
	manager = strings.TrimSpace(manager)

	if t.DepMgr != nil {
		if err := t.DepMgr.InstallTool(ctx, pkg); err == nil {
			return fmt.Sprintf("Successfully installed package %s", pkg), nil
		}
	}

	if t.Runner != nil {
		if manager == "" {
			manager = detectPackageManager(state.ProjectPath, pkg)
		}

		cmdStr := pkg
		if manager != "" {
			switch strings.ToLower(manager) {
			case "pip":
				cmdStr = "pip install " + pkg
			case "npm":
				cmdStr = "npm install -g " + pkg
			case "gem":
				cmdStr = "gem install " + pkg
			case "opam":
				cmdStr = "opam install -y " + pkg
			case "cargo":
				cmdStr = "cargo install " + pkg
			case "go":
				cmdStr = "go install " + pkg
			case "apt", "apt-get":
				cmdStr = "apt-get update && apt-get install -y " + pkg
			case "apk":
				cmdStr = "apk add --no-cache " + pkg
			}
		}
		out, err := t.Runner.RunCommand(ctx, state.ProjectPath, cmdStr, "")
		if err != nil {
			return fmt.Sprintf("Installation failed: %v\nOutput: %s", err, out), err
		}
		return fmt.Sprintf("Package %s installed successfully.\nOutput:\n%s", pkg, out), nil
	}

	return "", errors.New("no package manager or sandbox runner configured")
}

func detectPackageManager(projectPath, pkg string) string {
	if entry, ok := toolPackageMap[strings.ToLower(pkg)]; ok && entry.Manager != "" {
		return entry.Manager
	}
	if projectPath == "" {
		return ""
	}
	if _, err := os.Stat(filepath.Join(projectPath, "package.json")); err == nil {
		return "npm"
	}
	if _, err := os.Stat(filepath.Join(projectPath, "pyproject.toml")); err == nil {
		return "pip"
	}
	if _, err := os.Stat(filepath.Join(projectPath, "requirements.txt")); err == nil {
		return "pip"
	}
	if _, err := os.Stat(filepath.Join(projectPath, "Gemfile")); err == nil {
		return "gem"
	}
	if _, err := os.Stat(filepath.Join(projectPath, "Cargo.toml")); err == nil {
		return "cargo"
	}
	if _, err := os.Stat(filepath.Join(projectPath, "go.mod")); err == nil {
		return "go"
	}
	if _, err := os.Stat(filepath.Join(projectPath, "dune-project")); err == nil {
		return "opam"
	}
	return ""
}
