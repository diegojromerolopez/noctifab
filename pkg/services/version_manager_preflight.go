package services

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// Standard toolchain and version manager directories on Unix/macOS systems.
var standardVersionManagerDirs = []string{
	".asdf/shims",
	".cargo/bin",
	".local/bin",
	".pyenv/shims",
	".rbenv/shims",
	".nodenv/shims",
}

var standardSystemBinDirs = []string{
	"/opt/homebrew/bin",
	"/usr/local/bin",
	"/usr/bin",
	"/bin",
}

// VersionManagerResolver discovers and resolves host toolchains and version managers.
type VersionManagerResolver struct {
	HomeDir string
}

// NewVersionManagerResolver creates a new resolver for the current user's home directory.
func NewVersionManagerResolver() *VersionManagerResolver {
	home, _ := os.UserHomeDir()
	return &VersionManagerResolver{HomeDir: home}
}

// DiscoverVersionManagerPaths returns existing version manager directories on the host.
func (r *VersionManagerResolver) DiscoverVersionManagerPaths() []string {
	var paths []string
	if r.HomeDir != "" {
		for _, rel := range standardVersionManagerDirs {
			p := filepath.Join(r.HomeDir, rel)
			if info, err := os.Stat(p); err == nil && info.IsDir() {
				paths = append(paths, p)
			}
		}
	}
	for _, p := range standardSystemBinDirs {
		if info, err := os.Stat(p); err == nil && info.IsDir() {
			paths = append(paths, p)
		}
	}
	return paths
}

// ResolveBinary looks up a binary on $PATH, falling back to version manager directories.
// If found in a version manager directory, it prepends that directory to process $PATH.
func (r *VersionManagerResolver) ResolveBinary(binary string) (string, error) {
	cleanBin := strings.TrimSpace(binary)
	if cleanBin == "" {
		return "", fmt.Errorf("empty binary name")
	}

	// 1. Direct LookPath on active PATH
	if p, err := exec.LookPath(cleanBin); err == nil {
		return p, nil
	}

	// 2. Scan discovered version manager directories
	for _, dir := range r.DiscoverVersionManagerPaths() {
		candidate := filepath.Join(dir, cleanBin)
		if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
			// Ensure executable bit on Unix
			if info.Mode()&0111 != 0 || strings.HasSuffix(candidate, ".exe") {
				// Augment active process PATH so all downstream child commands find it
				currentPath := os.Getenv("PATH")
				if !strings.Contains(currentPath, dir) {
					_ = os.Setenv("PATH", dir+string(os.PathListSeparator)+currentPath)
				}
				fmt.Printf("ℹ [Version Manager Preflight] Resolved binary %q via %s\n", cleanBin, candidate)
				return candidate, nil
			}
		}
	}

	return "", fmt.Errorf("binary %q not found on PATH or in version manager directories", cleanBin)
}

// EnsureVenvTestRunner checks if the required test runner (e.g. pytest) is installed in project's .venv.
// If missing, it invokes uv pip install or python -m pip install to automatically pre-install it.
func EnsureVenvTestRunner(projectPath, runnerBin string) error {
	runnerClean := strings.ToLower(strings.TrimSpace(runnerBin))
	if runnerClean != "pytest" && runnerClean != "tox" && runnerClean != "flake8" && runnerClean != "mypy" {
		return nil
	}

	for _, venvName := range []string{".venv", "venv"} {
		venvDir := filepath.Join(projectPath, venvName)
		if info, err := os.Stat(venvDir); err == nil && info.IsDir() {
			runnerPath := filepath.Join(venvDir, "bin", runnerClean)
			if _, rErr := os.Stat(runnerPath); rErr == nil {
				// Already installed in venv
				return nil
			}

			fmt.Printf("⚡ [Venv Preflight] Required test runner %q missing in %s; auto-installing...\n", runnerClean, venvName)
			// Prefer uv if installed
			if uvPath, err := exec.LookPath("uv"); err == nil {
				cmd := exec.Command(uvPath, "pip", "install", runnerClean, "--python", filepath.Join(venvDir, "bin", "python"))
				cmd.Dir = projectPath
				if out, err := cmd.CombinedOutput(); err == nil {
					fmt.Printf("✔ [Venv Preflight] Installed %s via uv into %s\n", runnerClean, venvName)
					return nil
				} else {
					fmt.Printf("⚠ [Venv Preflight] uv install failed (%v): %s; falling back to python -m pip\n", err, string(out))
				}
			}

			// Fallback to python -m pip install
			pythonBin := filepath.Join(venvDir, "bin", "python")
			if _, pErr := os.Stat(pythonBin); pErr == nil {
				cmd := exec.Command(pythonBin, "-m", "pip", "install", runnerClean)
				cmd.Dir = projectPath
				if out, err := cmd.CombinedOutput(); err == nil {
					fmt.Printf("✔ [Venv Preflight] Installed %s via pip into %s\n", runnerClean, venvName)
					return nil
				} else {
					return fmt.Errorf("failed to install %s into %s: %w (output: %s)", runnerClean, venvName, err, string(out))
				}
			}
		}
	}
	return nil
}
