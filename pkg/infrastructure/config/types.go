package config

import (
	"strings"
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
	ConfigVersion  string                   `yaml:"config_version"`
	Agents         AgentsConfig             `yaml:"agents"`
	Storage        StorageConfig            `yaml:"storage"`
	LLM            LLMConfig                `yaml:"llm"`
	LLMs           []LLMConfig              `yaml:"llms"`
	VCS            VCSConfig                `yaml:"vcs"`
	Sandbox        SandboxConfig            `yaml:"sandbox"`
	Roles          RolesConfig              `yaml:"roles"`
	Profiles       map[string]ProfileConfig `yaml:"profiles"`
	Jira           JiraConfig               `yaml:"jira"`
	Telemetry      TelemetryConfig          `yaml:"telemetry"`
	SAST           SASTConfig               `yaml:"sast"`
	Unblocker      UnblockerConfig          `yaml:"unblocker"`
	Context        ContextConfig            `yaml:"context"`
	WorkspaceCache WorkspaceCacheConfig     `yaml:"workspace_cache"`
	// Prompts holds per-agent, per-action prompt customizations
	// (agent -> action -> override). See pkg/infrastructure/prompts for the
	// (agent, action) catalog and docs/prompts.md for usage.
	Prompts map[string]map[string]PromptOverride `yaml:"prompts,omitempty"`

	PollInterval Duration `yaml:"poll_interval"`
	// StoryExecInterval is the tick frequency of the story execution loop
	// (how often RunOnce is retried while a story is executing). Defaults to
	// 2s; the coarser PollInterval governs server-wide idle polling.
	StoryExecInterval          Duration `yaml:"story_exec_interval"`
	MaxClarificationWait       Duration `yaml:"max_clarification_wait"`
	ClarificationTimeoutAction string   `yaml:"clarification_timeout_action"`

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
	ExecutionReport     string   `yaml:"execution_report,omitempty"`
}

// PromptOverride customizes the prompt template of one agent action.
type PromptOverride struct {
	// Path is a full-template override file (absolute or relative to the
	// project workspace). It replaces the entire default prompt body.
	Path string `yaml:"path,omitempty"`
	// Append is a string appended verbatim to the END of the default prompt
	// body. It never applies to a full-template override.
	Append string `yaml:"append,omitempty"`
}

type AgentsConfig struct {
	Architecture        string               `yaml:"architecture"`
	TaskExecutionOrder  string               `yaml:"task_execution_order,omitempty"`
	MaxToolsPerResponse int                  `yaml:"max_tools_per_response"`
	Orchestrator        AgentRoleConfig      `yaml:"orchestrator"`
	ProductManager      AgentRoleConfig      `yaml:"product_manager"`
	Planner             AgentRoleConfig      `yaml:"planner"`
	Generators          AgentRoleConfig      `yaml:"generators"`
	Testers             AgentRoleConfig      `yaml:"testers"`
	QA                  QAConfig             `yaml:"qa"`
	Unblocker           AgentRoleConfig      `yaml:"unblocker"`
	WorkspaceCache      WorkspaceCacheConfig `yaml:"workspace_cache"`
}

// QAConfig reserves the bounded configuration contract for the experimental QA role.
// Phase 0 exposes capability state only; QA runtime behavior is implemented later.
type QAConfig struct {
	Enabled            bool               `yaml:"enabled"`
	Iterations         int                `yaml:"iterations"`
	MaxDuration        Duration           `yaml:"max_duration"`
	MaxScenarios       int                `yaml:"max_scenarios"`
	MaxReviewRounds    int                `yaml:"max_review_rounds"`
	MaxOutputBytes     int                `yaml:"max_output_bytes"`
	Blocking           bool               `yaml:"blocking"`
	Network            string             `yaml:"network"`
	BuildCommand       []string           `yaml:"build_command"`
	ValidationCommands []string           `yaml:"validation_commands"`
	TesterPathPrefixes []string           `yaml:"tester_path_prefixes"`
	Model              string             `yaml:"model,omitempty"`
	Temperature        float64            `yaml:"temperature,omitempty"`
	Profile            string             `yaml:"profile,omitempty"`
	Providers          []AgentProviderRef `yaml:"providers,omitempty"`
}

type AgentRoleConfig struct {
	Number         int                `yaml:"number"`
	Iterations     int                `yaml:"iterations"`
	Model          string             `yaml:"model,omitempty"`
	Temperature    float64            `yaml:"temperature,omitempty"`
	Profile        string             `yaml:"profile,omitempty"`
	Providers      []AgentProviderRef `yaml:"providers,omitempty"`
	MaxUserStories int                `yaml:"max_user_stories,omitempty"`
}

