package config

import (
	"reflect"
	"testing"
	"time"

	"gopkg.in/yaml.v3"
)

func TestDefaultConfig_Exhaustive(t *testing.T) {
	cfg := DefaultConfig()
	if cfg == nil {
		t.Fatal("DefaultConfig() returned nil")
	}

	// 1. Root & Runtime
	if cfg.ConfigVersion != "2.0" {
		t.Errorf("expected ConfigVersion '2.0', got %q", cfg.ConfigVersion)
	}
	if cfg.Runtime.SpecSource != "" {
		t.Errorf("expected Runtime.SpecSource '', got %q", cfg.Runtime.SpecSource)
	}
	if cfg.Runtime.MaxActions != 100 {
		t.Errorf("expected Runtime.MaxActions 100, got %d", cfg.Runtime.MaxActions)
	}
	if time.Duration(cfg.Runtime.MaxDuration) != 0 {
		t.Errorf("expected Runtime.MaxDuration 0, got %v", time.Duration(cfg.Runtime.MaxDuration))
	}
	if time.Duration(cfg.Runtime.MaxSilentStallDuration) != 30*time.Minute {
		t.Errorf("expected Runtime.MaxSilentStallDuration 30m, got %v", time.Duration(cfg.Runtime.MaxSilentStallDuration))
	}
	if cfg.Runtime.MaxTokensPerStory != 0 {
		t.Errorf("expected Runtime.MaxTokensPerStory 0, got %d", cfg.Runtime.MaxTokensPerStory)
	}
	if cfg.Runtime.MaxTokensPerTask != 0 {
		t.Errorf("expected Runtime.MaxTokensPerTask 0, got %d", cfg.Runtime.MaxTokensPerTask)
	}

	// 2. Logging
	if cfg.Logging.Level != "info" {
		t.Errorf("expected Logging.Level 'info', got %q", cfg.Logging.Level)
	}
	if cfg.Logging.File != "" {
		t.Errorf("expected Logging.File '', got %q", cfg.Logging.File)
	}

	// 3. Storage & OCC
	if cfg.Storage.Provider != "sqlite" {
		t.Errorf("expected Storage.Provider 'sqlite', got %q", cfg.Storage.Provider)
	}
	if cfg.Storage.ConnString != ".noctifab/data/noctifab.db" {
		t.Errorf("expected Storage.ConnString '.noctifab/data/noctifab.db', got %q", cfg.Storage.ConnString)
	}
	if cfg.Storage.JSONFilePath != ".noctifab/data/state.json" {
		t.Errorf("expected Storage.JSONFilePath '.noctifab/data/state.json', got %q", cfg.Storage.JSONFilePath)
	}
	if cfg.Storage.OCC.MaxRetries != 20 {
		t.Errorf("expected Storage.OCC.MaxRetries 20, got %d", cfg.Storage.OCC.MaxRetries)
	}
	if time.Duration(cfg.Storage.OCC.BackoffBase) != 200*time.Millisecond {
		t.Errorf("expected Storage.OCC.BackoffBase 200ms, got %v", time.Duration(cfg.Storage.OCC.BackoffBase))
	}
	if cfg.Storage.OCC.BackoffFactor != 2.0 {
		t.Errorf("expected Storage.OCC.BackoffFactor 2.0, got %f", cfg.Storage.OCC.BackoffFactor)
	}

	// 4. Orchestrator & Polling
	if time.Duration(cfg.PollInterval) != 5*time.Minute {
		t.Errorf("expected PollInterval 5m, got %v", time.Duration(cfg.PollInterval))
	}
	if time.Duration(cfg.MaxClarificationWait) != 30*time.Minute {
		t.Errorf("expected MaxClarificationWait 30m, got %v", time.Duration(cfg.MaxClarificationWait))
	}
	if cfg.ClarificationTimeoutAction != "abort" {
		t.Errorf("expected ClarificationTimeoutAction 'abort', got %q", cfg.ClarificationTimeoutAction)
	}

	// 5. Agents
	if cfg.Agents.Architecture != "code_first" {
		t.Errorf("expected Agents.Architecture 'code_first', got %q", cfg.Agents.Architecture)
	}
	if cfg.Agents.TaskExecutionOrder != "generator_first" {
		t.Errorf("expected TaskExecutionOrder 'generator_first', got %q", cfg.Agents.TaskExecutionOrder)
	}
	if cfg.Agents.ProductManager.Number != 1 || cfg.Agents.ProductManager.Iterations != 2 || cfg.Agents.ProductManager.Passes != 2 || cfg.Agents.ProductManager.GetMaxUserStories() != 5 {
		t.Errorf("expected ProductManager {1, 2, 2, 5}, got %+v", cfg.Agents.ProductManager)
	}
	if cfg.Agents.Planner.Number != 1 || cfg.Agents.Planner.Iterations != 5 {
		t.Errorf("expected Planner {1, 5}, got %+v", cfg.Agents.Planner)
	}
	if cfg.Agents.Generators.Number != 3 || cfg.Agents.Generators.Iterations != 20 {
		t.Errorf("expected Generators {3, 20}, got %+v", cfg.Agents.Generators)
	}
	if cfg.Agents.Testers.Number != 2 || cfg.Agents.Testers.Iterations != 15 {
		t.Errorf("expected Testers {2, 15}, got %+v", cfg.Agents.Testers)
	}
	if cfg.Agents.QA.Enabled || cfg.Agents.QA.Iterations != 1 {
		t.Errorf("expected QA {Enabled: false, Iterations: 1}, got %+v", cfg.Agents.QA)
	}
	if !cfg.WorkspaceCache.IsEnabled() {
		t.Error("expected WorkspaceCache.IsEnabled() true")
	}

	// 6. LLM
	if cfg.LLM.Provider != "openai" {
		t.Errorf("expected LLM.Provider 'openai', got %q", cfg.LLM.Provider)
	}
	if cfg.LLM.Model != "latest" {
		t.Errorf("expected LLM.Model 'latest', got %q", cfg.LLM.Model)
	}
	if cfg.LLM.Temperature != 0.0 {
		t.Errorf("expected LLM.Temperature 0.0, got %f", cfg.LLM.Temperature)
	}
	if cfg.LLM.TokenUsageLimit != 0 {
		t.Errorf("expected LLM.TokenUsageLimit 0, got %d", cfg.LLM.TokenUsageLimit)
	}
	if cfg.LLM.MaxRetries != 5 {
		t.Errorf("expected LLM.MaxRetries 5, got %d", cfg.LLM.MaxRetries)
	}
	if time.Duration(cfg.LLM.RetryBackoff) != 100*time.Millisecond {
		t.Errorf("expected LLM.RetryBackoff 100ms, got %v", time.Duration(cfg.LLM.RetryBackoff))
	}
	if cfg.LLM.RetryBackoffFactor != 2.0 {
		t.Errorf("expected LLM.RetryBackoffFactor 2.0, got %f", cfg.LLM.RetryBackoffFactor)
	}
	if cfg.LLM.Failover.Enabled {
		t.Error("expected LLM.Failover.Enabled false")
	}
	if time.Duration(cfg.LLM.Failover.Cooldown) != 5*time.Minute {
		t.Errorf("expected LLM.Failover.Cooldown 5m, got %v", time.Duration(cfg.LLM.Failover.Cooldown))
	}
	if time.Duration(cfg.LLM.MaxTimeout) != 60*time.Second {
		t.Errorf("expected LLM.MaxTimeout 60s, got %v", time.Duration(cfg.LLM.MaxTimeout))
	}
	if time.Duration(cfg.LLM.IdleTimeout) != 15*time.Second {
		t.Errorf("expected LLM.IdleTimeout 15s, got %v", time.Duration(cfg.LLM.IdleTimeout))
	}
	if cfg.LLM.Streaming == nil || !*cfg.LLM.Streaming {
		t.Error("expected LLM.Streaming true")
	}

	// 7. VCS
	if cfg.VCS.Provider != "github" {
		t.Errorf("expected VCS.Provider 'github', got %q", cfg.VCS.Provider)
	}
	if cfg.VCS.BaseBranch != "auto" {
		t.Errorf("expected VCS.BaseBranch 'auto', got %q", cfg.VCS.BaseBranch)
	}
	if !cfg.VCS.IsCreateBranchEnabled() {
		t.Error("expected VCS.IsCreateBranchEnabled() true")
	}
	if cfg.VCS.Repository != "local/repo" {
		t.Errorf("expected VCS.Repository 'local/repo', got %q", cfg.VCS.Repository)
	}
	if cfg.VCS.BranchPrefix != "noctifab/" {
		t.Errorf("expected VCS.BranchPrefix 'noctifab/', got %q", cfg.VCS.BranchPrefix)
	}
	if !cfg.VCS.UseWorktrees {
		t.Error("expected VCS.UseWorktrees true")
	}
	if cfg.VCS.PullRequest.AutoCreate || cfg.VCS.PullRequest.AutoMerge || cfg.VCS.PullRequest.AutoRebase || cfg.VCS.PullRequest.Draft {
		t.Errorf("expected PullRequest flags false by default, got %+v", cfg.VCS.PullRequest)
	}

	// 8. Sandbox
	if cfg.Sandbox.Mode != "host" {
		t.Errorf("expected Sandbox.Mode 'host', got %q", cfg.Sandbox.Mode)
	}
	if cfg.Sandbox.TimeoutSeconds != 300 {
		t.Errorf("expected Sandbox.TimeoutSeconds 300, got %d", cfg.Sandbox.TimeoutSeconds)
	}
	if cfg.Sandbox.TestCommand != "go test -v ./..." {
		t.Errorf("expected Sandbox.TestCommand 'go test -v ./...', got %q", cfg.Sandbox.TestCommand)
	}
	if cfg.Sandbox.FormatterCommand != "go fmt ./..." {
		t.Errorf("expected Sandbox.FormatterCommand 'go fmt ./...', got %q", cfg.Sandbox.FormatterCommand)
	}
	if cfg.Sandbox.GetLinterCommand() != "golangci-lint run" {
		t.Errorf("expected GetLinterCommand 'golangci-lint run', got %q", cfg.Sandbox.GetLinterCommand())
	}
	if cfg.Sandbox.GetMaxLinterIssues() != 100 {
		t.Errorf("expected GetMaxLinterIssues 100, got %d", cfg.Sandbox.GetMaxLinterIssues())
	}
	if cfg.Sandbox.GetMaxLinterConsecutiveFailures() != 2 {
		t.Errorf("expected GetMaxLinterConsecutiveFailures 2, got %d", cfg.Sandbox.GetMaxLinterConsecutiveFailures())
	}
	if cfg.Sandbox.GetMaxLinterRetries() != 3 {
		t.Errorf("expected GetMaxLinterRetries 3, got %d", cfg.Sandbox.GetMaxLinterRetries())
	}
	if !reflect.DeepEqual(cfg.Sandbox.ExcludePaths, []string{".noctifab"}) {
		t.Errorf("expected ExcludePaths ['.noctifab'], got %v", cfg.Sandbox.ExcludePaths)
	}

	// 9. Telemetry, SAST, Unblocker, Context
	if cfg.Telemetry.Enabled {
		t.Error("expected Telemetry.Enabled false")
	}
	if !cfg.Telemetry.Metrics.IsEnabled() {
		t.Error("expected Telemetry.Metrics.IsEnabled() true")
	}
	if cfg.SAST.Enabled {
		t.Error("expected SAST.Enabled false")
	}
	if !cfg.Unblocker.Enabled {
		t.Error("expected Unblocker.Enabled true")
	}
	if cfg.Context.GetCompactionMode() != "none" {
		t.Errorf("expected Context.GetCompactionMode 'none', got %q", cfg.Context.GetCompactionMode())
	}
	if cfg.Context.GetMode() != "full" {
		t.Errorf("expected Context.GetMode 'full', got %q", cfg.Context.GetMode())
	}
}

