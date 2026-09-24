package config

import "strings"

// E2EConfig defines configuration for end-to-end acceptance testing.
type E2EConfig struct {
	Mode    string `yaml:"mode"`    // "docker" (default) or "native"
	Command string `yaml:"command"` // optional explicit override
}

type LinterConfig struct {
	Command             *string `yaml:"command,omitempty"`
	MaxIssues           *int    `yaml:"max_issues,omitempty"`
	ConsecutiveFailures *int    `yaml:"consecutive_failures,omitempty"`
	MaxRetries          *int    `yaml:"max_retries,omitempty"`
}

type SandboxConfig struct {
	Mode                  string       `yaml:"mode"`
	TimeoutSeconds        int          `yaml:"timeout_seconds"`
	IdleTimeoutSeconds    int          `yaml:"idle_timeout_seconds"`
	PerTestTimeoutSeconds int          `yaml:"per_test_timeout_seconds"`
	TestCommand           string       `yaml:"test_command"`
	FormatterCommand      string       `yaml:"formatter_command"`
	SyntaxCheckCommand    string       `yaml:"syntax_check_command"`
	Linter                LinterConfig `yaml:"linter"`
	E2E                   E2EConfig    `yaml:"e2e"`
	// Legacy flat fields for backward compatibility
	LinterCommand                *string  `yaml:"linter_command,omitempty"`
	MaxLinterRetries             *int     `yaml:"max_linter_retries,omitempty"`
	MaxLinterIssues              *int     `yaml:"max_linter_issues,omitempty"`
	MaxLinterConsecutiveFailures *int     `yaml:"max_linter_consecutive_failures,omitempty"`
	ExcludePaths                 []string `yaml:"exclude_paths"`
	SkipFolders                  []string `yaml:"skip_folders"`
	AllowedCommands              []string `yaml:"allowed_commands"`
	AutoInstallDeps              bool     `yaml:"auto_install_deps"`
	PackageManagers              []string `yaml:"package_managers"`
	ForbiddenPatterns            []string `yaml:"forbidden_patterns"`
}

func (s SandboxConfig) GetLinterCommand() string {
	if s.Linter.Command != nil {
		return *s.Linter.Command
	}
	if s.LinterCommand != nil {
		return *s.LinterCommand
	}
	return "golangci-lint run"
}

func (s SandboxConfig) GetMaxLinterIssues() int {
	if s.Linter.MaxIssues != nil {
		return *s.Linter.MaxIssues
	}
	if s.MaxLinterIssues != nil {
		return *s.MaxLinterIssues
	}
	return 100
}

func (s SandboxConfig) GetMaxLinterConsecutiveFailures() int {
	if s.Linter.ConsecutiveFailures != nil {
		return *s.Linter.ConsecutiveFailures
	}
	if s.MaxLinterConsecutiveFailures != nil {
		return *s.MaxLinterConsecutiveFailures
	}
	return 2
}

func (s SandboxConfig) GetMaxLinterRetries() int {
	if s.Linter.MaxRetries != nil {
		return *s.Linter.MaxRetries
	}
	if s.MaxLinterRetries != nil {
		return *s.MaxLinterRetries
	}
	return 3
}

// GetE2EMode returns the configured E2E mode ("docker" or "native"), defaulting to "docker".
func (s SandboxConfig) GetE2EMode() string {
	if s.E2E.Mode != "" {
		return strings.ToLower(strings.TrimSpace(s.E2E.Mode))
	}
	return "docker"
}

// GetE2ECommand returns the configured custom E2E command override.
func (s SandboxConfig) GetE2ECommand() string {
	return strings.TrimSpace(s.E2E.Command)
}
