package services

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/diegojromerolopez/noctifab/pkg/domain"
)

type contextKey string

const AgentRoleKey contextKey = "agent_role"

// ValidationResult records the outcome of a security check or target validation check.
type ValidationResult struct {
	Allowed  bool     `json:"allowed"`
	Warnings []string `json:"warnings,omitempty"`
	Reason   string   `json:"reason,omitempty"` // If blocked
}

// Validator defines security filters and holds the gate checking overall project goals.
type Validator interface {
	// Validate verifies that a specific agent action complies with sandbox policies.
	Validate(ctx context.Context, action domain.Action, state *domain.State) (*ValidationResult, error)

	// EvaluateGoals checks if all tasks and acceptance criteria in the state pass.
	EvaluateGoals(ctx context.Context, state *domain.State) (bool, error)
}

// ProfileConfig defines tool/command permissions (same structure as config).
type ProfileConfig struct {
	AllowedTools    []string `yaml:"allowed_tools"`
	AllowedCommands []string `yaml:"allowed_commands"`
}

var defaultRoleProfiles = map[string]ProfileConfig{
	"orchestrator": {
		AllowedTools:    []string{"*"},
		AllowedCommands: []string{"*"},
	},
	"planner": {
		AllowedTools:    []string{"add_task", "log_message", "noop"},
		AllowedCommands: []string{},
	},
	"tester": {
		AllowedTools:    []string{"read_file", "write_file", "edit_file", "list_directory", "find_files", "grep_search", "run_tests", "noop"},
		AllowedCommands: []string{},
	},
	"generator": {
		AllowedTools:    []string{"read_file", "write_file", "edit_file", "list_directory", "find_files", "grep_search", "run_tests", "noop"},
		AllowedCommands: []string{},
	},
}

// PolicyValidator implements Validator interface.
type PolicyValidator struct {
	AllowedCommands   []string
	ProtectedBranch   string
	Profiles          map[string]ProfileConfig
	forbiddenPatterns []*regexp.Regexp
}

var _ Validator = (*PolicyValidator)(nil)

func NewPolicyValidator(allowedCommands []string, protectedBranch string, profiles map[string]ProfileConfig) *PolicyValidator {
	if protectedBranch == "" {
		protectedBranch = "main"
	}
	if profiles == nil {
		profiles = make(map[string]ProfileConfig)
	}
	return &PolicyValidator{
		AllowedCommands:   allowedCommands,
		ProtectedBranch:   protectedBranch,
		Profiles:          profiles,
		forbiddenPatterns: compileForbiddenPatterns(nil),
	}
}

// SetForbiddenPatterns updates the regex patterns used to reject write_file
// and edit_file content. Invalid regex patterns are silently skipped to
// avoid crashing the daemon on a bad config; the linter will flag them.
func (v *PolicyValidator) SetForbiddenPatterns(patterns []string) {
	v.forbiddenPatterns = compileForbiddenPatterns(patterns)
}

func compileForbiddenPatterns(patterns []string) []*regexp.Regexp {
	var compiled []*regexp.Regexp
	for _, p := range patterns {
		if p == "" {
			continue
		}
		re, err := regexp.Compile(p)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Warning: invalid forbidden_pattern regex %q: %v (skipping)\n", p, err)
			continue
		}
		compiled = append(compiled, re)
	}
	return compiled
}

