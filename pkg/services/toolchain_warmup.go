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

// WarmupCommandForDir inspects a workspace directory and returns a dry-run/prefetch
// toolchain compilation command that downloads and indexes dependencies in the background
// without mutating code.
func WarmupCommandForDir(dir string) (string, []string) {
	// 1. Rust
	if _, err := os.Stat(filepath.Join(dir, "Cargo.toml")); err == nil {
		return "cargo", []string{"check", "--tests"}
	}
	// 2. Go
	if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
		return "go", []string{"test", "-run=^$", "./..."}
	}
	// 3. Python (pytest)
	if _, err := os.Stat(filepath.Join(dir, "pyproject.toml")); err == nil {
		return "python3", []string{"-m", "pytest", "--collect-only", "-q"}
	}
	if _, err := os.Stat(filepath.Join(dir, "setup.py")); err == nil {
		return "python3", []string{"-m", "pytest", "--collect-only", "-q"}
	}
	// 4. .NET
	if matches, err := filepath.Glob(filepath.Join(dir, "*.csproj")); err == nil && len(matches) > 0 {
		return "dotnet", []string{"build", "--no-incremental"}
	}
	if matches, err := filepath.Glob(filepath.Join(dir, "*.sln")); err == nil && len(matches) > 0 {
		return "dotnet", []string{"build", "--no-incremental"}
	}
	// 5. Node.js
	if _, err := os.Stat(filepath.Join(dir, "package.json")); err == nil {
		return "npm", []string{"test", "--", "--listTests"}
	}

	return "", nil
}

// WarmupWorktreeToolchain starts a non-blocking background pre-compilation routine
// inside the target worktree. It overlaps toolchain dependency fetching/indexing
// with LLM reasoning, eliminating cold compiler latency on the first test turn.
func WarmupWorktreeToolchain(ctx context.Context, worktreeDir string) <-chan struct{} {
	done := make(chan struct{})

	bin, args := WarmupCommandForDir(worktreeDir)
	if bin == "" {
		// Try resolving manifest in immediate subdirectories
		entries, err := os.ReadDir(worktreeDir)
		if err == nil {
			for _, entry := range entries {
				if entry.IsDir() && !strings.HasPrefix(entry.Name(), ".") {
					subDir := filepath.Join(worktreeDir, entry.Name())
					if subBin, subArgs := WarmupCommandForDir(subDir); subBin != "" {
						bin, args = subBin, subArgs
						worktreeDir = subDir
						break
					}
				}
			}
		}
	}

	if bin == "" {
		close(done)
		return done
	}

	go func() {
		defer close(done)

		// Create a bounded context (max 30s) so warmup never hangs or leaks
		warmupCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
		defer cancel()

		cmd := exec.CommandContext(warmupCtx, bin, args...)
		cmd.Dir = worktreeDir
		rootProjectDir := ResolveRootProjectDir(worktreeDir)
		cmd.Env = append(os.Environ(), BuildSharedCacheEnv(rootProjectDir)...)

		fmt.Printf("⚡ [Toolchain JIT Warmup] Pre-warming dependencies in background via %s %s...\n", bin, strings.Join(args, " "))
		_ = cmd.Run()
		fmt.Printf("✔ [Toolchain JIT Warmup] Dependency pre-warming completed for %s\n", worktreeDir)
	}()

	return done
}
