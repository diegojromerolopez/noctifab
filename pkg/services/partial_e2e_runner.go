package services

import (
	"context"
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/diegojromerolopez/noctifab/pkg/domain"
)

// PartialStateE2ERunner executes targeted E2E checks against the subset of
// features or commands implemented so far in a partial state before allowing
// generation to proceed to downstream stories or tasks.
type PartialStateE2ERunner struct {
	Sandbox Sandbox
}

// NewPartialStateE2ERunner creates a new PartialStateE2ERunner.
func NewPartialStateE2ERunner(sandbox Sandbox) *PartialStateE2ERunner {
	return &PartialStateE2ERunner{Sandbox: sandbox}
}

// ExtractImplementedCommands identifies specific command tokens (e.g. "get", "set", "ping")
// implemented in the current task or story context.
func (p *PartialStateE2ERunner) ExtractImplementedCommands(state *domain.State, task domain.Task) []string {
	seen := make(map[string]bool)
	var commands []string

	add := func(cmd string) {
		cmd = strings.ToLower(strings.TrimSpace(cmd))
		// Standard commands are 2 to 12 alphanumeric characters and not generic stop words
		if len(cmd) >= 2 && len(cmd) <= 12 && !isGenericStopWord(cmd) && !seen[cmd] {
			seen[cmd] = true
			commands = append(commands, cmd)
		}
	}

	// Extract from task title
	for _, word := range strings.Fields(task.Title) {
		clean := strings.ToLower(strings.Trim(word, ":;,.-_()[]{}'\"/\\"))
		if clean == "get" || clean == "set" || clean == "hset" || clean == "hget" ||
			clean == "del" || clean == "ping" || clean == "put" || clean == "post" ||
			clean == "delete" || clean == "head" || clean == "lpush" || clean == "rpush" ||
			clean == "sadd" || clean == "zadd" || clean == "health" || clean == "version" ||
			clean == "help" || clean == "echo" || clean == "info" || clean == "status" || clean == "smoke" {
			add(clean)
		}
	}

	// Extract from target file stems
	for _, tf := range task.TargetFiles {
		base := filepath.Base(tf)
		stem := strings.ToLower(strings.TrimSuffix(base, filepath.Ext(base)))
		stem = strings.TrimPrefix(stem, "test_")
		stem = strings.TrimSuffix(stem, "_test")
		if stem == "get" || stem == "set" || stem == "hset" || stem == "hget" ||
			stem == "del" || stem == "ping" || stem == "put" || stem == "route" ||
			stem == "health" || stem == "version" || stem == "help" || stem == "smoke" {
			add(stem)
		}
	}

	// First Partial Deliverable Mandate (US-001):
	// If no specific commands were matched but this is US-001 (walking skeleton),
	// target foundational smoke, health, ping, help, or version capabilities.
	if len(commands) == 0 {
		isFirstStory := false
		if state != nil {
			fn := strings.ToLower(state.Metadata.FeatureName)
			ip := strings.ToLower(state.Metadata.InputPath)
			if strings.Contains(fn, "us-001") || strings.Contains(fn, "us-1") || strings.Contains(fn, "walking-skeleton") ||
				strings.Contains(ip, "us-001") || strings.Contains(ip, "us-1") {
				isFirstStory = true
			}
		}
		sID := strings.ToLower(task.StoryID)
		if strings.Contains(sID, "us-001") || strings.Contains(sID, "us-1") || sID == "story-0001" {
			isFirstStory = true
		}
		if isFirstStory {
			add("ping")
			add("health")
			add("smoke")
			add("help")
			add("version")
		}
	}

	return commands
}

// BuildTargetedE2ECommand synthesizes a targeted command filter for partial-state execution
// based on the detected runtime framework (e.g. pytest -k, go test -run, cargo test).
func (p *PartialStateE2ERunner) BuildTargetedE2ECommand(baseCmd string, commands []string) string {
	if len(commands) == 0 {
		return baseCmd
	}

	cmdLower := strings.ToLower(baseCmd)
	joinedFilter := strings.Join(commands, " or ")

	if strings.Contains(cmdLower, "pytest") {
		// Append -k filter if not already present
		if !strings.Contains(baseCmd, " -k ") {
			return fmt.Sprintf("%s -k %q", baseCmd, joinedFilter)
		}
	} else if strings.Contains(cmdLower, "go test") {
		regexPattern := fmt.Sprintf("Test.*(%s)", strings.Join(commands, "|"))
		if !strings.Contains(baseCmd, " -run ") {
			return fmt.Sprintf("%s -run %q", baseCmd, regexPattern)
		}
	}

	return baseCmd
}

// ValidatePartialE2E executes targeted E2E tests for the currently implemented commands.
// It verifies that in-scope tests actually ran and succeeded.
func (p *PartialStateE2ERunner) ValidatePartialE2E(ctx context.Context, projectPath string, baseCmd string, state *domain.State, task domain.Task) (bool, string, error) {
	if p.Sandbox == nil || baseCmd == "" {
		return true, "", nil
	}

	commands := p.ExtractImplementedCommands(state, task)
	if len(commands) == 0 {
		// No specific isolated commands to verify in partial state
		return true, "", nil
	}

	targetedCmd := p.BuildTargetedE2ECommand(baseCmd, commands)
	out, err := p.Sandbox.RunCommand(ctx, projectPath, targetedCmd, "")
	if err != nil {
		// Check if failure was strictly in-scope for the active commands
		if isE2EFailureInScope(state, task, out) {
			return false, fmt.Sprintf(
				"Partial-state E2E test failed for implemented command(s) [%s]:\nCommand: %s\nOutput:\n%s\nError: %v",
				strings.Join(commands, ", "), targetedCmd, out, err,
			), nil
		}
		// Failure outside of active commands is ignored in partial state
		return true, "", nil
	}

	// Verify that at least one test was executed
	noTestsRan, notice := EvaluateE2ETestExecution(out)
	if noTestsRan {
		return false, fmt.Sprintf(
			"Partial-state E2E verification failed: implemented commands [%s] had 0 passing E2E tests executed: %s",
			strings.Join(commands, ", "), notice,
		), nil
	}

	return true, "", nil
}