func (v *PolicyValidator) Validate(ctx context.Context, action domain.Action, state *domain.State) (*ValidationResult, error) {
	// 1. Role-based dynamic checks
	role, _ := ctx.Value(AgentRoleKey).(string)
	if role != "" {
		// Get default profile for this role
		profile, exists := defaultRoleProfiles[role]
		if !exists {
			// Fallback to a safe default if role is unrecognized
			profile = ProfileConfig{
				AllowedTools:    []string{"run_tests", "read_file", "noop"},
				AllowedCommands: []string{},
			}
		}

		// Apply user configuration overrides if present
		if userProfile, ok := v.Profiles[role]; ok {
			if len(userProfile.AllowedTools) > 0 {
				profile.AllowedTools = userProfile.AllowedTools
			}
			if len(userProfile.AllowedCommands) > 0 {
				profile.AllowedCommands = userProfile.AllowedCommands
			}
		}

		// Check tool permissions
		toolAllowed := false
		for _, tool := range profile.AllowedTools {
			if tool == "*" || tool == action.Tool {
				toolAllowed = true
				break
			}
		}
		if !toolAllowed {
			return &ValidationResult{
				Allowed: false,
				Reason:  fmt.Sprintf("Role authorization violation: role '%s' is not authorized to call tool '%s'", role, action.Tool),
			}, nil
		}

		// Check specific run_tests commands if white-listed in the role profile
		if action.Tool == "run_tests" && len(profile.AllowedCommands) > 0 {
			command, _ := action.Args["command"].(string)
			if command != "" {
				parts := strings.Fields(command)
				if len(parts) > 0 {
					binary := parts[0]
					cmdAllowed := false
					for _, c := range profile.AllowedCommands {
						if c == "*" || c == binary {
							cmdAllowed = true
							break
						}
					}
					if !cmdAllowed {
						return &ValidationResult{
							Allowed: false,
							Reason:  fmt.Sprintf("Role authorization violation: role '%s' is not authorized to execute command '%s'", role, binary),
						}, nil
					}
				}
			}
		}
	}

	// 2. Global sandbox path checks (always enforced)
	switch action.Tool {
	case "write_file", "edit_file", "read_file":
		path, ok := action.Args["path"].(string)
		if !ok {
			return &ValidationResult{Allowed: false, Reason: "missing path argument"}, nil
		}
		// Path traversal check
		cleanProj := filepath.Clean(state.ProjectPath)
		var absPath string
		if filepath.IsAbs(path) {
			absPath = filepath.Clean(path)
		} else {
			absPath = filepath.Clean(filepath.Join(cleanProj, path))
		}
		if !strings.HasPrefix(absPath, cleanProj) {
			return &ValidationResult{
				Allowed: false,
				Reason:  fmt.Sprintf("Sandbox violation: path '%s' resolves outside the workspace boundary '%s'", path, state.ProjectPath),
			}, nil
		}
		if strings.Contains(absPath, "tests/holdout") || strings.Contains(absPath, "holdout") {
			return &ValidationResult{
				Allowed: false,
				Reason:  "Sandbox violation: tests/holdout directory must not be created, modified, or used under any circumstance",
			}, nil
		}

		// Forbidden pattern content checks (write_file/edit_file only).
		// These enforce project-specific compile/SPEC constraints such as
		// Rust #![deny(unsafe_code)] at write time so the agent gets
		// immediate feedback instead of at the test-validation stage.
		if action.Tool == "write_file" || action.Tool == "edit_file" {
			if content, ok := action.Args["content"].(string); ok && content != "" {
				if hit := v.findForbiddenPattern(content); hit != "" {
					return &ValidationResult{
						Allowed: false,
						Reason:  fmt.Sprintf("SPEC violation: file content matches forbidden pattern %q. This constraint must be respected: %s", hit, hit),
					}, nil
				}
			}
			if replacement, ok := action.Args["replacement_content"].(string); ok && replacement != "" {
				if hit := v.findForbiddenPattern(replacement); hit != "" {
					return &ValidationResult{
						Allowed: false,
						Reason:  fmt.Sprintf("SPEC violation: replacement_content matches forbidden pattern %q. This constraint must be respected.", hit),
					}, nil
				}
			}
		}

	case "git_checkout", "git_push":
		branch, _ := action.Args["branch"].(string)
		if branch == v.ProtectedBranch || branch == "master" {
			return &ValidationResult{
				Allowed: false,
				Reason:  fmt.Sprintf("VCS violation: Direct operations on protected branch '%s' are blocked", branch),
			}, nil
		}

	case "run_tests":
		command, _ := action.Args["command"].(string)
		if command != "" {
			parts := strings.Fields(command)
			if len(parts) > 0 {
				binary := parts[0]
				allowed := false
				for _, cmd := range v.AllowedCommands {
					if cmd == "*" || cmd == binary {
						allowed = true
						break
					}
				}
				if !allowed {
					return &ValidationResult{
						Allowed: false,
						Reason:  fmt.Sprintf("Sandbox violation: command '%s' is not in the whitelist of allowed commands", binary),
					}, nil
				}
			}
		}
	}

	return &ValidationResult{Allowed: true}, nil
}

func (v *PolicyValidator) EvaluateGoals(ctx context.Context, state *domain.State) (bool, error) {
	if len(state.Tasks) == 0 {
		return false, nil
	}
	for _, task := range state.Tasks {
		if task.Status != domain.TaskSuccess {
			return false, nil
		}
	}
	return true, nil
}

// findForbiddenPattern returns the first matching forbidden pattern's source
// string, or "" if none match. Patterns are compiled at construction time and
// applied to file content written by agents to enforce project-specific
// compile/SPEC constraints (e.g. Rust #![deny(unsafe_code)]).
func (v *PolicyValidator) findForbiddenPattern(content string) string {
	for _, re := range v.forbiddenPatterns {
		if re.MatchString(content) {
			return re.String()
		}
	}
	return ""
}
