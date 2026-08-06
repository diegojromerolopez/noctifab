package config

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"gopkg.in/yaml.v3"
)

// DefaultConfig returns a new Config populated with baseline default settings.
func DefaultConfig() *Config {
	return &Config{
		ConfigVersion: "1.0",
		Agents: AgentsConfig{
			Architecture:        "code_first",
			MaxToolsPerResponse: 5,
			ProductManager: AgentRoleConfig{
				Number:         1,
				Iterations:     2,
				MaxUserStories: 5,
			},
			Planner: AgentRoleConfig{
				Number:     1,
				Iterations: 2,
			},
			Architect: AgentRoleConfig{
				Number:     1,
				Iterations: 2,
			},
			Generators: AgentRoleConfig{
				Number:     3,
				Iterations: 5,
			},
			Testers: AgentRoleConfig{
				Number:     2,
				Iterations: 3,
			},
			QA: AgentRoleConfig{
				Number:     1,
				Iterations: 2,
			},
			Security: AgentRoleConfig{
				Number:     1,
				Iterations: 2,
			},
			Performance: AgentRoleConfig{
				Number:     1,
				Iterations: 2,
			},
			Docs: AgentRoleConfig{
				Number:     1,
				Iterations: 2,
			},
			DevOps: AgentRoleConfig{
				Number:     1,
				Iterations: 2,
			},
		},
		WorkspaceCache: WorkspaceCacheConfig{
			Enabled: boolPtr(true),
		},
		PollInterval:               Duration(5 * time.Minute),
		StoryExecInterval:          Duration(2 * time.Second),
		MaxClarificationWait:       Duration(30 * time.Minute),
		ClarificationTimeoutAction: "abort",
		Storage: StorageConfig{
			Provider:           "sqlite",
			ConnString:         ".noctifab/data/noctifab.db",
			JSONFilePath:       ".noctifab/data/state.json",
			KeepFinishedStates: 20,
		},
		LLM: LLMConfig{
			Provider:           "openai",
			Model:              "latest",
			Temperature:        0.0,
			URL:                "",
			MaxRetries:         5,
			RetryBackoff:       Duration(100 * time.Millisecond),
			RetryBackoffFactor: 2.0,
			ResetPeriod:        "daily",
			Failover: FailoverConfig{
				Enabled:      false,
				Cooldown:     Duration(5 * time.Minute),
				MaxCallLimit: 0,
				Backends:     nil,
			},
			MaxTimeout:  Duration(60 * time.Second),
			IdleTimeout: Duration(15 * time.Second),
			Streaming:   boolPtr(true),
			// skip_on_credit_exhausted: stop attempting a provider chain the
			// moment an HTTP 402 credit-limit response is detected, rather than
			// burning wall-clock time on retries and lower-model fallbacks that
			// cannot succeed without a funded key.
			SkipOnCreditExhausted: true,
		},
		VCS: VCSConfig{
			Provider:     "github",
			Repository:   "local/repo",
			BaseBranch:   "master",
			BranchPrefix: "noctifab/",
			UseWorktrees: true,
			TokenEnv:     "GITHUB_TOKEN",
			ConventionalCommits: ConventionalCommitConfig{
				Enabled:      true,
				DefaultScope: "core",
			},
			GitMutexTimeout:     Duration(30 * time.Second),
			GitOperationRetries: 3,
			GitRetryBackoff:     Duration(500 * time.Millisecond),
			PullRequest: PullRequestConfig{
				AutoCreate: false,
				AutoMerge:  false,
				AutoRebase: false,
				Draft:      false,
				Assignees:  nil,
				Labels:     nil,
			},
			CI: CIConfig{
				AutoFix:    false,
				MaxRetries: 3,
			},
		},
		Sandbox: SandboxConfig{
			Mode:               "host",
			TimeoutSeconds:     300,
			IdleTimeoutSeconds: 30,
			GracePeriodSeconds: 30,
			TestCommand:        "go test -v ./...",
			LinterCommand:      "golangci-lint run",
			FormatterCommand:   "go fmt ./...",
			MaxLinterRetries:   3,
			// MaxLinterIssues: a completed project with ≤100 style warnings is
			// far better than a permanently stalled task. Projects that need
			// stricter enforcement can set this to 0.
			MaxLinterIssues: 100,
			ExcludePaths:    []string{".noctifab"},
			AllowedCommands: []string{"go", "git", "npm", "python", "make"},
			AutoInstallDeps: false,
			PackageManagers: []string{"pip", "go", "brew", "curl", "npm"},
		},
		Roles: RolesConfig{
			Orchestrator: RoleSetting{Profile: "orchestrator", Temperature: 0.0},
			Planner:      RoleSetting{Profile: "planner", Temperature: 0.5},
			Generator:    RoleSetting{Profile: "generator", Temperature: 0.0},
			Tester:       RoleSetting{Profile: "tester", Temperature: 0.0},
		},
		Profiles:            make(map[string]ProfileConfig),
		AutoCommit:          false,
		MaxActions:          100,
		MaxDuration:         Duration(0),
		ConversationMode:    "sliding-window",
		MaxHistoryMessages:  10,
		CompactionThreshold: 15,
		MaxHistoryTokens:    4096,
		ShutdownGracePeriod: Duration(30 * time.Second),
		OCCMaxRetries:       5,
		OCCBackoffBase:      Duration(50 * time.Millisecond),
		OCCBackoffFactor:    2.0,
		TokenUsageLimit:     0,
		Telemetry: TelemetryConfig{
			Enabled:     false,
			Exporter:    "otlp",
			Endpoint:    "",
			ServiceName: "noctifab",
			Metrics: MetricsConfig{
				Enabled: boolPtr(true),
			},
		},
		SAST: SASTConfig{
			Enabled:        false,
			Scanners:       []string{"gosec"},
			FailOnSeverity: "high",
		},
		Unblocker: UnblockerConfig{
			Enabled:           true,
			PollInterval:      Duration(30 * time.Second),
			MaxRetries:        3,
			StallThreshold:    Duration(5 * time.Minute),
			ConflictThreshold: Duration(15 * time.Minute),
			LLMAssessment:     true,
		},
		Context: ContextConfig{
			Mode:            "full",
			DiffWindowLines: 15,
			Compaction:      "none",
		},
		LogLevel: "info",
	}
}

// WriteDefaultConfig writes a default configuration file to the specified path.
func WriteDefaultConfig(path string) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create config directory: %w", err)
	}

	cfg := DefaultConfig()
	data, err := yaml.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("failed to marshal default config: %w", err)
	}

	if err := os.WriteFile(path, data, 0644); err != nil {
		return fmt.Errorf("failed to write default config file: %w", err)
	}

	return nil
}

func boolPtr(b bool) *bool {
	return &b
}