type StorageConfig struct {
	Provider     string `yaml:"provider"`
	ConnString   string `yaml:"conn_string"`
	JSONFilePath string `yaml:"json_file_path"`
	// KeepFinishedStates bounds how many terminal (SUCCESS/FAILED) story
	// states are retained; older ones are pruned on daemon startup
	// (0 = built-in default of 20, negative = never prune).
	KeepFinishedStates int `yaml:"keep_finished_states"`
}

type FailoverConfig struct {
	Enabled      bool              `yaml:"enabled"`
	Cooldown     Duration          `yaml:"cooldown"`
	MaxCallLimit int               `yaml:"max_call_limit"`
	Backends     []FailoverBackend `yaml:"backends"`
}

type FailoverBackend struct {
	Provider    string   `yaml:"provider"`
	Model       string   `yaml:"model"`
	APIKeys     APIKeys  `yaml:"api_keys"`
	URL         string   `yaml:"url"`
	MaxRetries  int      `yaml:"max_retries"`
	IdleTimeout Duration `yaml:"idle_timeout"`
	Streaming   *bool    `yaml:"streaming"`
}

// APIKeys represents one or more API key secret source names, supporting a scalar string or a slice of strings.
type APIKeys []string

func (a *APIKeys) UnmarshalYAML(value *yaml.Node) error {
	if value.Kind == yaml.ScalarNode {
		var s string
		if err := value.Decode(&s); err != nil {
			return err
		}
		if strings.TrimSpace(s) != "" {
			*a = APIKeys{strings.TrimSpace(s)}
		}
		return nil
	}
	if value.Kind == yaml.SequenceNode {
		var list []string
		if err := value.Decode(&list); err != nil {
			return err
		}
		var clean []string
		for _, item := range list {
			if strings.TrimSpace(item) != "" {
				clean = append(clean, strings.TrimSpace(item))
			}
		}
		*a = APIKeys(clean)
		return nil
	}
	return nil
}

type ProviderSpec struct {
	Name               string   `yaml:"name"`
	Provider           string   `yaml:"provider"`
	Model              string   `yaml:"model,omitempty"`
	APIKey             string   `yaml:"api_key,omitempty"`
	APIKeys            APIKeys  `yaml:"api_keys,omitempty"`
	APIKeyValue        string   `yaml:"-"`
	APIKeyPool         []string `yaml:"-"`
	URL                string   `yaml:"url,omitempty"`
	MaxRetries         int      `yaml:"max_retries,omitempty"`
	RetryBackoff       Duration `yaml:"retry_backoff,omitempty"`
	RetryBackoffFactor float64  `yaml:"retry_backoff_factor,omitempty"`
	MaxTimeout         Duration `yaml:"max_timeout,omitempty"`
	IdleTimeout        Duration `yaml:"idle_timeout,omitempty"`
	MaxTokens          int      `yaml:"max_tokens,omitempty"`
	Temperature        float64  `yaml:"temperature,omitempty"`
	Streaming          *bool    `yaml:"streaming,omitempty"`
	// DisableJSONMode disables response_format=json_object for this provider.
	// Required when the provider/model does not support forced JSON mode (e.g.
	// QwenCloud thinking models, which output a reasoning trace before the JSON
	// payload and reject the json_object response format). When true, the LLM
	// client skips the response_format field entirely and relies on
	// ExtractJSONBlock to parse the JSON envelope from the raw response.
	DisableJSONMode bool `yaml:"disable_json_mode,omitempty"`
	// EnableThinking enables chain-of-thought / reasoning output (e.g. QwenCloud thinking mode).
	EnableThinking *bool `yaml:"enable_thinking,omitempty"`
	// ThinkingBudget caps the reasoning token budget (e.g. for QwenCloud thinking models).
	ThinkingBudget *int `yaml:"thinking_budget,omitempty"`
	// ExtraParams holds provider-specific extra body parameters passed verbatim
	// in the API request.
	ExtraParams map[string]string `yaml:"extra_params,omitempty"`
}