func TestSandboxLinterConfig_YAMLUnmarshaling(t *testing.T) {
	t.Run("when structured sandbox.linter is configured it overrides flat defaults", func(t *testing.T) {
		yamlData := `
sandbox:
  linter:
    command: make lint
    max_issues: 50
    consecutive_failures: 3
    max_retries: 5
`
		cfg := DefaultConfig()
		if err := yaml.Unmarshal([]byte(yamlData), cfg); err != nil {
			t.Fatalf("unexpected yaml unmarshal error: %v", err)
		}
		if cfg.Sandbox.GetLinterCommand() != "make lint" {
			t.Errorf("expected GetLinterCommand 'make lint', got %q", cfg.Sandbox.GetLinterCommand())
		}
		if cfg.Sandbox.GetMaxLinterIssues() != 50 {
			t.Errorf("expected GetMaxLinterIssues 50, got %d", cfg.Sandbox.GetMaxLinterIssues())
		}
		if cfg.Sandbox.GetMaxLinterConsecutiveFailures() != 3 {
			t.Errorf("expected GetMaxLinterConsecutiveFailures 3, got %d", cfg.Sandbox.GetMaxLinterConsecutiveFailures())
		}
		if cfg.Sandbox.GetMaxLinterRetries() != 5 {
			t.Errorf("expected GetMaxLinterRetries 5, got %d", cfg.Sandbox.GetMaxLinterRetries())
		}
	})

	t.Run("structured command equal to default still wins over legacy", func(t *testing.T) {
		yamlData := `
sandbox:
  linter:
    command: golangci-lint run
  linter_command: ruff check
`
		cfg := DefaultConfig()
		if err := yaml.Unmarshal([]byte(yamlData), cfg); err != nil {
			t.Fatalf("unexpected yaml unmarshal error: %v", err)
		}
		if cfg.Sandbox.GetLinterCommand() != "golangci-lint run" {
			t.Errorf("expected structured default to win, got %q", cfg.Sandbox.GetLinterCommand())
		}
	})

	t.Run("structured max_issues 0 means strict mode and wins over legacy", func(t *testing.T) {
		yamlData := `
sandbox:
  linter:
    max_issues: 0
  max_linter_issues: 25
`
		cfg := DefaultConfig()
		if err := yaml.Unmarshal([]byte(yamlData), cfg); err != nil {
			t.Fatalf("unexpected yaml unmarshal error: %v", err)
		}
		if cfg.Sandbox.GetMaxLinterIssues() != 0 {
			t.Errorf("expected strict 0, got %d", cfg.Sandbox.GetMaxLinterIssues())
		}
	})

	t.Run("structured max_issues equal to default 100 wins over legacy", func(t *testing.T) {
		yamlData := `
sandbox:
  linter:
    max_issues: 100
  max_linter_issues: 25
`
		cfg := DefaultConfig()
		if err := yaml.Unmarshal([]byte(yamlData), cfg); err != nil {
			t.Fatalf("unexpected yaml unmarshal error: %v", err)
		}
		if cfg.Sandbox.GetMaxLinterIssues() != 100 {
			t.Errorf("expected structured 100, got %d", cfg.Sandbox.GetMaxLinterIssues())
		}
	})

	t.Run("structured consecutive_failures equal to default 2 wins over legacy", func(t *testing.T) {
		yamlData := `
sandbox:
  linter:
    consecutive_failures: 2
  max_linter_consecutive_failures: 9
`
		cfg := DefaultConfig()
		if err := yaml.Unmarshal([]byte(yamlData), cfg); err != nil {
			t.Fatalf("unexpected yaml unmarshal error: %v", err)
		}
		if cfg.Sandbox.GetMaxLinterConsecutiveFailures() != 2 {
			t.Errorf("expected structured 2, got %d", cfg.Sandbox.GetMaxLinterConsecutiveFailures())
		}
	})

	t.Run("structured max_retries equal to default 3 wins over legacy", func(t *testing.T) {
		yamlData := `
sandbox:
  linter:
    max_retries: 3
  max_linter_retries: 10
`
		cfg := DefaultConfig()
		if err := yaml.Unmarshal([]byte(yamlData), cfg); err != nil {
			t.Fatalf("unexpected yaml unmarshal error: %v", err)
		}
		if cfg.Sandbox.GetMaxLinterRetries() != 3 {
			t.Errorf("expected structured 3, got %d", cfg.Sandbox.GetMaxLinterRetries())
		}
	})

	t.Run("when legacy flat sandbox keys are configured they are correctly returned by accessors", func(t *testing.T) {
		yamlData := `
sandbox:
  linter_command: ruff check
  max_linter_issues: 25
  max_linter_consecutive_failures: 4
  max_linter_retries: 6
`
		cfg := DefaultConfig()
		if err := yaml.Unmarshal([]byte(yamlData), cfg); err != nil {
			t.Fatalf("unexpected yaml unmarshal error: %v", err)
		}
		if cfg.Sandbox.GetLinterCommand() != "ruff check" {
			t.Errorf("expected GetLinterCommand 'ruff check', got %q", cfg.Sandbox.GetLinterCommand())
		}
		if cfg.Sandbox.GetMaxLinterIssues() != 25 {
			t.Errorf("expected GetMaxLinterIssues 25, got %d", cfg.Sandbox.GetMaxLinterIssues())
		}
		if cfg.Sandbox.GetMaxLinterConsecutiveFailures() != 4 {
			t.Errorf("expected GetMaxLinterConsecutiveFailures 4, got %d", cfg.Sandbox.GetMaxLinterConsecutiveFailures())
		}
		if cfg.Sandbox.GetMaxLinterRetries() != 6 {
			t.Errorf("expected GetMaxLinterRetries 6, got %d", cfg.Sandbox.GetMaxLinterRetries())
		}
	})
}
