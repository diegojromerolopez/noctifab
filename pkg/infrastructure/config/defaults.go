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
		ConfigVersion: "2.0",
		Agents: AgentsConfig{
			Architecture:       "code_first",
			TaskExecutionOrder: "generator_first",
			ProductManager: AgentRoleConfig{
				Number:         1,
				Iterations:     2,
				MaxUserStories: 5,
				Passes:         2,
			},
			Planner: AgentRoleConfig{
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
			QA: QAConfig{
				Enabled:            false,
				Iterations:         1,
				MaxDuration:        Duration(2 * time.Minute),
				MaxScenarios:       8,
				MaxReviewRounds:    2,
				MaxOutputBytes:     65536,
				Blocking:           true,
				Network:            "none",
				BuildCommand:       []string{"make", "build"},
				ValidationCommands: nil,
				TesterPathPrefixes: []string{"test/", "tests/", "spec/", "specs/"},
			},
			LastResort: LastResortAgentConfig{
				Enabled:             true,
				Model:               "",
				Temperature:         0.1,
				MaxTurns:            2,
				Timeout:             Duration(180 * time.Second),
				AllowSpecMutation:   true,
				AllowScopeReduction: true,
				EnforceSpecQuality:  true,
			},
		},
		WorkspaceCache: WorkspaceCacheConfig{
			Enabled: boolPtr(true),
		},
		Spec: SpecConfig{
			OutputFile:          "SPEC.md",
			ConsensusAudit:      boolPtr(true),
			MaxHistoryTurns:     10,
			AutoGenerateRoadmap: boolPtr(true),
		},
		PollInterval:               Duration(5 * time.Minute),
		StoryExecInterval:          Duration(2 * time.Second),
		MaxClarificationWait:       Duration(30 * time.Minute),
		ClarificationTimeoutAction: "abort",
		Runtime: RuntimeConfig{
			SpecSource:             "",
			MaxActions:             100,
			MaxDuration:            Duration(0),
			MaxSilentStallDuration: Duration(30 * time.Minute),
			MaxTokensPerStory:      0,
			MaxTokensPerTask:       0,
			MaxTokens:              100000000,
			Loops:                  1,
			Loop:                   LoopConfig{Count: 1},
		},
		Storage: StorageConfig{
			Provider:           "sqlite",
			ConnString:         ".noctifab/data/noctifab.db",
			JSONFilePath:       ".noctifab/data/state.json",
			KeepFinishedStates: 20,
			OCC: OCCConfig{
				MaxRetries:    20,
				BackoffBase:   Duration(200 * time.Millisecond),
				BackoffFactor: 2.0,
			},
		},
		LLM: LLMConfig{
			Provider:           "openai",
			Model:              "latest",
			Temperature:        0.0,
			URL:                "",
			MaxRetries:         5,
			RetryBackoff:       Duration(100 * time.Millisecond),
			RetryBackoffFactor: 2.0,
			TokenUsageLimit:    0,
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
			BaseBranch:   "auto",
			CreateBranch: boolPtr(true),
			BranchPrefix: "noctifab/",
			UseWorktrees: true,
			TokenEnv:     "GITHUB_TOKEN",
			PullRequest: PullRequestConfig{
				AutoCreate: false,
				AutoMerge:  false,
				AutoRebase: false,
				Draft:      false,
				Assignees:  nil,
				Labels:     nil,
			},
		},
		Sandbox: SandboxConfig{
			Mode:               "host",
			TimeoutSeconds:     300,
			IdleTimeoutSeconds: 30,
			TestCommand:        "go test -v ./...",
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
			LastResort:   RoleSetting{Profile: "last_resort", Temperature: 0.1},
		},
		Profiles: make(map[string]ProfileConfig),
		Logging: LoggingConfig{
			Level: "info",
			File:  "",
		},
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
			LastResortTriggers: LastResortTriggersConfig{
				RetriesExhaustion:         true,
				CyclicLoopDetection:       true,
				MissingToolchainFastAbort: true,
				QADeadlockTurns:           2,
				WatchdogTimeoutTurns:      2,
				StallCountThreshold:       4,
			},
		},
		Context: ContextConfig{
			Mode:            "full",
			DiffWindowLines: 15,
			Compaction:      "none",
		},
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
