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
			Architecture:        "code_first_verification_loop",
			MaxToolsPerResponse: 5,
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
		PollInterval:               Duration(5 * time.Minute),
		MaxClarificationWait:       Duration(30 * time.Minute),
		ClarificationTimeoutAction: "abort",
		Storage: StorageConfig{
			Provider:     "sqlite",
			ConnString:   ".noctifab/data/noctifab.db",
			JSONFilePath: ".noctifab/data/state.json",
		},
		LLM: LLMConfig{
			Provider:           "openai",
			Model:              "gpt-4o",
			Temperature:        0.0,
			APIKeyEnv:          "",
			URL:                "",
			MaxRetries:         5,
			RetryBackoff:       Duration(100 * time.Millisecond),
			RetryBackoffFactor: 2.0,
			MaxBudgetUSD:       10.0,
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
		},
		VCS: VCSConfig{
			Provider:     "github",
			Repository:   "",
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
			ExcludePaths:       []string{".noctifab"},
			AllowedCommands:    []string{"go", "git", "npm", "python", "make"},
			AutoInstallDeps:    false,
			PackageManagers:    []string{"pip", "go", "brew", "curl", "npm"},
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
		Context: ContextConfig{
			Mode:            "full",
			DiffWindowLines: 15,
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
