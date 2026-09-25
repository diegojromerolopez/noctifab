package services

import (
	"context"
	"os"
	"path/filepath"
	"strings"
)

// RunDeterministicAutoFormat inspects the workspace projectPath and executes standard
// deterministic language formatters (gofmt, cargo fmt, ruff format, rubocop -A, prettier)
// locally in the sandbox without consuming LLM turns or tokens.
func RunDeterministicAutoFormat(ctx context.Context, runner Sandbox, projectPath string) {
	if runner == nil || projectPath == "" {
		return
	}

	cmds := detectDeterministicFormatters(projectPath)
	for _, cmd := range cmds {
		_, _ = runner.RunCommand(ctx, projectPath, cmd, "")
	}
}

func detectDeterministicFormatters(projectPath string) []string {
	if projectPath == "" {
		return nil
	}
	clean := filepath.Clean(projectPath)
	if clean == "." || clean == "/" || clean == "/tmp" || clean == "/var/tmp" || clean == filepath.Clean(os.TempDir()) {
		return nil
	}

	var cmds []string

	// 1. Go: gofmt
	if fileExists(filepath.Join(projectPath, "go.mod")) || hasExtInDir(projectPath, ".go") {
		cmds = append(cmds, "gofmt -w .")
	}

	// 2. Rust: cargo fmt
	if fileExists(filepath.Join(projectPath, "Cargo.toml")) || hasExtInDir(projectPath, ".rs") {
		cmds = append(cmds, "cargo fmt --all || true")
	}

	// 3. Python: ruff format / black
	if fileExists(filepath.Join(projectPath, "pyproject.toml")) || fileExists(filepath.Join(projectPath, "requirements.txt")) || fileExists(filepath.Join(projectPath, "setup.py")) || hasExtInDir(projectPath, ".py") {
		cmds = append(cmds, "ruff format . 2>/dev/null || black . 2>/dev/null || true")
	}

	// 4. Ruby: rubocop -A
	if fileExists(filepath.Join(projectPath, "Gemfile")) || fileExists(filepath.Join(projectPath, ".rubocop.yml")) || hasExtInDir(projectPath, ".rb") {
		cmds = append(cmds, "bundle exec rubocop -A 2>/dev/null || rubocop -A 2>/dev/null || true")
	}

	// 5. JavaScript / TypeScript: prettier
	if fileExists(filepath.Join(projectPath, "package.json")) || hasExtInDir(projectPath, ".ts") || hasExtInDir(projectPath, ".js") {
		cmds = append(cmds, "npx prettier --write . 2>/dev/null || prettier --write . 2>/dev/null || true")
	}

	// 6. C / C++: clang-format
	if hasExtInDir(projectPath, ".c") || hasExtInDir(projectPath, ".h") || hasExtInDir(projectPath, ".cpp") {
		cmds = append(cmds, "clang-format -i $(find . -maxdepth 4 -name '*.c' -o -name '*.h' -o -name '*.cpp' 2>/dev/null) 2>/dev/null || true")
	}

	return cmds
}

func fileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}

func hasExtInDir(projectPath, ext string) bool {
	found := false
	_ = filepath.Walk(projectPath, func(path string, info os.FileInfo, err error) error {
		if err != nil || found {
			return filepath.SkipDir
		}
		if info.IsDir() {
			name := info.Name()
			if strings.HasPrefix(name, ".") && name != "." {
				return filepath.SkipDir
			}
			if name == "target" || name == "node_modules" || name == "vendor" || name == "dist" || name == "build" || name == "output" || name == "log" || name == "report" {
				return filepath.SkipDir
			}
			rel, _ := filepath.Rel(projectPath, path)
			if strings.Count(rel, string(filepath.Separator)) > 3 {
				return filepath.SkipDir
			}
			return nil
		}
		if strings.EqualFold(filepath.Ext(path), ext) {
			found = true
			return filepath.SkipDir
		}
		return nil
	})
	return found
}
