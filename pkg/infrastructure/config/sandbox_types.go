package config

type LinterConfig struct {
	Command             string `yaml:"command,omitempty"`
	MaxIssues           int    `yaml:"max_issues,omitempty"`
	ConsecutiveFailures int    `yaml:"consecutive_failures,omitempty"`
	MaxRetries          int    `yaml:"max_retries,omitempty"`
}

type SandboxConfig struct {
	Mode               string       `yaml:"mode"`
	TimeoutSeconds     int          `yaml:"timeout_seconds"`
	IdleTimeoutSeconds int          `yaml:"idle_timeout_seconds"`
	TestCommand        string       `yaml:"test_command"`
	FormatterCommand   string       `yaml:"formatter_command"`
	Linter             LinterConfig `yaml:"linter"`
	// Legacy flat fields for backward compatibility
	LinterCommand                string   `yaml:"linter_command,omitempty"`
	MaxLinterRetries             int      `yaml:"max_linter_retries,omitempty"`
	MaxLinterIssues              int      `yaml:"max_linter_issues,omitempty"`
	MaxLinterConsecutiveFailures int      `yaml:"max_linter_consecutive_failures,omitempty"`
	ExcludePaths                 []string `yaml:"exclude_paths"`
	AllowedCommands              []string `yaml:"allowed_commands"`
	AutoInstallDeps              bool     `yaml:"auto_install_deps"`
	PackageManagers              []string `yaml:"package_managers"`
	ForbiddenPatterns            []string `yaml:"forbidden_patterns"`
}

func (s SandboxConfig) GetLinterCommand() string {
	if s.Linter.Command != "" && s.Linter.Command != "golangci-lint run" {
		return s.Linter.Command
	}
	if s.LinterCommand != "" {
		return s.LinterCommand
	}
	if s.Linter.Command != "" {
		return s.Linter.Command
	}
	return "golangci-lint run"
}

func (s SandboxConfig) GetMaxLinterIssues() int {
	if s.Linter.MaxIssues != 0 && s.Linter.MaxIssues != 100 {
		return s.Linter.MaxIssues
	}
	if s.MaxLinterIssues != 0 {
		return s.MaxLinterIssues
	}
	if s.Linter.MaxIssues != 0 {
		return s.Linter.MaxIssues
	}
	return 100
}

func (s SandboxConfig) GetMaxLinterConsecutiveFailures() int {
	if s.Linter.ConsecutiveFailures > 0 && s.Linter.ConsecutiveFailures != 2 {
		return s.Linter.ConsecutiveFailures
	}
	if s.MaxLinterConsecutiveFailures > 0 {
		return s.MaxLinterConsecutiveFailures
	}
	if s.Linter.ConsecutiveFailures > 0 {
		return s.Linter.ConsecutiveFailures
	}
	return 2
}

func (s SandboxConfig) GetMaxLinterRetries() int {
	if s.Linter.MaxRetries > 0 && s.Linter.MaxRetries != 3 {
		return s.Linter.MaxRetries
	}
	if s.MaxLinterRetries > 0 {
		return s.MaxLinterRetries
	}
	if s.Linter.MaxRetries > 0 {
		return s.Linter.MaxRetries
	}
	return 3
}