type LLMConfig struct {
	Priority              []string       `yaml:"priority,omitempty"`
	Providers             []ProviderSpec `yaml:"providers,omitempty"`
	Provider              string         `yaml:"provider"`
	Model                 string         `yaml:"model"`
	Temperature           float64        `yaml:"temperature"`
	APIKey                string         `yaml:"api_key"`
	APIKeys               APIKeys        `yaml:"api_keys"`
	APIKeyValue           string         `yaml:"-"`
	APIKeyPool            []string       `yaml:"-"`
	URL                   string         `yaml:"url"`
	MaxRetries            int            `yaml:"max_retries"`
	RetryBackoff          Duration       `yaml:"retry_backoff"`
	RetryBackoffFactor    float64        `yaml:"retry_backoff_factor"`
	ResetPeriod           string         `yaml:"reset_period"`
	Failover              FailoverConfig `yaml:"failover"`
	SkipOnCreditExhausted bool           `yaml:"skip_on_credit_exhausted"`
	MaxTimeout            Duration       `yaml:"max_timeout"`
	IdleTimeout           Duration       `yaml:"idle_timeout"`
	MaxTokens             int            `yaml:"max_tokens"`
	Streaming             *bool          `yaml:"streaming"`
	// MaxPromptTokens is a pre-send cap on the estimated token size of
	// outgoing prompts (0 = built-in default of 262144, negative = disabled).
	MaxPromptTokens int64 `yaml:"max_prompt_tokens"`
}

type PullRequestConfig struct {
	AutoCreate bool     `yaml:"auto_create"`
	AutoMerge  bool     `yaml:"auto_merge"`
	AutoRebase bool     `yaml:"auto_rebase"`
	Draft      bool     `yaml:"draft"`
	Assignees  []string `yaml:"assignees"`
	Labels     []string `yaml:"labels"`
}

type CIConfig struct {
	AutoFix    bool `yaml:"auto_fix"`
	MaxRetries int  `yaml:"max_retries"`
}

type VCSConfig struct {
	Provider            string                   `yaml:"provider"`
	Repository          string                   `yaml:"repository"`
	BaseBranch          string                   `yaml:"base_branch"`
	BranchPrefix        string                   `yaml:"branch_prefix"`
	UseWorktrees        bool                     `yaml:"use_worktrees"`
	Token               string                   `yaml:"token"`
	TokenEnv            string                   `yaml:"token_env"`
	TokenValue          string                   `yaml:"-"`
	ConventionalCommits ConventionalCommitConfig `yaml:"conventional_commits"`
	GitMutexTimeout     Duration                 `yaml:"git_mutex_timeout"`
	GitOperationRetries int                      `yaml:"git_operation_retries"`
	GitRetryBackoff     Duration                 `yaml:"git_retry_backoff"`
	PullRequest         PullRequestConfig        `yaml:"pull_request"`
	CI                  CIConfig                 `yaml:"ci"`
}

type ConventionalCommitConfig struct {
	Enabled      bool   `yaml:"enabled"`
	DefaultScope string `yaml:"default_scope"`
}

type SandboxConfig struct {
	Mode               string `yaml:"mode"`
	TimeoutSeconds     int    `yaml:"timeout_seconds"`
	IdleTimeoutSeconds int    `yaml:"idle_timeout_seconds"`
	GracePeriodSeconds int    `yaml:"grace_period_seconds"`
	TestCommand        string `yaml:"test_command"`
	LinterCommand      string `yaml:"linter_command"`
	FormatterCommand   string `yaml:"formatter_command"`
	MaxLinterRetries   int    `yaml:"max_linter_retries"`
	// MaxLinterIssues is the maximum number of linter issues tolerated before
	// task validation fails. 0 means strict (zero issues allowed). -1 means
	// disabled (never fail on linter issues). Default is 100, which allows
	// minor style issues to accumulate without blocking task completion —
	// a completed project with some linter warnings is preferable to a
	// permanently stalled task.
	MaxLinterIssues   int      `yaml:"max_linter_issues"`
	ExcludePaths      []string `yaml:"exclude_paths"`
	AllowedCommands   []string `yaml:"allowed_commands"`
	AutoInstallDeps   bool     `yaml:"auto_install_deps"`
	PackageManagers   []string `yaml:"package_managers"`
	ForbiddenPatterns []string `yaml:"forbidden_patterns"`
}

type AgentProviderRef struct {
	Name           string   `yaml:"name,omitempty"`
	Provider       string   `yaml:"provider,omitempty"`
	Model          string   `yaml:"model,omitempty"`
	Models         []string `yaml:"models,omitempty"`
	EnableThinking *bool    `yaml:"enable_thinking,omitempty"`
	ThinkingBudget *int     `yaml:"thinking_budget,omitempty"`
}

