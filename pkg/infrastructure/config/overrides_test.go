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
		"NOCTIFAB_DB_PATH":                "db-path-env",
		"NOCTIFAB_STORAGE_PROVIDER":       "sqlite",
		"NOCTIFAB_STORAGE_CONN":           "storage-conn-env",
		"NOCTIFAB_INPUT":                  "input-env",
		"NOCTIFAB_AUTO_COMMIT":            "true",
		"NOCTIFAB_AGENTS_COUNT":           "9",
		"NOCTIFAB_INTERVAL":               "12m",
		"NOCTIFAB_VCS_PROVIDER":           "github",
		"NOCTIFAB_VCS_TOKEN":              "vcs-token-env",
		"NOCTIFAB_VCS_REPO":               "owner/repo-env",
		"NOCTIFAB_LLM_PROVIDER":           "openai",
		"NOCTIFAB_LLM_MODEL":              "llm-model-env",
		"NOCTIFAB_LLM_API_KEY":            "llm-api-key-env",
		"NOCTIFAB_LLM_URL":                "llm-url-env",
		"NOCTIFAB_LLM_PLANNER_MODEL":      "planner-env",
		"NOCTIFAB_LLM_GENERATOR_MODEL":    "generator-env",
		"NOCTIFAB_LLM_TESTER_MODEL":      "tester-env",
		"NOCTIFAB_JIRA_USER":              "jira-user-env",
		"NOCTIFAB_JIRA_TOKEN":             "jira-token-env",
		"NOCTIFAB_JIRA_URL":               "jira-url-env",
		"NOCTIFAB_HTTP_MAX_RETRIES":       "7",
		"NOCTIFAB_HTTP_RETRY_BACKOFF":     "150ms",
		"NOCTIFAB_MAX_TOOLS_PER_RESPONSE": "6",
		"NOCTIFAB_MAX_ACTIONS":            "150",
		"NOCTIFAB_MAX_DURATION":           "3h",
		"NOCTIFAB_CONVERSATION_MODE":      "sliding-env",
		"NOCTIFAB_MAX_HISTORY_MESSAGES":   "25",
		"NOCTIFAB_COMPACTION_THRESHOLD":   "35",
		"NOCTIFAB_MAX_HISTORY_TOKENS":     "9999",
		"NOCTIFAB_SANDBOX_MODE":           "docker",
		"NOCTIFAB_SHUTDOWN_GRACE_PERIOD":  "45s",
		"NOCTIFAB_OCC_MAX_RETRIES":        "12",
		"NOCTIFAB_OCC_BACKOFF_BASE":       "99ms",
		"NOCTIFAB_OCC_BACKOFF_FACTOR":     "3.14",
		"NOCTIFAB_MAX_BUDGET_USD":         "99.9",
		"NOCTIFAB_TOKEN_USAGE_LIMIT":      "8888",
		"NOCTIFAB_LOG_LEVEL":              "debug",
		"NOCTIFAB_LOG_FILE":               "log-file-env",
	}

	for k, v := range envVars {
		_ = os.Setenv(k, v)
		defer func(key string) { _ = os.Unsetenv(key) }(k)
	}

	// Prepare mock cmd with ALL flags
	cmd := &cobra.Command{Use: "test"}
	flags := []string{
		"config", "db-path", "storage-provider", "storage-conn", "input", "auto-commit",
		"agents", "interval", "vcs-provider", "vcs-repo", "llm-provider",
		"llm-model", "llm-url", "llm-planner-model", "llm-generator-model",
		"llm-tester-model", "jira-user", "jira-url", "http-max-retries",
		"http-retry-backoff", "max-tools-per-response", "max-actions", "max-duration",
		"conversation-mode", "max-history-messages", "compaction-threshold",
		"max-history-tokens", "sandbox-mode", "shutdown-grace-period", "occ-max-retries",
		"occ-backoff-base", "occ-backoff-factor", "max-budget-usd", "token-usage-limit",
		"log-level", "log-file",
	}
	for _, f := range flags {
		if f == "auto-commit" {
			cmd.Flags().Bool(f, false, "")
		} else {
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
	if cfg.Input != "input-env" {
		t.Errorf("expected input-env, got %s", cfg.Input)
	}
	if !cfg.AutoCommit {
		t.Error("expected auto commit true")
	}
	if cfg.Orchestrator.Concurrency != 9 {
		t.Errorf("expected concurrency 9, got %d", cfg.Orchestrator.Concurrency)
	}
	if time.Duration(cfg.Orchestrator.PollInterval) != 12*time.Minute {
		t.Errorf("expected 12m, got %v", time.Duration(cfg.Orchestrator.PollInterval))
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
	if cfg.Jira.User != "jira-user-env" {
		t.Errorf("expected jira-user-env, got %s", cfg.Jira.User)
	}
	if cfg.Jira.Token != "jira-token-env" {
		t.Errorf("expected jira-token-env, got %s", cfg.Jira.Token)
	}
	if cfg.Jira.URL != "jira-url-env" {
		t.Errorf("expected jira-url-env, got %s", cfg.Jira.URL)
	}
	if cfg.LLM.MaxRetries != 7 {
		t.Errorf("expected max retries 7, got %d", cfg.LLM.MaxRetries)
	}
	if time.Duration(cfg.LLM.RetryBackoff) != 150*time.Millisecond {
		t.Errorf("expected 150ms, got %v", time.Duration(cfg.LLM.RetryBackoff))
	}
	if cfg.Orchestrator.MaxToolsPerResponse != 6 {
		t.Errorf("expected 6, got %d", cfg.Orchestrator.MaxToolsPerResponse)
	}
	if cfg.MaxActions != 150 {
		t.Errorf("expected 150, got %d", cfg.MaxActions)
	}
	if time.Duration(cfg.MaxDuration) != 3*time.Hour {
		t.Errorf("expected 3h, got %v", time.Duration(cfg.MaxDuration))
	}
	if cfg.ConversationMode != "sliding-env" {
		t.Errorf("expected sliding-env, got %s", cfg.ConversationMode)
	}
	if cfg.MaxHistoryMessages != 25 {
		t.Errorf("expected 25, got %d", cfg.MaxHistoryMessages)
	}
	if cfg.CompactionThreshold != 35 {
		t.Errorf("expected 35, got %d", cfg.CompactionThreshold)
	}
	if cfg.MaxHistoryTokens != 9999 {
		t.Errorf("expected 9999, got %d", cfg.MaxHistoryTokens)
	}
	if cfg.Sandbox.Mode != "docker" {
		t.Errorf("expected docker, got %s", cfg.Sandbox.Mode)
	}
	if time.Duration(cfg.ShutdownGracePeriod) != 45*time.Second {
		t.Errorf("expected 45s, got %v", time.Duration(cfg.ShutdownGracePeriod))
	}
	if cfg.OCCMaxRetries != 12 {
		t.Errorf("expected 12, got %d", cfg.OCCMaxRetries)
	}
	if time.Duration(cfg.OCCBackoffBase) != 99*time.Millisecond {
		t.Errorf("expected 99ms, got %v", time.Duration(cfg.OCCBackoffBase))
	}
	if cfg.OCCBackoffFactor != 3.14 {
		t.Errorf("expected 3.14, got %f", cfg.OCCBackoffFactor)
	}
	if cfg.LLM.MaxBudgetUSD != 99.9 {
		t.Errorf("expected 99.9, got %f", cfg.LLM.MaxBudgetUSD)
	}
	if cfg.TokenUsageLimit != 8888 {
		t.Errorf("expected 8888, got %d", cfg.TokenUsageLimit)
	}
	if cfg.LogLevel != "debug" {
		t.Errorf("expected debug, got %s", cfg.LogLevel)
	}
	if cfg.LogFile != "log-file-env" {
		t.Errorf("expected log-file-env, got %s", cfg.LogFile)
	}

	// Override with CLI flags
	_ = cmd.Flags().Set("db-path", "db-path-flag")
	_ = cmd.Flags().Set("storage-provider", "postgres")
	_ = cmd.Flags().Set("storage-conn", "storage-conn-flag")
	_ = cmd.Flags().Set("input", "input-flag")
	_ = cmd.Flags().Set("auto-commit", "false")
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
	_ = cmd.Flags().Set("jira-user", "jira-user-flag")
	_ = cmd.Flags().Set("jira-url", "jira-url-flag")
	_ = cmd.Flags().Set("http-max-retries", "13")
	_ = cmd.Flags().Set("http-retry-backoff", "250ms")
	_ = cmd.Flags().Set("max-tools-per-response", "12")
	_ = cmd.Flags().Set("max-actions", "250")
	_ = cmd.Flags().Set("max-duration", "5h")
	_ = cmd.Flags().Set("conversation-mode", "sliding-flag")
	_ = cmd.Flags().Set("max-history-messages", "35")
	_ = cmd.Flags().Set("compaction-threshold", "45")
	_ = cmd.Flags().Set("max-history-tokens", "12000")
	_ = cmd.Flags().Set("sandbox-mode", "host")
	_ = cmd.Flags().Set("shutdown-grace-period", "55s")
	_ = cmd.Flags().Set("occ-max-retries", "18")
	_ = cmd.Flags().Set("occ-backoff-base", "150ms")
	_ = cmd.Flags().Set("occ-backoff-factor", "4.0")
	_ = cmd.Flags().Set("max-budget-usd", "150.0")
	_ = cmd.Flags().Set("token-usage-limit", "9999")
	_ = cmd.Flags().Set("log-level", "info")
	_ = cmd.Flags().Set("log-file", "log-file-flag")

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
	if cfg2.Input != "input-flag" {
		t.Errorf("expected input-flag, got %s", cfg2.Input)
	}
	if cfg2.AutoCommit {
		t.Error("expected auto commit false")
	}
	if cfg2.Orchestrator.Concurrency != 11 {
		t.Errorf("expected concurrency 11, got %d", cfg2.Orchestrator.Concurrency)
	}
	if time.Duration(cfg2.Orchestrator.PollInterval) != 15*time.Minute {
		t.Errorf("expected 15m, got %v", time.Duration(cfg2.Orchestrator.PollInterval))
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
	if cfg2.Jira.User != "jira-user-flag" {
		t.Errorf("expected jira-user-flag, got %s", cfg2.Jira.User)
	}
	if cfg2.Jira.URL != "jira-url-flag" {
		t.Errorf("expected jira-url-flag, got %s", cfg2.Jira.URL)
	}
	if cfg2.LLM.MaxRetries != 13 {
		t.Errorf("expected max retries 13, got %d", cfg2.LLM.MaxRetries)
	}
	if time.Duration(cfg2.LLM.RetryBackoff) != 250*time.Millisecond {
		t.Errorf("expected 250ms, got %v", time.Duration(cfg2.LLM.RetryBackoff))
	}
	if cfg2.Orchestrator.MaxToolsPerResponse != 12 {
		t.Errorf("expected 12, got %d", cfg2.Orchestrator.MaxToolsPerResponse)
	}
	if cfg2.MaxActions != 250 {
		t.Errorf("expected 250, got %d", cfg2.MaxActions)
	}
	if time.Duration(cfg2.MaxDuration) != 5*time.Hour {
		t.Errorf("expected 5h, got %v", time.Duration(cfg2.MaxDuration))
	}
	if cfg2.ConversationMode != "sliding-flag" {
		t.Errorf("expected sliding-flag, got %s", cfg2.ConversationMode)
	}
	if cfg2.MaxHistoryMessages != 35 {
		t.Errorf("expected 35, got %d", cfg2.MaxHistoryMessages)
	}
	if cfg2.CompactionThreshold != 45 {
		t.Errorf("expected 45, got %d", cfg2.CompactionThreshold)
	}
	if cfg2.MaxHistoryTokens != 12000 {
		t.Errorf("expected 12000, got %d", cfg2.MaxHistoryTokens)
	}
	if cfg2.Sandbox.Mode != "host" {
		t.Errorf("expected host, got %s", cfg2.Sandbox.Mode)
	}
	if time.Duration(cfg2.ShutdownGracePeriod) != 55*time.Second {
		t.Errorf("expected 55s, got %v", time.Duration(cfg2.ShutdownGracePeriod))
	}
	if cfg2.OCCMaxRetries != 18 {
		t.Errorf("expected 18, got %d", cfg2.OCCMaxRetries)
	}
	if time.Duration(cfg2.OCCBackoffBase) != 150*time.Millisecond {
		t.Errorf("expected 150ms, got %v", time.Duration(cfg2.OCCBackoffBase))
	}
	if cfg2.OCCBackoffFactor != 4.0 {
		t.Errorf("expected 4.0, got %f", cfg2.OCCBackoffFactor)
	}
	if cfg2.LLM.MaxBudgetUSD != 150.0 {
		t.Errorf("expected 150.0, got %f", cfg2.LLM.MaxBudgetUSD)
	}
	if cfg2.TokenUsageLimit != 9999 {
		t.Errorf("expected 9999, got %d", cfg2.TokenUsageLimit)
	}
	if cfg2.LogLevel != "info" {
		t.Errorf("expected info, got %s", cfg2.LogLevel)
	}
	if cfg2.LogFile != "log-file-flag" {
		t.Errorf("expected log-file-flag, got %s", cfg2.LogFile)
	}
}
