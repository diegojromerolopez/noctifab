package config

import (
	"reflect"
	"testing"
	"time"
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
	if cfg.Agents.ProductManager.Number != 1 || cfg.Agents.ProductManager.Iterations != 2 || cfg.Agents.ProductManager.Passes != 2 || cfg.Agents.ProductManager.MaxUserStories != 5 {
		t.Errorf("expected ProductManager {1, 2, 2, 5}, got %+v", cfg.Agents.ProductManager)
	}
	if cfg.Agents.Planner.Number != 1 || cfg.Agents.Planner.Iterations != 2 {
		t.Errorf("expected Planner {1, 2}, got %+v", cfg.Agents.Planner)
	}
	if cfg.Agents.Generators.Number != 3 || cfg.Agents.Generators.Iterations != 5 {
		t.Errorf("expected Generators {3, 5}, got %+v", cfg.Agents.Generators)
	}
	if cfg.Agents.Testers.Number != 2 || cfg.Agents.Testers.Iterations != 3 {
		t.Errorf("expected Testers {2, 3}, got %+v", cfg.Agents.Testers)
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
	if cfg.Sandbox.LinterCommand != "golangci-lint run" {
		t.Errorf("expected Sandbox.LinterCommand 'golangci-lint run', got %q", cfg.Sandbox.LinterCommand)
	}
	if cfg.Sandbox.FormatterCommand != "go fmt ./..." {
		t.Errorf("expected Sandbox.FormatterCommand 'go fmt ./...', got %q", cfg.Sandbox.FormatterCommand)
	}
	if cfg.Sandbox.MaxLinterRetries != 3 {
		t.Errorf("expected Sandbox.MaxLinterRetries 3, got %d", cfg.Sandbox.MaxLinterRetries)
	}
	if cfg.Sandbox.MaxLinterIssues != 100 {
		t.Errorf("expected Sandbox.MaxLinterIssues 100, got %d", cfg.Sandbox.MaxLinterIssues)
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
