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
		content, _ := os.ReadFile(filepath.Join(projectPath, "docker-compose.e2e.yml"))
		service := detectTestRunnerService(string(content))
		if service == "" {
			service = "test-runner"
		}
		return "docker compose -f docker-compose.e2e.yml up --build --exit-code-from " + service
	}
	if _, err := os.Stat(filepath.Join(projectPath, "docker-compose.yml")); err == nil {
		if content, rErr := os.ReadFile(filepath.Join(projectPath, "docker-compose.yml")); rErr == nil {
			service := detectTestRunnerService(string(content))
			if service != "" {
				return "docker compose up --build --exit-code-from " + service
			}
			if strings.Contains(string(content), "e2e:") {
				return "docker compose up --build --exit-code-from e2e"
			}
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

// detectTestRunnerService parses top-level service names from docker-compose YAML content
// and returns the best candidate for the test-runner container.
func detectTestRunnerService(content string) string {
	if strings.TrimSpace(content) == "" {
		return ""
	}
	services := extractComposeServices(content)
	if len(services) == 0 {
		return ""
	}

	// 1. Exact priority matches
	priorities := []string{
		"test-runner-e2e",
		"test-runner",
		"test_runner",
		"test-client",
		"e2e-runner",
		"e2e",
		"tests",
		"test",
	}
	for _, p := range priorities {
		for _, s := range services {
			if strings.EqualFold(s, p) {
				return s
			}
		}
	}

	// 2. Substring matches for runner / e2e / test (excluding server / backend)
	for _, s := range services {
		lower := strings.ToLower(s)
		if strings.Contains(lower, "server") || strings.Contains(lower, "backend") || strings.Contains(lower, "db") || strings.Contains(lower, "redis") {
			continue
		}
		if strings.Contains(lower, "runner") || strings.Contains(lower, "e2e") || strings.Contains(lower, "test") {
			return s
		}
	}

	return ""
}

// extractComposeServices extracts service names directly under the `services:` block in a docker-compose YAML.
func extractComposeServices(content string) []string {
	var services []string
	lines := strings.Split(content, "\n")
	inServices := false
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "#") || trimmed == "" {
			continue
		}
		if trimmed == "services:" {
			inServices = true
			continue
		}
		if inServices {
			// Top-level key under root (no indent) ends services block
			if len(line) > 0 && !strings.HasPrefix(line, " ") && !strings.HasPrefix(line, "\t") {
				break
			}
			// Look for 2-space or single-tab indented service name followed by a colon
			if (strings.HasPrefix(line, "  ") && !strings.HasPrefix(line, "   ")) ||
				(strings.HasPrefix(line, "\t") && !strings.HasPrefix(line, "\t\t") && !strings.HasPrefix(line, "\t ")) {
				colonIdx := strings.Index(trimmed, ":")
				if colonIdx > 0 {
					serviceName := strings.TrimSpace(trimmed[:colonIdx])
					if !strings.Contains(serviceName, " ") && !strings.Contains(serviceName, "$") {
						services = append(services, serviceName)
					}
				}
			}
		}
	}
	return services
}

func detectNativeE2ECommand(projectPath string) string {
	// 1. Explicit Makefile e2e target has highest priority for native execution,
	// unless its recipe explicitly invokes docker.
	if _, err := os.Stat(filepath.Join(projectPath, "Makefile")); err == nil {
		content, rErr := os.ReadFile(filepath.Join(projectPath, "Makefile"))
		if rErr == nil && strings.Contains(string(content), "e2e:") && isMakefileE2ENative(string(content)) {
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

func isMakefileE2ENative(content string) bool {
	lines := strings.Split(content, "\n")
	inE2E := false
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "e2e:") {
			inE2E = true
			continue
		}
		if inE2E {
			if strings.HasPrefix(line, "\t") || strings.HasPrefix(line, " ") {
				if strings.Contains(trimmed, "docker") {
					return false
				}
			} else if trimmed != "" && !strings.HasPrefix(trimmed, "#") {
				break
			}
		}
	}
	return inE2E
}
