package config

import (
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

func applyEnvOverrides(cfg *Config) {
	if val, ok := os.LookupEnv("NOCTIFAB_DB_PATH"); ok {
		cfg.Storage.ConnString = val
	}
	if val, ok := os.LookupEnv("NOCTIFAB_STORAGE_PROVIDER"); ok {
		cfg.Storage.Provider = val
	}
	if val, ok := os.LookupEnv("NOCTIFAB_STORAGE_CONN"); ok {
		cfg.Storage.ConnString = val
	}
	if val, ok := os.LookupEnv("NOCTIFAB_INPUT"); ok {
		cfg.Input = val
	}
	if val, ok := os.LookupEnv("NOCTIFAB_AUTO_COMMIT"); ok {
		if b, err := strconv.ParseBool(val); err == nil {
			cfg.AutoCommit = b
		}
	}
	if val, ok := os.LookupEnv("NOCTIFAB_AGENTS_COUNT"); ok {
		if i, err := strconv.Atoi(val); err == nil {
			cfg.Agents.Generators.Number = i
		}
	}
	if val, ok := os.LookupEnv("NOCTIFAB_INTERVAL"); ok {
		if d, err := time.ParseDuration(val); err == nil {
			cfg.PollInterval = Duration(d)
		}
	}
	if val, ok := os.LookupEnv("NOCTIFAB_VCS_PROVIDER"); ok {
		cfg.VCS.Provider = val
	}
	if val, ok := os.LookupEnv("NOCTIFAB_VCS_TOKEN"); ok {
		cfg.VCS.TokenValue = val
	}
	if val, ok := os.LookupEnv("NOCTIFAB_VCS_REPO"); ok {
		cfg.VCS.Repository = val
	}
	if val, ok := os.LookupEnv("NOCTIFAB_LLM_PROVIDER"); ok {
		cfg.LLM.Provider = val
	}
	if val, ok := os.LookupEnv("NOCTIFAB_LLM_MODEL"); ok {
		cfg.LLM.Model = val
	}
	if val, ok := os.LookupEnv("NOCTIFAB_LLM_API_KEY"); ok {
		cfg.LLM.APIKeyValue = val
		cfg.LLM.APIKeyPool = []string{val}
	}
	if val, ok := os.LookupEnv("NOCTIFAB_LLM_URL"); ok {
		cfg.LLM.URL = val
	}
	if val, ok := os.LookupEnv("NOCTIFAB_LLM_PLANNER_MODEL"); ok {
		cfg.Roles.Planner.Model = val
	}
	if val, ok := os.LookupEnv("NOCTIFAB_LLM_GENERATOR_MODEL"); ok {
		cfg.Roles.Generator.Model = val
	}
	if val, ok := os.LookupEnv("NOCTIFAB_LLM_TESTER_MODEL"); ok {
		cfg.Roles.Tester.Model = val
	}
	if val, ok := os.LookupEnv("NOCTIFAB_JIRA_USER"); ok {
		cfg.Jira.User = val
	}
	if val, ok := os.LookupEnv("NOCTIFAB_JIRA_TOKEN"); ok {
		cfg.Jira.Token = val
	}
	if val, ok := os.LookupEnv("NOCTIFAB_JIRA_URL"); ok {
		cfg.Jira.URL = val
	}
	if val, ok := os.LookupEnv("NOCTIFAB_HTTP_MAX_RETRIES"); ok {
		if i, err := strconv.Atoi(val); err == nil {
			cfg.LLM.MaxRetries = i
		}
	}
	if val, ok := os.LookupEnv("NOCTIFAB_HTTP_RETRY_BACKOFF"); ok {
		if d, err := time.ParseDuration(val); err == nil {
			cfg.LLM.RetryBackoff = Duration(d)
		}
	}
	if val, ok := os.LookupEnv("NOCTIFAB_MAX_TOOLS_PER_RESPONSE"); ok {
		if i, err := strconv.Atoi(val); err == nil {
			cfg.Agents.MaxToolsPerResponse = i
		}
	}
	if val, ok := os.LookupEnv("NOCTIFAB_MAX_ACTIONS"); ok {
		if i, err := strconv.Atoi(val); err == nil {
			cfg.MaxActions = i
		}
	}
	if val, ok := os.LookupEnv("NOCTIFAB_MAX_DURATION"); ok {
		if d, err := time.ParseDuration(val); err == nil {
			cfg.MaxDuration = Duration(d)
		}
	}
	if val, ok := os.LookupEnv("NOCTIFAB_CONVERSATION_MODE"); ok {
		cfg.ConversationMode = val
	}
	if val, ok := os.LookupEnv("NOCTIFAB_MAX_HISTORY_MESSAGES"); ok {
		if i, err := strconv.Atoi(val); err == nil {
			cfg.MaxHistoryMessages = i
		}
	}
	if val, ok := os.LookupEnv("NOCTIFAB_COMPACTION_THRESHOLD"); ok {
		if i, err := strconv.Atoi(val); err == nil {
			cfg.CompactionThreshold = i
		}
	}
	if val, ok := os.LookupEnv("NOCTIFAB_MAX_HISTORY_TOKENS"); ok {
		if i, err := strconv.Atoi(val); err == nil {
			cfg.MaxHistoryTokens = i
		}
	}
	if val, ok := os.LookupEnv("NOCTIFAB_SANDBOX_MODE"); ok {
		cfg.Sandbox.Mode = val
	}
	if val, ok := os.LookupEnv("NOCTIFAB_SHUTDOWN_GRACE_PERIOD"); ok {
		if d, err := time.ParseDuration(val); err == nil {
			cfg.ShutdownGracePeriod = Duration(d)
		}
	}
	if val, ok := os.LookupEnv("NOCTIFAB_OCC_MAX_RETRIES"); ok {
		if i, err := strconv.Atoi(val); err == nil {
			cfg.OCCMaxRetries = i
		}
	}
	if val, ok := os.LookupEnv("NOCTIFAB_OCC_BACKOFF_BASE"); ok {
		if d, err := time.ParseDuration(val); err == nil {
			cfg.OCCBackoffBase = Duration(d)
		}
	}
	if val, ok := os.LookupEnv("NOCTIFAB_OCC_BACKOFF_FACTOR"); ok {
		if f, err := strconv.ParseFloat(val, 64); err == nil {
			cfg.OCCBackoffFactor = f
		}
	}
	if val, ok := os.LookupEnv("NOCTIFAB_TOKEN_USAGE_LIMIT"); ok {
		if i, err := strconv.ParseInt(val, 10, 64); err == nil {
			cfg.TokenUsageLimit = i
		}
	}
	if val, ok := os.LookupEnv("NOCTIFAB_LOG_LEVEL"); ok {
		cfg.LogLevel = val
	}
	if val, ok := os.LookupEnv("NOCTIFAB_LOG_FILE"); ok {
		cfg.LogFile = val
	}
	if val, ok := os.LookupEnv("NOCTIFAB_PR_AUTO_CREATE"); ok {
		if b, err := strconv.ParseBool(val); err == nil {
			cfg.VCS.PullRequest.AutoCreate = b
		}
	}
	if val, ok := os.LookupEnv("NOCTIFAB_PR_AUTO_MERGE"); ok {
		if b, err := strconv.ParseBool(val); err == nil {
			cfg.VCS.PullRequest.AutoMerge = b
		}
	}
	if val, ok := os.LookupEnv("NOCTIFAB_PR_AUTO_REBASE"); ok {
		if b, err := strconv.ParseBool(val); err == nil {
			cfg.VCS.PullRequest.AutoRebase = b
		}
	}
	if val, ok := os.LookupEnv("NOCTIFAB_PR_DRAFT"); ok {
		if b, err := strconv.ParseBool(val); err == nil {
			cfg.VCS.PullRequest.Draft = b
		}
	}
	if val, ok := os.LookupEnv("NOCTIFAB_PR_ASSIGNEES"); ok {
		cfg.VCS.PullRequest.Assignees = splitAndTrim(val)
	}
	if val, ok := os.LookupEnv("NOCTIFAB_PR_LABELS"); ok {
		cfg.VCS.PullRequest.Labels = splitAndTrim(val)
	}
	if val, ok := os.LookupEnv("NOCTIFAB_CI_AUTO_FIX"); ok {
		if b, err := strconv.ParseBool(val); err == nil {
			cfg.VCS.CI.AutoFix = b
		}
	}
	if val, ok := os.LookupEnv("NOCTIFAB_CI_MAX_RETRIES"); ok {
		if i, err := strconv.Atoi(val); err == nil {
			cfg.VCS.CI.MaxRetries = i
		}
	}
	if val, ok := os.LookupEnv("NOCTIFAB_UNBLOCKER_ENABLED"); ok {
		if b, err := strconv.ParseBool(val); err == nil {
			cfg.Unblocker.Enabled = b
		}
	}
	if val, ok := os.LookupEnv("NOCTIFAB_UNBLOCKER_POLL_INTERVAL"); ok {
		if d, err := time.ParseDuration(val); err == nil {
			cfg.Unblocker.PollInterval = Duration(d)
		}
	}
	if val, ok := os.LookupEnv("NOCTIFAB_UNBLOCKER_MAX_RETRIES"); ok {
		if i, err := strconv.Atoi(val); err == nil {
			cfg.Unblocker.MaxRetries = i
		}
	}
	if val, ok := os.LookupEnv("NOCTIFAB_UNBLOCKER_STALL_THRESHOLD"); ok {
		if d, err := time.ParseDuration(val); err == nil {
			cfg.Unblocker.StallThreshold = Duration(d)
		}
	}
	if val, ok := os.LookupEnv("NOCTIFAB_UNBLOCKER_CONFLICT_THRESHOLD"); ok {
		if d, err := time.ParseDuration(val); err == nil {
			cfg.Unblocker.ConflictThreshold = Duration(d)
		}
	}
	if val, ok := os.LookupEnv("NOCTIFAB_UNBLOCKER_LLM_ASSESSMENT"); ok {
		if b, err := strconv.ParseBool(val); err == nil {
			cfg.Unblocker.LLMAssessment = b
		}
	}
}

func splitAndTrim(s string) []string {
	if s == "" {
		return nil
	}
	parts := strings.Split(s, ",")
	result := make([]string, 0, len(parts))
	for _, p := range parts {
		v := strings.TrimSpace(p)
		if v != "" {
			result = append(result, v)
		}
	}
	return result
}

func applyFlagOverrides(cfg *Config, cmd *cobra.Command) {
	setIfChanged := func(flagName string, applyFn func(val string)) {
		if flag := cmd.Flags().Lookup(flagName); flag != nil && flag.Changed {
			applyFn(flag.Value.String())
		}
	}

	setIfChanged("db-path", func(val string) {
		cfg.Storage.ConnString = val
	})
	setIfChanged("storage-provider", func(val string) {
		cfg.Storage.Provider = val
	})
	setIfChanged("storage-conn", func(val string) {
		cfg.Storage.ConnString = val
	})
	setIfChanged("input", func(val string) {
		cfg.Input = val
	})
	setIfChanged("auto-commit", func(val string) {
		if b, err := strconv.ParseBool(val); err == nil {
			cfg.AutoCommit = b
		}
	})
	setIfChanged("agents", func(val string) {
		if i, err := strconv.Atoi(val); err == nil {
			cfg.Agents.Generators.Number = i
		}
	})
	setIfChanged("interval", func(val string) {
		if d, err := time.ParseDuration(val); err == nil {
			cfg.PollInterval = Duration(d)
		}
	})
	setIfChanged("vcs-provider", func(val string) {
		cfg.VCS.Provider = val
	})
	setIfChanged("vcs-repo", func(val string) {
		cfg.VCS.Repository = val
	})
	setIfChanged("llm-provider", func(val string) {
		cfg.LLM.Provider = val
	})
	setIfChanged("llm-model", func(val string) {
		cfg.LLM.Model = val
	})
	setIfChanged("llm-url", func(val string) {
		cfg.LLM.URL = val
	})
	setIfChanged("llm-planner-model", func(val string) {
		cfg.Roles.Planner.Model = val
	})
	setIfChanged("llm-generator-model", func(val string) {
		cfg.Roles.Generator.Model = val
	})
	setIfChanged("llm-tester-model", func(val string) {
		cfg.Roles.Tester.Model = val
	})
	setIfChanged("jira-user", func(val string) {
		cfg.Jira.User = val
	})
	setIfChanged("jira-url", func(val string) {
		cfg.Jira.URL = val
	})
	setIfChanged("http-max-retries", func(val string) {
		if i, err := strconv.Atoi(val); err == nil {
			cfg.LLM.MaxRetries = i
		}
	})
	setIfChanged("http-retry-backoff", func(val string) {
		if d, err := time.ParseDuration(val); err == nil {
			cfg.LLM.RetryBackoff = Duration(d)
		}
	})
	setIfChanged("max-tools-per-response", func(val string) {
		if i, err := strconv.Atoi(val); err == nil {
			cfg.Agents.MaxToolsPerResponse = i
		}
	})
	setIfChanged("max-actions", func(val string) {
		if i, err := strconv.Atoi(val); err == nil {
			cfg.MaxActions = i
		}
	})
	setIfChanged("max-duration", func(val string) {
		if d, err := time.ParseDuration(val); err == nil {
			cfg.MaxDuration = Duration(d)
		}
	})
	setIfChanged("conversation-mode", func(val string) {
		cfg.ConversationMode = val
	})
	setIfChanged("max-history-messages", func(val string) {
		if i, err := strconv.Atoi(val); err == nil {
			cfg.MaxHistoryMessages = i
		}
	})
	setIfChanged("compaction-threshold", func(val string) {
		if i, err := strconv.Atoi(val); err == nil {
			cfg.CompactionThreshold = i
		}
	})
	setIfChanged("max-history-tokens", func(val string) {
		if i, err := strconv.Atoi(val); err == nil {
			cfg.MaxHistoryTokens = i
		}
	})
	setIfChanged("sandbox-mode", func(val string) {
		cfg.Sandbox.Mode = val
	})
	setIfChanged("shutdown-grace-period", func(val string) {
		if d, err := time.ParseDuration(val); err == nil {
			cfg.ShutdownGracePeriod = Duration(d)
		}
	})
	setIfChanged("occ-max-retries", func(val string) {
		if i, err := strconv.Atoi(val); err == nil {
			cfg.OCCMaxRetries = i
		}
	})
	setIfChanged("occ-backoff-base", func(val string) {
		if d, err := time.ParseDuration(val); err == nil {
			cfg.OCCBackoffBase = Duration(d)
		}
	})
	setIfChanged("occ-backoff-factor", func(val string) {
		if f, err := strconv.ParseFloat(val, 64); err == nil {
			cfg.OCCBackoffFactor = f
		}
	})
	setIfChanged("token-usage-limit", func(val string) {
		if i, err := strconv.ParseInt(val, 10, 64); err == nil {
			cfg.TokenUsageLimit = i
		}
	})
	setIfChanged("log-level", func(val string) {
		cfg.LogLevel = val
	})
	setIfChanged("log-file", func(val string) {
		cfg.LogFile = val
	})
	setIfChanged("pr-auto-create", func(val string) {
		if b, err := strconv.ParseBool(val); err == nil {
			cfg.VCS.PullRequest.AutoCreate = b
		}
	})
	setIfChanged("pr-auto-merge", func(val string) {
		if b, err := strconv.ParseBool(val); err == nil {
			cfg.VCS.PullRequest.AutoMerge = b
		}
	})
	setIfChanged("pr-auto-rebase", func(val string) {
		if b, err := strconv.ParseBool(val); err == nil {
			cfg.VCS.PullRequest.AutoRebase = b
		}
	})
	setIfChanged("pr-draft", func(val string) {
		if b, err := strconv.ParseBool(val); err == nil {
			cfg.VCS.PullRequest.Draft = b
		}
	})
	setIfChanged("pr-assignees", func(val string) {
		cfg.VCS.PullRequest.Assignees = splitAndTrim(val)
	})
	setIfChanged("pr-labels", func(val string) {
		cfg.VCS.PullRequest.Labels = splitAndTrim(val)
	})
	setIfChanged("ci-auto-fix", func(val string) {
		if b, err := strconv.ParseBool(val); err == nil {
			cfg.VCS.CI.AutoFix = b
		}
	})
	setIfChanged("ci-max-retries", func(val string) {
		if i, err := strconv.Atoi(val); err == nil {
			cfg.VCS.CI.MaxRetries = i
		}
	})
	setIfChanged("unblocker-enabled", func(val string) {
		if b, err := strconv.ParseBool(val); err == nil {
			cfg.Unblocker.Enabled = b
		}
	})
	setIfChanged("unblocker-poll-interval", func(val string) {
		if d, err := time.ParseDuration(val); err == nil {
			cfg.Unblocker.PollInterval = Duration(d)
		}
	})
	setIfChanged("unblocker-max-retries", func(val string) {
		if i, err := strconv.Atoi(val); err == nil {
			cfg.Unblocker.MaxRetries = i
		}
	})
	setIfChanged("unblocker-stall-threshold", func(val string) {
		if d, err := time.ParseDuration(val); err == nil {
			cfg.Unblocker.StallThreshold = Duration(d)
		}
	})
	setIfChanged("unblocker-conflict-threshold", func(val string) {
		if d, err := time.ParseDuration(val); err == nil {
			cfg.Unblocker.ConflictThreshold = Duration(d)
		}
	})
	setIfChanged("unblocker-llm-assessment", func(val string) {
		if b, err := strconv.ParseBool(val); err == nil {
			cfg.Unblocker.LLMAssessment = b
		}
	})
}
