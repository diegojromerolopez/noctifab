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
	if val, ok := os.LookupEnv("NOCTIFAB_SPEC_SOURCE"); ok {
		cfg.Runtime.SpecSource = val
	} else if val, ok := os.LookupEnv("NOCTIFAB_INPUT"); ok {
		cfg.Runtime.SpecSource = val
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
	if val, ok := os.LookupEnv("NOCTIFAB_MAX_ACTIONS"); ok {
		if i, err := strconv.Atoi(val); err == nil {
			cfg.Runtime.MaxActions = i
		}
	}
	if val, ok := os.LookupEnv("NOCTIFAB_MAX_DURATION"); ok {
		if d, err := time.ParseDuration(val); err == nil {
			cfg.Runtime.MaxDuration = Duration(d)
		}
	}
	if val, ok := os.LookupEnv("NOCTIFAB_SANDBOX_MODE"); ok {
		cfg.Sandbox.Mode = val
	}
	if val, ok := os.LookupEnv("NOCTIFAB_OCC_MAX_RETRIES"); ok {
		if i, err := strconv.Atoi(val); err == nil {
			cfg.Storage.OCC.MaxRetries = i
		}
	}
	if val, ok := os.LookupEnv("NOCTIFAB_OCC_BACKOFF_BASE"); ok {
		if d, err := time.ParseDuration(val); err == nil {
			cfg.Storage.OCC.BackoffBase = Duration(d)
		}
	}
	if val, ok := os.LookupEnv("NOCTIFAB_OCC_BACKOFF_FACTOR"); ok {
		if f, err := strconv.ParseFloat(val, 64); err == nil {
			cfg.Storage.OCC.BackoffFactor = f
		}
	}
	if val, ok := os.LookupEnv("NOCTIFAB_TOKEN_USAGE_LIMIT"); ok {
		if i, err := strconv.ParseInt(val, 10, 64); err == nil {
			cfg.LLM.TokenUsageLimit = i
		}
	}
	if val, ok := os.LookupEnv("NOCTIFAB_LOG_LEVEL"); ok {
		cfg.Logging.Level = val
	}
	if val, ok := os.LookupEnv("NOCTIFAB_LOG_FILE"); ok {
		cfg.Logging.File = val
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
	if val, ok := lookupEnvOrFallback("NOCTIFAB_FALLBACK_ENABLED", "NOCTIFAB_UNBLOCKER_ENABLED"); ok {
		if b, err := strconv.ParseBool(val); err == nil {
			cfg.Fallback.Enabled = b
			cfg.Unblocker.Enabled = b
		}
	}
	if val, ok := lookupEnvOrFallback("NOCTIFAB_FALLBACK_POLL_INTERVAL", "NOCTIFAB_UNBLOCKER_POLL_INTERVAL"); ok {
		if d, err := time.ParseDuration(val); err == nil {
			cfg.Fallback.PollInterval = Duration(d)
			cfg.Unblocker.PollInterval = Duration(d)
		}
	}
	if val, ok := lookupEnvOrFallback("NOCTIFAB_FALLBACK_MAX_RETRIES", "NOCTIFAB_UNBLOCKER_MAX_RETRIES"); ok {
		if i, err := strconv.Atoi(val); err == nil {
			cfg.Fallback.MaxRetries = i
			cfg.Unblocker.MaxRetries = i
		}
	}
	if val, ok := lookupEnvOrFallback("NOCTIFAB_FALLBACK_STALL_THRESHOLD", "NOCTIFAB_UNBLOCKER_STALL_THRESHOLD"); ok {
		if d, err := time.ParseDuration(val); err == nil {
			cfg.Fallback.StallThreshold = Duration(d)
			cfg.Unblocker.StallThreshold = Duration(d)
		}
	}
	if val, ok := lookupEnvOrFallback("NOCTIFAB_FALLBACK_CONFLICT_THRESHOLD", "NOCTIFAB_UNBLOCKER_CONFLICT_THRESHOLD"); ok {
		if d, err := time.ParseDuration(val); err == nil {
			cfg.Fallback.ConflictThreshold = Duration(d)
			cfg.Unblocker.ConflictThreshold = Duration(d)
		}
	}
	if val, ok := lookupEnvOrFallback("NOCTIFAB_FALLBACK_LLM_ASSESSMENT", "NOCTIFAB_UNBLOCKER_LLM_ASSESSMENT"); ok {
		if b, err := strconv.ParseBool(val); err == nil {
			cfg.Fallback.LLMAssessment = b
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
	setIfChanged("spec-source", func(val string) {
		cfg.Runtime.SpecSource = val
	})
	setIfChanged("input", func(val string) {
		if !cmd.Flags().Changed("spec-source") {
			cfg.Runtime.SpecSource = val
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
	setIfChanged("max-actions", func(val string) {
		if i, err := strconv.Atoi(val); err == nil {
			cfg.Runtime.MaxActions = i
		}
	})
	setIfChanged("max-duration", func(val string) {
		if d, err := time.ParseDuration(val); err == nil {
			cfg.Runtime.MaxDuration = Duration(d)
		}
	})
	setIfChanged("sandbox-mode", func(val string) {
		cfg.Sandbox.Mode = val
	})
	setIfChanged("occ-max-retries", func(val string) {
		if i, err := strconv.Atoi(val); err == nil {
			cfg.Storage.OCC.MaxRetries = i
		}
	})
	setIfChanged("occ-backoff-base", func(val string) {
		if d, err := time.ParseDuration(val); err == nil {
			cfg.Storage.OCC.BackoffBase = Duration(d)
		}
	})
	setIfChanged("occ-backoff-factor", func(val string) {
		if f, err := strconv.ParseFloat(val, 64); err == nil {
			cfg.Storage.OCC.BackoffFactor = f
		}
	})
	setIfChanged("token-usage-limit", func(val string) {
		if i, err := strconv.ParseInt(val, 10, 64); err == nil {
			cfg.LLM.TokenUsageLimit = i
		}
	})
	setIfChanged("log-level", func(val string) {
		cfg.Logging.Level = val
	})
	setIfChanged("log-file", func(val string) {
		cfg.Logging.File = val
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
	setIfChangedEither := func(canonical, alias string, apply func(val string)) {
		setIfChanged(canonical, apply)
		setIfChanged(alias, apply)
	}

	setIfChangedEither("fallback-enabled", "unblocker-enabled", func(val string) {
		if b, err := strconv.ParseBool(val); err == nil {
			cfg.Fallback.Enabled = b
			cfg.Unblocker.Enabled = b
		}
	})
	setIfChangedEither("fallback-poll-interval", "unblocker-poll-interval", func(val string) {
		if d, err := time.ParseDuration(val); err == nil {
			cfg.Fallback.PollInterval = Duration(d)
			cfg.Unblocker.PollInterval = Duration(d)
		}
	})
	setIfChangedEither("fallback-max-retries", "unblocker-max-retries", func(val string) {
		if i, err := strconv.Atoi(val); err == nil {
			cfg.Fallback.MaxRetries = i
			cfg.Unblocker.MaxRetries = i
		}
	})
	setIfChangedEither("fallback-stall-threshold", "unblocker-stall-threshold", func(val string) {
		if d, err := time.ParseDuration(val); err == nil {
			cfg.Fallback.StallThreshold = Duration(d)
			cfg.Unblocker.StallThreshold = Duration(d)
		}
	})
	setIfChangedEither("fallback-conflict-threshold", "unblocker-conflict-threshold", func(val string) {
		if d, err := time.ParseDuration(val); err == nil {
			cfg.Fallback.ConflictThreshold = Duration(d)
			cfg.Unblocker.ConflictThreshold = Duration(d)
		}
	})
	setIfChangedEither("fallback-llm-assessment", "unblocker-llm-assessment", func(val string) {
		if b, err := strconv.ParseBool(val); err == nil {
			cfg.Fallback.LLMAssessment = b
			cfg.Unblocker.LLMAssessment = b
		}
	})
}

func lookupEnvOrFallback(canonical, alias string) (string, bool) {
	if val, ok := os.LookupEnv(canonical); ok {
		return val, true
	}
	return os.LookupEnv(alias)
}
