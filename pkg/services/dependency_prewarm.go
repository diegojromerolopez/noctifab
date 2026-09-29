package services

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/diegojromerolopez/noctifab/pkg/infrastructure/telemetry"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

// PreWarmResult contains the outcome of a background cache pre-warm operation.
type PreWarmResult struct {
	Manifest string        `json:"manifest"`
	Manager  string        `json:"manager"`
	Command  string        `json:"command"`
	Duration time.Duration `json:"duration"`
	Output   string        `json:"output"`
	Error    string        `json:"error,omitempty"`
}

// DetectManifests scans the project root for package/toolchain manifests.
func DetectManifests(projectPath string) []string {
	if projectPath == "" {
		return nil
	}
	candidates := []string{
		"go.mod",
		"Cargo.toml",
		"package.json",
		"requirements.txt",
		"pyproject.toml",
		"Gemfile",
		"dune-project",
	}
	var detected []string
	for _, c := range candidates {
		full := filepath.Join(projectPath, c)
		if info, err := os.Stat(full); err == nil && !info.IsDir() {
			detected = append(detected, c)
		}
	}
	return detected
}

// GetPreWarmCommand determines the package manager and cache pre-warming command for a manifest.
func GetPreWarmCommand(manifest, projectPath string) (manager string, cmdStr string) {
	switch manifest {
	case "go.mod":
		return "go", "go mod download"
	case "Cargo.toml":
		return "cargo", "cargo fetch"
	case "package.json":
		return "npm", "npm install --package-lock-only"
	case "requirements.txt":
		venvPip := filepath.Join(projectPath, ".venv", "bin", "pip")
		if _, err := os.Stat(venvPip); err == nil {
			return "pip", venvPip + " download -r requirements.txt"
		}
		return "pip", "pip download -r requirements.txt"
	case "pyproject.toml":
		venvPip := filepath.Join(projectPath, ".venv", "bin", "pip")
		if _, err := os.Stat(venvPip); err == nil {
			return "pip", venvPip + " install --dry-run ."
		}
		return "pip", "pip install --dry-run ."
	case "Gemfile":
		return "gem", "bundle package"
	case "dune-project":
		return "opam", "dune external-lib-deps --missing @install"
	default:
		return "", ""
	}
}

// PreWarmDependencies proactively warms toolchain and package dependency caches for detected manifests.
func (dm *DependencyManager) PreWarmDependencies(ctx context.Context, projectPath string, runner Sandbox) ([]PreWarmResult, error) {
	if runner == nil || projectPath == "" {
		return nil, nil
	}

	ctx, span := telemetry.Tracer().Start(ctx, "DependencyManager.PreWarmDependencies",
		trace.WithAttributes(attribute.String("project_path", projectPath)))
	defer span.End()

	manifests := DetectManifests(projectPath)
	if len(manifests) == 0 {
		return nil, nil
	}

	type workItem struct {
		manifest string
		manager  string
		cmd      string
	}
	var items []workItem
	for _, m := range manifests {
		mgr, cmd := GetPreWarmCommand(m, projectPath)
		if cmd == "" {
			continue
		}
		if dm != nil && len(dm.AllowedPkgManagers) > 0 && !dm.IsAllowed(mgr) {
			continue
		}
		items = append(items, workItem{manifest: m, manager: mgr, cmd: cmd})
	}

	if len(items) == 0 {
		return nil, nil
	}

	results := make([]PreWarmResult, len(items))
	var wg sync.WaitGroup
	wg.Add(len(items))

	for i, item := range items {
		go func(idx int, w workItem) {
			defer wg.Done()
			start := time.Now()
			out, err := runner.RunCommand(ctx, projectPath, w.cmd, "")
			duration := time.Since(start)

			res := PreWarmResult{
				Manifest: w.manifest,
				Manager:  w.manager,
				Command:  w.cmd,
				Duration: duration,
				Output:   strings.TrimSpace(out),
			}
			if err != nil {
				res.Error = err.Error()
			}
			results[idx] = res
		}(i, item)
	}

	wg.Wait()
	return results, nil
}

// StartBackgroundPreWarm launches dependency pre-warming in a detached background goroutine,
// returning a channel that receives the results upon completion without blocking the caller.
func (dm *DependencyManager) StartBackgroundPreWarm(ctx context.Context, projectPath string, runner Sandbox) <-chan []PreWarmResult {
	resCh := make(chan []PreWarmResult, 1)
	go func() {
		defer close(resCh)
		results, _ := dm.PreWarmDependencies(ctx, projectPath, runner)
		resCh <- results
	}()
	return resCh
}