// GenerateDynamicRoundTripNonce produces a non-hardcoded key and value pair
// guaranteed to be unique and dynamic for live application round-trip verification.
func GenerateDynamicRoundTripNonce() (key string, value string) {
	seed := time.Now().UnixNano() + rand.Int63()
	key = fmt.Sprintf("k_%x", seed%1000000)
	value = fmt.Sprintf("v_%x_%x", seed, rand.Int63())
	return key, value
}

// VerifyStoryAddedE2ETests checks whether the workspace's E2E test suite (e.g. tests/e2e/, tests/integration/)
// contains test coverage for the commands/features implemented in the active user story.
func (p *PartialStateE2ERunner) VerifyStoryAddedE2ETests(projectPath string, state *domain.State, commands []string) (bool, string) {
	if len(commands) == 0 {
		return true, ""
	}

	testFiles := collectE2ETestFiles(projectPath)
	if len(testFiles) == 0 {
		return false, fmt.Sprintf(
			"Mandatory Partial Deliverability E2E failure: No E2E test files found in workspace (e.g. under tests/e2e/ or tests/integration/) to verify implemented command(s) [%s]. The user story must author black-box E2E tests.",
			strings.Join(commands, ", "),
		)
	}

	var combinedContent strings.Builder
	for _, tf := range testFiles {
		if data, err := os.ReadFile(tf); err == nil {
			combinedContent.WriteString(strings.ToLower(string(data)))
			combinedContent.WriteString("\n")
		}
	}
	allE2EText := combinedContent.String()

	// Check if this is the generic US-001 fallback set
	isFallbackSet := len(commands) == 5 &&
		containsAll(commands, []string{"ping", "health", "smoke", "help", "version"})

	if isFallbackSet {
		// For US-001 fallback, an E2E harness (docker-compose, Makefile e2e, or tests/e2e)
		// or at least one smoke/health/ping/help/version assertion must be present
		foundAny := false
		for _, cmd := range commands {
			if strings.Contains(allE2EText, cmd) {
				foundAny = true
				break
			}
		}
		if !foundAny {
			for _, tf := range testFiles {
				base := filepath.Base(tf)
				if strings.Contains(base, "docker-compose") || strings.Contains(allE2EText, "test:") || strings.Contains(allE2EText, "e2e:") {
					foundAny = true
					break
				}
			}
		}
		if !foundAny {
			return false, "Mandatory Partial Deliverability E2E failure: US-001 did not add any black-box E2E smoke tests (expected test coverage for ping, health, smoke, help, or version in tests/e2e/ or tests/integration/)."
		}
		return true, ""
	}

	// For explicitly implemented commands, ensure each is referenced in the E2E test suite
	var missing []string
	for _, cmd := range commands {
		if !strings.Contains(allE2EText, cmd) {
			missing = append(missing, cmd)
		}
	}

	if len(missing) > 0 {
		return false, fmt.Sprintf(
			"Mandatory Partial Deliverability E2E failure: User story implemented command(s) [%s], but no corresponding black-box E2E test cases were found in the E2E test suite.",
			strings.Join(missing, ", "),
		)
	}

	return true, ""
}

func collectE2ETestFiles(projectPath string) []string {
	var files []string
	candidates := []string{
		filepath.Join(projectPath, "tests", "e2e"),
		filepath.Join(projectPath, "test", "e2e"),
		filepath.Join(projectPath, "tests", "integration"),
		filepath.Join(projectPath, "test", "integration"),
	}
	for _, dir := range candidates {
		if fi, err := os.Stat(dir); err == nil && fi.IsDir() {
			_ = filepath.Walk(dir, func(p string, info os.FileInfo, err error) error {
				if err == nil && !info.IsDir() {
					ext := strings.ToLower(filepath.Ext(p))
					if ext == ".sh" || ext == ".py" || ext == ".go" || ext == ".rs" ||
						ext == ".c" || ext == ".js" || ext == ".ts" || ext == ".rb" {
						files = append(files, p)
					}
				}
				return nil
			})
		}
	}
	// Also inspect top-level tests/ directory for test scripts or blackbox runners
	testsDir := filepath.Join(projectPath, "tests")
	if entries, err := os.ReadDir(testsDir); err == nil {
		for _, e := range entries {
			if !e.IsDir() {
				name := strings.ToLower(e.Name())
				if strings.Contains(name, "e2e") || strings.Contains(name, "smoke") || strings.Contains(name, "blackbox") {
					files = append(files, filepath.Join(testsDir, e.Name()))
				}
			}
		}
	}
	// Check Makefile for inline e2e recipes
	makefilePath := filepath.Join(projectPath, "Makefile")
	if _, err := os.Stat(makefilePath); err == nil {
		files = append(files, makefilePath)
	}
	// Check docker-compose files
	for _, comp := range []string{"docker-compose.e2e.yml", "docker-compose.yml"} {
		cp := filepath.Join(projectPath, comp)
		if _, err := os.Stat(cp); err == nil {
			files = append(files, cp)
		}
	}
	return files
}

func containsAll(slice []string, items []string) bool {
	m := make(map[string]bool)
	for _, s := range slice {
		m[s] = true
	}
	for _, it := range items {
		if !m[it] {
			return false
		}
	}
	return true
}
