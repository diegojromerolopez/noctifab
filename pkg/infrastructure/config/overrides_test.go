package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/spf13/cobra"
)

func TestLoad_AndOverrides(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "noctifab-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer func() { _ = os.RemoveAll(tmpDir) }()

	configPath := filepath.Join(tmpDir, "config.yaml")

	_ = os.Setenv("NOCTIFAB_CONFIG", configPath)
	defer func() { _ = os.Unsetenv("NOCTIFAB_CONFIG") }()

	// Set required variables
	_ = os.Setenv("GITHUB_TOKEN", "test-token")
	defer func() { _ = os.Unsetenv("GITHUB_TOKEN") }()
	_ = os.Setenv("OPENAI_API_KEY", "test-api-key")
	defer func() { _ = os.Unsetenv("OPENAI_API_KEY") }()

	// Write empty file to trigger YAML load without error
	if err := os.WriteFile(configPath, []byte(""), 0644); err != nil {
		t.Fatalf("failed to write empty config file: %v", err)
	}

	// Set ALL environment variables
	envVars := map[string]string{
		"NOCTIFAB_DB_PATH":             "db-path-env",
		"NOCTIFAB_STORAGE_PROVIDER":    "sqlite",
		"NOCTIFAB_STORAGE_CONN":        "storage-conn-env",
		"NOCTIFAB_INPUT":               "input-env",
		"NOCTIFAB_AUTO_COMMIT":         "true",
		"NOCTIFAB_AGENTS_COUNT":        "9",
		"NOCTIFAB_INTERVAL":            "12m",
		"NOCTIFAB_VCS_PROVIDER":        "github",
		"NOCTIFAB_VCS_TOKEN":           "vcs-token-env",
		"NOCTIFAB_VCS_REPO":            "owner/repo-env",
		"NOCTIFAB_LLM_PROVIDER":        "openai",
		"NOCTIFAB_LLM_MODEL":           "llm-model-env",
		"NOCTIFAB_LLM_API_KEY":         "llm-api-key-env",
		"NOCTIFAB_LLM_URL":             "llm-url-env",
		"NOCTIFAB_LLM_PLANNER_MODEL":   "planner-env",
		"NOCTIFAB_LLM_GENERATOR_MODEL": "generator-env",
		"NOCTIFAB_LLM_TESTER_MODEL":    "tester-env",
		"NOCTIFAB_HTTP_MAX_RETRIES":    "7",
		"NOCTIFAB_HTTP_RETRY_BACKOFF":  "150ms",
		"NOCTIFAB_MAX_ACTIONS":         "150",
		"NOCTIFAB_MAX_DURATION":        "3h",
		"NOCTIFAB_SANDBOX_MODE":        "docker",
		"NOCTIFAB_OCC_MAX_RETRIES":     "12",
		"NOCTIFAB_OCC_BACKOFF_BASE":    "99ms",
		"NOCTIFAB_OCC_BACKOFF_FACTOR":  "3.14",
		"NOCTIFAB_TOKEN_USAGE_LIMIT":   "8888",
		"NOCTIFAB_LOG_LEVEL":           "debug",
		"NOCTIFAB_LOG_FILE":            "log-file-env",
		"NOCTIFAB_PR_AUTO_CREATE":      "true",
		"NOCTIFAB_PR_AUTO_MERGE":       "true",
		"NOCTIFAB_PR_AUTO_REBASE":      "true",
		"NOCTIFAB_PR_DRAFT":            "true",
		"NOCTIFAB_PR_ASSIGNEES":        "user1, user2",
		"NOCTIFAB_PR_LABELS":           "auto,bot",
	}

	for k, v := range envVars {
		_ = os.Setenv(k, v)
		defer func(key string) { _ = os.Unsetenv(key) }(k)
	}

	// Prepare mock cmd with ALL flags
	cmd := &cobra.Command{Use: "test"}
	flags := []string{
		"config", "db-path", "storage-provider", "storage-conn", "input",
		"agents", "interval", "vcs-provider", "vcs-repo", "llm-provider",
		"llm-model", "llm-url", "llm-planner-model", "llm-generator-model",
		"llm-tester-model", "http-max-retries",
		"http-retry-backoff", "max-actions", "max-duration",
		"sandbox-mode", "occ-max-retries",
		"occ-backoff-base", "occ-backoff-factor", "token-usage-limit",
		"log-level", "log-file", "pr-auto-create", "pr-auto-merge", "pr-auto-rebase",
		"pr-draft", "pr-assignees", "pr-labels",
	}
	for _, f := range flags {
		switch f {
		case "pr-auto-create", "pr-auto-merge", "pr-auto-rebase", "pr-draft":
			cmd.Flags().Bool(f, false, "")
		default:
			cmd.Flags().String(f, "", "")
		}
	}

	cfg, err := Load(cmd)
	if err != nil {
		t.Fatalf("Load with valid env failed: %v", err)
	}

	// Assert environment variables took effect
	if cfg.Storage.ConnString != "storage-conn-env" {
		t.Errorf("expected storage-conn-env, got %s", cfg.Storage.ConnString)
	}
	if cfg.Storage.Provider != "sqlite" {
		t.Errorf("expected sqlite, got %s", cfg.Storage.Provider)
	}
	if cfg.Runtime.SpecSource != "input-env" {
		t.Errorf("expected input-env, got %s", cfg.Runtime.SpecSource)
	}
	if cfg.Agents.Generators.Number != 9 {
		t.Errorf("expected concurrency 9, got %d", cfg.Agents.Generators.Number)
	}
	if time.Duration(cfg.PollInterval) != 12*time.Minute {
		t.Errorf("expected 12m, got %v", time.Duration(cfg.PollInterval))
	}
	if cfg.VCS.Provider != "github" {
		t.Errorf("expected github, got %s", cfg.VCS.Provider)
	}
	if cfg.VCS.TokenValue != "vcs-token-env" {
		t.Errorf("expected vcs-token-env, got %s", cfg.VCS.TokenValue)
	}
	if cfg.VCS.Repository != "owner/repo-env" {
		t.Errorf("expected owner/repo-env, got %s", cfg.VCS.Repository)
	}
	if cfg.LLM.Provider != "openai" {
		t.Errorf("expected openai, got %s", cfg.LLM.Provider)
	}
	if cfg.LLM.Model != "llm-model-env" {
		t.Errorf("expected llm-model-env, got %s", cfg.LLM.Model)
	}
	if cfg.LLM.APIKeyValue != "llm-api-key-env" {
		t.Errorf("expected llm-api-key-env, got %s", cfg.LLM.APIKeyValue)
	}
	if cfg.LLM.URL != "llm-url-env" {
		t.Errorf("expected llm-url-env, got %s", cfg.LLM.URL)
	}
	if cfg.Roles.Planner.Model != "planner-env" {
		t.Errorf("expected planner-env, got %s", cfg.Roles.Planner.Model)
	}
	if cfg.Roles.Generator.Model != "generator-env" {
		t.Errorf("expected generator-env, got %s", cfg.Roles.Generator.Model)
	}
	if cfg.Roles.Tester.Model != "tester-env" {
		t.Errorf("expected tester-env, got %s", cfg.Roles.Tester.Model)
	}
	if cfg.LLM.MaxRetries != 7 {
		t.Errorf("expected max retries 7, got %d", cfg.LLM.MaxRetries)
	}
	if time.Duration(cfg.LLM.RetryBackoff) != 150*time.Millisecond {
		t.Errorf("expected 150ms, got %v", time.Duration(cfg.LLM.RetryBackoff))
	}
	if cfg.Runtime.MaxActions != 150 {
		t.Errorf("expected 150, got %d", cfg.Runtime.MaxActions)
	}
	if time.Duration(cfg.Runtime.MaxDuration) != 3*time.Hour {
		t.Errorf("expected 3h, got %v", time.Duration(cfg.Runtime.MaxDuration))
	}
	if cfg.Sandbox.Mode != "docker" {
		t.Errorf("expected docker, got %s", cfg.Sandbox.Mode)
	}
	if cfg.Storage.OCC.MaxRetries != 12 {
		t.Errorf("expected 12, got %d", cfg.Storage.OCC.MaxRetries)
	}
	if time.Duration(cfg.Storage.OCC.BackoffBase) != 99*time.Millisecond {
		t.Errorf("expected 99ms, got %v", time.Duration(cfg.Storage.OCC.BackoffBase))
	}
	if cfg.Storage.OCC.BackoffFactor != 3.14 {
		t.Errorf("expected 3.14, got %f", cfg.Storage.OCC.BackoffFactor)
	}
	if cfg.LLM.TokenUsageLimit != 8888 {
		t.Errorf("expected 8888, got %d", cfg.LLM.TokenUsageLimit)
	}
	if cfg.Logging.Level != "debug" {
		t.Errorf("expected debug, got %s", cfg.Logging.Level)
	}
	if cfg.Logging.File != "log-file-env" {
		t.Errorf("expected log-file-env, got %s", cfg.Logging.File)
	}
	if !cfg.VCS.PullRequest.AutoCreate {
		t.Error("expected pr-auto-create true")
	}
	if !cfg.VCS.PullRequest.AutoMerge {
		t.Error("expected pr-auto-merge true")
	}
	if !cfg.VCS.PullRequest.AutoRebase {
		t.Error("expected pr-auto-rebase true")
	}
	if !cfg.VCS.PullRequest.Draft {
		t.Error("expected pr-draft true")
	}
	if len(cfg.VCS.PullRequest.Assignees) != 2 || cfg.VCS.PullRequest.Assignees[0] != "user1" {
		t.Errorf("expected [user1 user2], got %v", cfg.VCS.PullRequest.Assignees)
	}
	if len(cfg.VCS.PullRequest.Labels) != 2 || cfg.VCS.PullRequest.Labels[0] != "auto" {
		t.Errorf("expected [auto bot], got %v", cfg.VCS.PullRequest.Labels)
	}

	// Override with CLI flags
	_ = cmd.Flags().Set("db-path", "db-path-flag")
	_ = cmd.Flags().Set("storage-provider", "postgres")
	_ = cmd.Flags().Set("storage-conn", "storage-conn-flag")
	_ = cmd.Flags().Set("input", "input-flag")
	_ = cmd.Flags().Set("agents", "11")
	_ = cmd.Flags().Set("interval", "15m")
	_ = cmd.Flags().Set("vcs-provider", "gitlab")
	_ = cmd.Flags().Set("vcs-repo", "owner/repo-flag")
	_ = cmd.Flags().Set("llm-provider", "anthropic")
	_ = cmd.Flags().Set("llm-model", "llm-model-flag")
	_ = cmd.Flags().Set("llm-url", "llm-url-flag")
	_ = cmd.Flags().Set("llm-planner-model", "planner-flag")
	_ = cmd.Flags().Set("llm-generator-model", "generator-flag")
	_ = cmd.Flags().Set("llm-tester-model", "tester-flag")
	_ = cmd.Flags().Set("http-max-retries", "13")
	_ = cmd.Flags().Set("http-retry-backoff", "250ms")
	_ = cmd.Flags().Set("max-actions", "250")
	_ = cmd.Flags().Set("max-duration", "5h")
	_ = cmd.Flags().Set("sandbox-mode", "host")
	_ = cmd.Flags().Set("occ-max-retries", "18")
	_ = cmd.Flags().Set("occ-backoff-base", "150ms")
	_ = cmd.Flags().Set("occ-backoff-factor", "4.0")
	_ = cmd.Flags().Set("token-usage-limit", "9999")
	_ = cmd.Flags().Set("log-level", "info")
	_ = cmd.Flags().Set("log-file", "log-file-flag")
	_ = cmd.Flags().Set("pr-auto-create", "false")
	_ = cmd.Flags().Set("pr-auto-merge", "false")
	_ = cmd.Flags().Set("pr-auto-rebase", "false")
	_ = cmd.Flags().Set("pr-draft", "false")
	_ = cmd.Flags().Set("pr-assignees", "flag1, flag2")
	_ = cmd.Flags().Set("pr-labels", "flag-a,flag-b")

	cfg2, err := Load(cmd)
	if err != nil {
		t.Fatalf("Load with valid flags failed: %v", err)
	}

	if cfg2.Storage.ConnString != "storage-conn-flag" {
		t.Errorf("expected storage-conn-flag, got %s", cfg2.Storage.ConnString)
	}
	if cfg2.Storage.Provider != "postgres" {
		t.Errorf("expected postgres, got %s", cfg2.Storage.Provider)
	}
	if cfg2.Runtime.SpecSource != "input-flag" {
		t.Errorf("expected input-flag, got %s", cfg2.Runtime.SpecSource)
	}
	if cfg2.Agents.Generators.Number != 11 {
		t.Errorf("expected concurrency 11, got %d", cfg2.Agents.Generators.Number)
	}
	if time.Duration(cfg2.PollInterval) != 15*time.Minute {
		t.Errorf("expected 15m, got %v", time.Duration(cfg2.PollInterval))
	}
	if cfg2.VCS.Provider != "gitlab" {
		t.Errorf("expected gitlab, got %s", cfg2.VCS.Provider)
	}
	if cfg2.VCS.Repository != "owner/repo-flag" {
		t.Errorf("expected owner/repo-flag, got %s", cfg2.VCS.Repository)
	}
	if cfg2.LLM.Provider != "anthropic" {
		t.Errorf("expected anthropic, got %s", cfg2.LLM.Provider)
	}
	if cfg2.LLM.Model != "llm-model-flag" {
		t.Errorf("expected llm-model-flag, got %s", cfg2.LLM.Model)
	}
	if cfg2.LLM.URL != "llm-url-flag" {
		t.Errorf("expected llm-url-flag, got %s", cfg2.LLM.URL)
	}
	if cfg2.Roles.Planner.Model != "planner-flag" {
		t.Errorf("expected planner-flag, got %s", cfg2.Roles.Planner.Model)
	}
	if cfg2.Roles.Generator.Model != "generator-flag" {
		t.Errorf("expected generator-flag, got %s", cfg2.Roles.Generator.Model)
	}
	if cfg2.Roles.Tester.Model != "tester-flag" {
		t.Errorf("expected tester-flag, got %s", cfg2.Roles.Tester.Model)
	}
	if cfg2.LLM.MaxRetries != 13 {
		t.Errorf("expected max retries 13, got %d", cfg2.LLM.MaxRetries)
	}
	if time.Duration(cfg2.LLM.RetryBackoff) != 250*time.Millisecond {
		t.Errorf("expected 250ms, got %v", time.Duration(cfg2.LLM.RetryBackoff))
	}
	if cfg2.Runtime.MaxActions != 250 {
		t.Errorf("expected 250, got %d", cfg2.Runtime.MaxActions)
	}
	if time.Duration(cfg2.Runtime.MaxDuration) != 5*time.Hour {
		t.Errorf("expected 5h, got %v", time.Duration(cfg2.Runtime.MaxDuration))
	}
	if cfg2.Sandbox.Mode != "host" {
		t.Errorf("expected host, got %s", cfg2.Sandbox.Mode)
	}
	if cfg2.Storage.OCC.MaxRetries != 18 {
		t.Errorf("expected 18, got %d", cfg2.Storage.OCC.MaxRetries)
	}
	if time.Duration(cfg2.Storage.OCC.BackoffBase) != 150*time.Millisecond {
		t.Errorf("expected 150ms, got %v", time.Duration(cfg2.Storage.OCC.BackoffBase))
	}
	if cfg2.Storage.OCC.BackoffFactor != 4.0 {
		t.Errorf("expected 4.0, got %f", cfg2.Storage.OCC.BackoffFactor)
	}
	if cfg2.LLM.TokenUsageLimit != 9999 {
		t.Errorf("expected 9999, got %d", cfg2.LLM.TokenUsageLimit)
	}
	if cfg2.Logging.Level != "info" {
		t.Errorf("expected info, got %s", cfg2.Logging.Level)
	}
	if cfg2.Logging.File != "log-file-flag" {
		t.Errorf("expected log-file-flag, got %s", cfg2.Logging.File)
	}
	if cfg2.VCS.PullRequest.AutoCreate {
		t.Error("expected pr-auto-create false")
	}
	if cfg2.VCS.PullRequest.AutoMerge {
		t.Error("expected pr-auto-merge false")
	}
	if cfg2.VCS.PullRequest.AutoRebase {
		t.Error("expected pr-auto-rebase false")
	}
	if cfg2.VCS.PullRequest.Draft {
		t.Error("expected pr-draft false")
	}
	if len(cfg2.VCS.PullRequest.Assignees) != 2 || cfg2.VCS.PullRequest.Assignees[0] != "flag1" {
		t.Errorf("expected [flag1 flag2], got %v", cfg2.VCS.PullRequest.Assignees)
	}
	if len(cfg2.VCS.PullRequest.Labels) != 2 || cfg2.VCS.PullRequest.Labels[0] != "flag-a" {
		t.Errorf("expected [flag-a flag-b], got %v", cfg2.VCS.PullRequest.Labels)
	}
}

