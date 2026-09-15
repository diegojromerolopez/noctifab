package services

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// DetectE2ECommand determines the appropriate E2E command based on the mode,
// custom override, and workspace file layout.
// Supported modes:
//   - "docker" (default): Checks for docker-compose.e2e.yml or docker-compose.yml.
//   - "native": Checks for local test suites (Makefile e2e target, npm e2e scripts,
//     tests/e2e directories for python/go/rust, or fallback test targets) without Docker.
func DetectE2ECommand(projectPath, mode, customCmd string) string {
	if trimmed := strings.TrimSpace(customCmd); trimmed != "" {
		return trimmed
	}

	mode = strings.ToLower(strings.TrimSpace(mode))
	if mode == "" {
		mode = "docker"
	}

	if mode == "native" {
		return detectNativeE2ECommand(projectPath)
	}

	return detectDockerE2ECommand(projectPath)
}

func detectDockerE2ECommand(projectPath string) string {
	if _, err := os.Stat(filepath.Join(projectPath, "docker-compose.e2e.yml")); err == nil {
		return "docker compose -f docker-compose.e2e.yml up --build --exit-code-from test-runner"
	}
	if _, err := os.Stat(filepath.Join(projectPath, "docker-compose.yml")); err == nil {
		if content, rErr := os.ReadFile(filepath.Join(projectPath, "docker-compose.yml")); rErr == nil && strings.Contains(string(content), "e2e:") {
			return "docker compose up --build --exit-code-from e2e"
		}
	}
	// Fallback to Makefile e2e target if no docker-compose file is present
	if _, err := os.Stat(filepath.Join(projectPath, "Makefile")); err == nil {
		content, rErr := os.ReadFile(filepath.Join(projectPath, "Makefile"))
		if rErr == nil && strings.Contains(string(content), "e2e:") {
			return "make e2e"
		}
	}
	return ""
}

func detectNativeE2ECommand(projectPath string) string {
	// 1. Explicit Makefile e2e target has highest priority for native execution
	if _, err := os.Stat(filepath.Join(projectPath, "Makefile")); err == nil {
		content, rErr := os.ReadFile(filepath.Join(projectPath, "Makefile"))
		if rErr == nil && strings.Contains(string(content), "e2e:") {
			return "make e2e"
		}
	}

	// 2. Node/JS e2e script in package.json
	if _, err := os.Stat(filepath.Join(projectPath, "package.json")); err == nil {
		if content, rErr := os.ReadFile(filepath.Join(projectPath, "package.json")); rErr == nil {
			str := string(content)
			if strings.Contains(str, `"test:e2e"`) {
				return "npm run test:e2e"
			}
			if strings.Contains(str, `"e2e"`) {
				return "npm run e2e"
			}
		}
	}

	// 3. Python e2e tests
	hasTestsE2E := dirExists(filepath.Join(projectPath, "tests", "e2e"))
	hasTestE2E := dirExists(filepath.Join(projectPath, "test", "e2e"))
	if hasTestsE2E || hasTestE2E {
		if fileExists(filepath.Join(projectPath, "pyproject.toml")) ||
			fileExists(filepath.Join(projectPath, "requirements.txt")) ||
			fileExists(filepath.Join(projectPath, "setup.py")) {
			subPath := "tests/e2e"
			if !hasTestsE2E && hasTestE2E {
				subPath = "test/e2e"
			}
			if fileExists(filepath.Join(projectPath, "pyproject.toml")) {
				if _, err := exec.LookPath("uv"); err == nil {
					return "uv run pytest " + subPath
				}
			}
			return "pytest " + subPath
		}

		// 4. Rust e2e tests
		if fileExists(filepath.Join(projectPath, "Cargo.toml")) {
			return "cargo test --test e2e"
		}

		// 5. Go e2e tests
		if fileExists(filepath.Join(projectPath, "go.mod")) {
			if hasTestsE2E {
				return "go test -v ./tests/e2e/..."
			}
			return "go test -v ./test/e2e/..."
		}
	}

	// 6. Rust integration test file tests/e2e.rs
	if fileExists(filepath.Join(projectPath, "Cargo.toml")) && fileExists(filepath.Join(projectPath, "tests", "e2e.rs")) {
		return "cargo test --test e2e"
	}

	// 7. Makefile test target fallback
	if _, err := os.Stat(filepath.Join(projectPath, "Makefile")); err == nil {
		content, rErr := os.ReadFile(filepath.Join(projectPath, "Makefile"))
		if rErr == nil && strings.Contains(string(content), "test:") {
			return "make test"
		}
	}

	return ""
}

func dirExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}
