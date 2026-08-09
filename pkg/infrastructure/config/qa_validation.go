package config

import (
	"fmt"
	"path"
	"strings"
	"unicode"
)

func validateQAConfig(cfg *Config) error {
	qa := cfg.Agents.QA
	if !qa.Enabled {
		return nil
	}
	if cfg.Agents.Architecture != "code_first" {
		return fmt.Errorf("agents.qa.enabled requires agents.architecture to be code_first")
	}
	if !cfg.VCS.UseWorktrees {
		return fmt.Errorf("agents.qa.enabled requires vcs.use_worktrees to be true")
	}
	if !qa.Blocking {
		return fmt.Errorf("agents.qa.blocking must be true when QA is enabled")
	}
	if qa.Network != "none" {
		return fmt.Errorf("agents.qa.network must be none when QA is enabled")
	}
	if qa.Iterations < 1 || qa.Iterations > 3 {
		return fmt.Errorf("agents.qa.iterations must be between 1 and 3, got %d", qa.Iterations)
	}
	if qa.MaxDuration <= 0 {
		return fmt.Errorf("agents.qa.max_duration must be positive")
	}
	if qa.MaxScenarios <= 0 {
		return fmt.Errorf("agents.qa.max_scenarios must be positive, got %d", qa.MaxScenarios)
	}
	if qa.MaxReviewRounds < 1 || qa.MaxReviewRounds > 5 {
		return fmt.Errorf("agents.qa.max_review_rounds must be between 1 and 5, got %d", qa.MaxReviewRounds)
	}
	if qa.MaxOutputBytes < 1024 || qa.MaxOutputBytes > 1048576 {
		return fmt.Errorf("agents.qa.max_output_bytes must be between 1024 and 1048576, got %d", qa.MaxOutputBytes)
	}
	if len(qa.BuildCommand) == 0 || strings.TrimSpace(qa.BuildCommand[0]) == "" {
		return fmt.Errorf("agents.qa.build_command must contain a non-empty executable")
	}
	if len(qa.ValidationCommands) == 0 {
		return fmt.Errorf("agents.qa.validation_commands must contain at least one executable path")
	}
	for _, command := range qa.ValidationCommands {
		if !isCleanRelativeExecutable(command) {
			return fmt.Errorf("agents.qa.validation_commands entry must be a clean relative executable path without arguments: %q", command)
		}
	}
	if len(qa.TesterPathPrefixes) == 0 {
		return fmt.Errorf("agents.qa.tester_path_prefixes must contain at least one prefix")
	}
	for _, prefix := range qa.TesterPathPrefixes {
		if !isCleanRelativePrefix(prefix) {
			return fmt.Errorf("agents.qa.tester_path_prefixes entry must be a clean relative prefix without traversal: %q", prefix)
		}
	}
	return nil
}

func isCleanRelativeExecutable(value string) bool {
	if value == "" || strings.ContainsRune(value, '\\') || strings.IndexFunc(value, unicode.IsSpace) >= 0 {
		return false
	}
	cleaned := path.Clean(value)
	if cleaned == "." || path.IsAbs(value) || hasParentSegment(value) {
		return false
	}
	return value == cleaned || value == "./"+cleaned
}

func isCleanRelativePrefix(value string) bool {
	if value == "" || strings.TrimSpace(value) != value || strings.ContainsRune(value, '\\') || path.IsAbs(value) || hasParentSegment(value) {
		return false
	}
	withoutDot := strings.TrimPrefix(value, "./")
	withoutSlash := strings.TrimSuffix(withoutDot, "/")
	return withoutSlash != "" && path.Clean(withoutSlash) == withoutSlash &&
		(value == withoutSlash || value == withoutSlash+"/" || value == "./"+withoutSlash || value == "./"+withoutSlash+"/")
}

func hasParentSegment(value string) bool {
	for _, segment := range strings.Split(value, "/") {
		if segment == ".." {
			return true
		}
	}
	return false
}