type RoleSetting struct {
	Model       string             `yaml:"model,omitempty"`
	Temperature float64            `yaml:"temperature"`
	Profile     string             `yaml:"profile,omitempty"`
	Providers   []AgentProviderRef `yaml:"providers,omitempty"`
}

type RolesConfig struct {
	Orchestrator RoleSetting `yaml:"orchestrator"`
	Planner      RoleSetting `yaml:"planner"`
	Generator    RoleSetting `yaml:"generator"`
	Tester       RoleSetting `yaml:"tester"`
	QA           RoleSetting `yaml:"qa"`
	Unblocker    RoleSetting `yaml:"unblocker"`
}

type ProfileConfig struct {
	AllowedTools    []string `yaml:"allowed_tools"`
	AllowedCommands []string `yaml:"allowed_commands"`
}

type MetricsConfig struct {
	Enabled *bool `yaml:"enabled"`
}

func (m MetricsConfig) IsEnabled() bool {
	if m.Enabled == nil {
		return true
	}
	return *m.Enabled
}

type TelemetryConfig struct {
	Enabled     bool          `yaml:"enabled"`
	Exporter    string        `yaml:"exporter"`
	Endpoint    string        `yaml:"endpoint"`
	ServiceName string        `yaml:"service_name"`
	Metrics     MetricsConfig `yaml:"metrics"`
}

type JiraConfig struct {
	User  string `yaml:"user"`
	Token string `yaml:"token"`
	URL   string `yaml:"url"`
}

type SASTConfig struct {
	Enabled        bool     `yaml:"enabled"`
	Scanners       []string `yaml:"scanners"`
	FailOnSeverity string   `yaml:"fail_on_severity"`
}

// UnblockerConfig controls the autonomous unblocker agent that periodically
// scans for stalled or blocked tasks/agents and injects corrective interventions.
type UnblockerConfig struct {
	// Enabled activates the unblocker goroutine (default: true).
	Enabled bool `yaml:"enabled"`
	// PollInterval defines how often the unblocker wakes up to scan for stalls (default: 30s).
	PollInterval Duration `yaml:"poll_interval"`
	// MaxRetries defines the maximum number of unblock/reset attempts before permanently failing a task (default: 3).
	MaxRetries int `yaml:"max_retries"`
	// StallThreshold is how long a task must be frozen IN_PROGRESS before being
	// considered stalled (default: 5m).
	StallThreshold Duration `yaml:"stall_threshold"`
	// ConflictThreshold is how long a CONFLICT_BLOCKED task waits before the
	// unblocker intervenes (default: 15m).
	ConflictThreshold Duration `yaml:"conflict_threshold"`
	// LLMAssessment enables LLM-based root-cause diagnosis of stalls. When false,
	// the unblocker applies heuristic-only corrections without calling the LLM
	// (cheaper, but less precise) (default: true).
	LLMAssessment bool `yaml:"llm_assessment"`
}

type ContextMode string

const (
	ContextModeFull       ContextMode = "full"
	ContextModeDiffWindow ContextMode = "diff_window"
	ContextModeTreeSitter ContextMode = "tree_sitter"
)

type ContextConfig struct {
	Mode              string `yaml:"mode"`
	DiffWindowLines   int    `yaml:"diff_window_lines"`
	CavemanCompaction bool   `yaml:"caveman_compaction"`
	Compaction        string `yaml:"compaction"` // Options: "none" (default), "simple_english", "caveman"
}

func (c ContextConfig) GetCompactionMode() string {
	mode := strings.ToLower(strings.TrimSpace(c.Compaction))
	if mode != "" {
		return mode
	}
	if c.CavemanCompaction {
		return "caveman"
	}
	return "none"
}

func (c ContextConfig) GetMode() ContextMode {
	switch ContextMode(strings.ToLower(strings.TrimSpace(c.Mode))) {
	case ContextModeDiffWindow:
		return ContextModeDiffWindow
	case ContextModeTreeSitter:
		return ContextModeTreeSitter
	default:
		return ContextModeFull
	}
}

type WorkspaceCacheConfig struct {
	Enabled *bool `yaml:"enabled"`
}

func (c *Config) GetWorkspaceCache() WorkspaceCacheConfig {
	if c.WorkspaceCache.Enabled != nil {
		return c.WorkspaceCache
	}
	if c.Agents.WorkspaceCache.Enabled != nil {
		return c.Agents.WorkspaceCache
	}
	return c.WorkspaceCache
}

func (w WorkspaceCacheConfig) IsEnabled() bool {
	if w.Enabled == nil {
		return true // Default: true
	}
	return *w.Enabled
}