func TestLoad_SpecSourceOverrides(t *testing.T) {
	t.Run("when NOCTIFAB_SPEC_SOURCE is set", func(t *testing.T) {
		_ = os.Setenv("NOCTIFAB_SPEC_SOURCE", "specs/feature.md")
		defer func() { _ = os.Unsetenv("NOCTIFAB_SPEC_SOURCE") }()

		cfg := &Config{}
		applyEnvOverrides(cfg)
		if cfg.Runtime.SpecSource != "specs/feature.md" {
			t.Errorf("expected specs/feature.md, got %s", cfg.Runtime.SpecSource)
		}
	})

	t.Run("when NOCTIFAB_INPUT alias is set", func(t *testing.T) {
		_ = os.Setenv("NOCTIFAB_INPUT", "specs/alias.md")
		defer func() { _ = os.Unsetenv("NOCTIFAB_INPUT") }()

		cfg := &Config{}
		applyEnvOverrides(cfg)
		if cfg.Runtime.SpecSource != "specs/alias.md" {
			t.Errorf("expected specs/alias.md, got %s", cfg.Runtime.SpecSource)
		}
	})

	t.Run("when both NOCTIFAB_SPEC_SOURCE and NOCTIFAB_INPUT are set, canonical wins", func(t *testing.T) {
		_ = os.Setenv("NOCTIFAB_SPEC_SOURCE", "specs/canonical.md")
		_ = os.Setenv("NOCTIFAB_INPUT", "specs/alias.md")
		defer func() {
			_ = os.Unsetenv("NOCTIFAB_SPEC_SOURCE")
			_ = os.Unsetenv("NOCTIFAB_INPUT")
		}()

		cfg := &Config{}
		applyEnvOverrides(cfg)
		if cfg.Runtime.SpecSource != "specs/canonical.md" {
			t.Errorf("expected canonical specs/canonical.md to win, got %s", cfg.Runtime.SpecSource)
		}
	})

	t.Run("when --spec-source flag is set", func(t *testing.T) {
		cmd := &cobra.Command{Use: "test"}
		cmd.Flags().String("spec-source", "", "")
		cmd.Flags().String("input", "", "")
		_ = cmd.Flags().Set("spec-source", "specs/feature-flag.md")

		cfg := &Config{}
		applyFlagOverrides(cfg, cmd)
		if cfg.Runtime.SpecSource != "specs/feature-flag.md" {
			t.Errorf("expected specs/feature-flag.md, got %s", cfg.Runtime.SpecSource)
		}
	})

	t.Run("when --input alias flag is set", func(t *testing.T) {
		cmd := &cobra.Command{Use: "test"}
		cmd.Flags().String("spec-source", "", "")
		cmd.Flags().String("input", "", "")
		_ = cmd.Flags().Set("input", "specs/alias-flag.md")

		cfg := &Config{}
		applyFlagOverrides(cfg, cmd)
		if cfg.Runtime.SpecSource != "specs/alias-flag.md" {
			t.Errorf("expected specs/alias-flag.md, got %s", cfg.Runtime.SpecSource)
		}
	})

	t.Run("when both --spec-source and --input flags are set, canonical wins", func(t *testing.T) {
		cmd := &cobra.Command{Use: "test"}
		cmd.Flags().String("spec-source", "", "")
		cmd.Flags().String("input", "", "")
		_ = cmd.Flags().Set("spec-source", "specs/canonical-flag.md")
		_ = cmd.Flags().Set("input", "specs/alias-flag.md")

		cfg := &Config{}
		applyFlagOverrides(cfg, cmd)
		if cfg.Runtime.SpecSource != "specs/canonical-flag.md" {
			t.Errorf("expected canonical specs/canonical-flag.md to win, got %s", cfg.Runtime.SpecSource)
		}
	})
}
