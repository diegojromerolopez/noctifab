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
		Orchestrator: OrchestratorConfig{
			MaxToolsPerResponse:        5,
			Concurrency:                3,
			PollInterval:               Duration(5 * time.Minute),
			MaxClarificationWait:       Duration(30 * time.Minute),
			ClarificationTimeoutAction: "abort",
		},
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
		},
		VCS: VCSConfig{
			Provider:     "github",
			Repository:   "",
			BaseBranch:   "master",
			BranchPrefix: "noctifab/",
			TokenEnv:     "GITHUB_TOKEN",
			ConventionalCommits: ConventionalCommitConfig{
				Enabled:      true,
				DefaultScope: "core",
			},
			GitMutexTimeout:     Duration(30 * time.Second),
			GitOperationRetries: 3,
			GitRetryBackoff:     Duration(500 * time.Millisecond),
		},
		Sandbox: SandboxConfig{
			Mode:               "host",
			TimeoutSeconds:     300,
			GracePeriodSeconds: 30,
			TestCommand:        "go test -v ./...",
			LinterCommand:      "golangci-lint run",
			FormatterCommand:   "go fmt ./...",
			ExcludePaths:       []string{"node_modules/", "vendor/", "bin/", "dist/", ".noctifab/"},
			AllowedCommands:    []string{"go", "git", "npm", "python", "make"},
		},
		Roles: RolesConfig{
			Orchestrator: RoleSetting{Profile: "orchestrator", Temperature: 0.0},
			Planner:      RoleSetting{Profile: "planner", Temperature: 0.5},
			Generator:    RoleSetting{Profile: "generator", Temperature: 0.0},
			Tester:       RoleSetting{Profile: "tester", Temperature: 0.0},
		},
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
		LogLevel:            "info",
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
