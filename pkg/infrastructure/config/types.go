package config

import (
	"time"

	"gopkg.in/yaml.v3"
)

// Duration is a wrapper around time.Duration to support YAML unmarshaling of duration strings.
type Duration time.Duration

func (d *Duration) UnmarshalYAML(value *yaml.Node) error {
	var s string
	if err := value.Decode(&s); err != nil {
		return err
	}
	dur, err := time.ParseDuration(s)
	if err != nil {
		return err
	}
	*d = Duration(dur)
	return nil
}

func (d Duration) MarshalYAML() (interface{}, error) {
	return time.Duration(d).String(), nil
}

type Config struct {
	ConfigVersion string                   `yaml:"config_version"`
	Orchestrator  OrchestratorConfig       `yaml:"orchestrator"`
	Storage       StorageConfig            `yaml:"storage"`
	LLM           LLMConfig                `yaml:"llm"`
	VCS           VCSConfig                `yaml:"vcs"`
	Sandbox       SandboxConfig            `yaml:"sandbox"`
	Roles         RolesConfig              `yaml:"roles"`
	Profiles      map[string]ProfileConfig `yaml:"profiles"`
	Jira          JiraConfig               `yaml:"jira"`

	Input               string   `yaml:"input"`
	AutoCommit          bool     `yaml:"auto_commit"`
	MaxActions          int      `yaml:"max_actions"`
	MaxDuration         Duration `yaml:"max_duration"`
	ConversationMode    string   `yaml:"conversation_mode"`
	MaxHistoryMessages  int      `yaml:"max_history_messages"`
	CompactionThreshold int      `yaml:"compaction_threshold"`
	MaxHistoryTokens    int      `yaml:"max_history_tokens"`
	ShutdownGracePeriod Duration `yaml:"shutdown_grace_period"`
	OCCMaxRetries       int      `yaml:"occ_max_retries"`
	OCCBackoffBase      Duration `yaml:"occ_backoff_base"`
	OCCBackoffFactor    float64  `yaml:"occ_backoff_factor"`
	TokenUsageLimit     int64    `yaml:"token_usage_limit"`
	LogLevel            string   `yaml:"log_level"`
	LogFile             string   `yaml:"log_file"`
}

type OrchestratorConfig struct {
	MaxToolsPerResponse        int      `yaml:"max_tools_per_response"`
	Concurrency                int      `yaml:"concurrency"`
	PollInterval               Duration `yaml:"poll_interval"`
	MaxClarificationWait       Duration `yaml:"max_clarification_wait"`
	ClarificationTimeoutAction string   `yaml:"clarification_timeout_action"`
}

type StorageConfig struct {
	Provider     string `yaml:"provider"`
	ConnString   string `yaml:"conn_string"`
	JSONFilePath string `yaml:"json_file_path"`
}

type LLMConfig struct {
	Provider           string   `yaml:"provider"`
	Model              string   `yaml:"model"`
	Temperature        float64  `yaml:"temperature"`
	APIKey             string   `yaml:"api_key"`
	APIKeyEnv          string   `yaml:"api_key_env"`
	APIKeyValue        string   `yaml:"-"`
	URL                string   `yaml:"url"`
	MaxRetries         int      `yaml:"max_retries"`
	RetryBackoff       Duration `yaml:"retry_backoff"`
	RetryBackoffFactor float64  `yaml:"retry_backoff_factor"`
	MaxBudgetUSD       float64  `yaml:"max_budget_usd"`
}

type VCSConfig struct {
	Provider            string                   `yaml:"provider"`
	Repository          string                   `yaml:"repository"`
	BaseBranch          string                   `yaml:"base_branch"`
	BranchPrefix        string                   `yaml:"branch_prefix"`
	Token               string                   `yaml:"token"`
	TokenEnv            string                   `yaml:"token_env"`
	TokenValue          string                   `yaml:"-"`
	ConventionalCommits ConventionalCommitConfig `yaml:"conventional_commits"`
	GitMutexTimeout     Duration                 `yaml:"git_mutex_timeout"`
	GitOperationRetries int                      `yaml:"git_operation_retries"`
	GitRetryBackoff     Duration                 `yaml:"git_retry_backoff"`
}

type ConventionalCommitConfig struct {
	Enabled      bool   `yaml:"enabled"`
	DefaultScope string `yaml:"default_scope"`
}

type SandboxConfig struct {
	Mode               string   `yaml:"mode"`
	TimeoutSeconds     int      `yaml:"timeout_seconds"`
	GracePeriodSeconds int      `yaml:"grace_period_seconds"`
	TestCommand        string   `yaml:"test_command"`
	LinterCommand      string   `yaml:"linter_command"`
	FormatterCommand   string   `yaml:"formatter_command"`
	ExcludePaths       []string `yaml:"exclude_paths"`
	AllowedCommands    []string `yaml:"allowed_commands"`
}

type RoleSetting struct {
	Model       string  `yaml:"model"`
	Temperature float64 `yaml:"temperature"`
	Profile     string  `yaml:"profile"`
}

type RolesConfig struct {
	Orchestrator RoleSetting `yaml:"orchestrator"`
	Planner      RoleSetting `yaml:"planner"`
	Generator    RoleSetting `yaml:"generator"`
	Tester       RoleSetting `yaml:"tester"`
}

type ProfileConfig struct {
	AllowedTools    []string `yaml:"allowed_tools"`
	AllowedCommands []string `yaml:"allowed_commands"`
}

type JiraConfig struct {
	User  string `yaml:"user"`
	Token string `yaml:"token"`
	URL   string `yaml:"url"`
}
