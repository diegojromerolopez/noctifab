package services

import (
	"context"
	"fmt"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/diegojromerolopez/noctifab/pkg/domain"
)

var AgentRoleKey = domain.RoleContextKey{}

type taskIDContextKey string

const TaskIDKey taskIDContextKey = "task_id"

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

// PolicyValidator implements Validator interface.
type PolicyValidator struct {
	AllowedCommands   []string
	ProtectedBranch   string
	Profiles          map[string]ProfileConfig
	forbiddenPatterns []*regexp.Regexp
	ExcludePaths      []string
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
		ExcludePaths:      nil,
	}
}

// SetForbiddenPatterns updates the regex patterns used to reject write_file
// and edit_file content. Invalid regex patterns are silently skipped to
// avoid crashing the daemon on a bad config; the linter will flag them.
func (v *PolicyValidator) SetForbiddenPatterns(patterns []string) {
	v.forbiddenPatterns = compileForbiddenPatterns(patterns)
}

func (v *PolicyValidator) Validate(ctx context.Context, action domain.Action, state *domain.State) (*ValidationResult, error) {
	role := domain.GetRoleFromContext(ctx)
	if role == "" {
		role, _ = ctx.Value(AgentRoleKey).(string)
	}
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
			if strings.TrimSpace(action.Tool) == "" {
				return &ValidationResult{
					Allowed: false,
					Reason:  fmt.Sprintf("Role authorization violation: tool name is empty. Role '%s' is bounded to authorized tools: [%s]. Please specify a valid tool name.", role, strings.Join(profile.AllowedTools, ", ")),
				}, nil
			}
			return &ValidationResult{
				Allowed: false,
				Reason:  fmt.Sprintf("Role authorization violation: role '%s' is not authorized to call tool '%s'. Authorized tools: [%s]. Please use allowed tools like read_file, list_directory, write_file, edit_file, or run_tests.", role, action.Tool, strings.Join(profile.AllowedTools, ", ")),
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
							Reason:  fmt.Sprintf("Role authorization violation: role '%s' is not authorized to execute command '%s'. Allowed commands: [%s].", role, binary, strings.Join(profile.AllowedCommands, ", ")),
						}, nil
					}
				}
			}
		}
	}

	// Immediate check: Guard against empty tool arguments
	if action.Tool != "noop" && action.Tool != "run_tests" && action.Tool != "run_e2e_tests" && action.Tool != "run_linter" {
		if len(action.Args) == 0 {
			return &ValidationResult{
				Allowed: false,
				Reason:  fmt.Sprintf("Invalid tool call: '%s' was invoked with empty arguments (map[]). Please provide the required arguments for this tool nested under the 'args' key. Example: {\"tool\": \"%s\", \"args\": {\"path\": \"...\"}}.", action.Tool, action.Tool),
			}, nil
		}
	}

	// 2. Global sandbox path checks (always enforced)
	switch action.Tool {
	case "write_files":
		entries, err := parseWriteFilesArgs(action.Args)
		if err != nil {
			return &ValidationResult{Allowed: false, Reason: fmt.Sprintf("Invalid tool call: 'write_files' argument validation failed: %v. Required schema: write_files(files={'relative/path': 'content'}).", err)}, nil
		}
		if len(entries) == 0 {
			return &ValidationResult{Allowed: false, Reason: "Invalid tool call: 'write_files' requires a non-empty 'files' dictionary mapping file paths to file contents. Required schema: write_files(files={'relative/path': 'content'})."}, nil
		}
		cleanProj := filepath.Clean(state.ProjectPath)
		for _, entry := range entries {
			var absPath string
			if filepath.IsAbs(entry.path) {
				absPath = filepath.Clean(entry.path)
			} else {
				absPath = filepath.Clean(filepath.Join(cleanProj, entry.path))
			}
			if !strings.HasPrefix(absPath, cleanProj) {
				return &ValidationResult{
					Allowed: false,
					Reason:  fmt.Sprintf("Sandbox violation: path '%s' resolves outside the workspace boundary '%s'", entry.path, state.ProjectPath),
				}, nil
			}
			if strings.Contains(absPath, "tests/holdout") || strings.Contains(absPath, "holdout") {
				return &ValidationResult{
					Allowed: false,
					Reason:  "Sandbox violation: tests/holdout directory must not be created, modified, or used under any circumstance",
				}, nil
			}
			if (strings.Contains(absPath, "tests/acceptance") || strings.Contains(absPath, "acceptance/")) && (role == "generator" || role == "tester") {
				return &ValidationResult{
					Allowed: false,
					Reason:  "Policy violation: independent QA acceptance tests in 'tests/acceptance/' are immutable and cannot be modified or deleted by worker agents (generator/tester). Workers must author unit tests in 'tests/unit/'.",
				}, nil
			}
			rel, err := filepath.Rel(cleanProj, absPath)
			if err == nil {
				parts := strings.Split(rel, string(filepath.Separator))
				for _, part := range parts {
					if part == ".noctifab" || part == ".git" {
						return &ValidationResult{
							Allowed: false,
							Reason:  fmt.Sprintf("Sandbox violation: path '%s' targets blacklisted directory '%s'", entry.path, part),
						}, nil
					}
					for _, exp := range v.ExcludePaths {
						cleanExp := strings.Trim(exp, "/")
						if cleanExp != "" && part == cleanExp {
							return &ValidationResult{
								Allowed: false,
								Reason:  fmt.Sprintf("Sandbox violation: path '%s' targets excluded path segment '%s'. Do NOT attempt to create files in this excluded directory, and UPDATE any test files to omit assertions on excluded paths.", entry.path, cleanExp),
							}, nil
						}
					}
				}
			}
			if hit := v.findForbiddenPattern(entry.content); hit != "" {
				return &ValidationResult{
					Allowed: false,
					Reason:  fmt.Sprintf("SPEC violation: file content for %q matches forbidden pattern %q. This constraint must be respected: %s", entry.path, hit, hit),
				}, nil
			}
		}

	case "list_directory":
		path, _ := action.Args["path"].(string)
		if strings.TrimSpace(path) == "" {
			if action.Args == nil {
				action.Args = make(map[string]any)
			}
			action.Args["path"] = "."
		}

	case "write_file", "edit_file", "apply_patch", "read_file", "delete_file":
		if action.Tool == "write_file" {
			path, _ := action.Args["path"].(string)
			if strings.TrimSpace(path) == "" {
				return &ValidationResult{
					Allowed: false,
					Reason:  "Invalid tool call: 'write_file' requires a non-empty 'path' string argument. Example: write_file(path='src/module.py', content='def run(): pass').",
				}, nil
			}
		}
		if action.Tool == "edit_file" {
			path, _ := action.Args["path"].(string)
			_, hasTarget := action.Args["old_content"].(string)
			if !hasTarget {
				_, hasTarget = action.Args["target_content"].(string)
			}
			_, hasReplacement := action.Args["new_content"].(string)
			if !hasReplacement {
				_, hasReplacement = action.Args["replacement_content"].(string)
			}
			_, hasEdits := action.Args["edits"].([]any)
			if !hasEdits {
				_, hasEdits = action.Args["replacements"].([]any)
			}
			if strings.TrimSpace(path) == "" || (!hasEdits && (!hasTarget || !hasReplacement)) {
				return &ValidationResult{
					Allowed: false,
					Reason:  "Invalid tool call: 'edit_file' requires 'path', and either 'target_content'/'replacement_content' (or 'old_content'/'new_content') or 'edits'.",
				}, nil
			}
		}
		if action.Tool == "apply_patch" {
			patch, hasPatch := action.Args["patch"].(string)
			if !hasPatch {
				patch, hasPatch = action.Args["diff"].(string)
			}
			if !hasPatch || strings.TrimSpace(patch) == "" {
				return &ValidationResult{
					Allowed: false,
					Reason:  "Invalid tool call: 'apply_patch' requires a non-empty 'patch' or 'diff' string argument containing a unified diff.",
				}, nil
			}
		}
		if action.Tool == "read_file" {
			path, _ := action.Args["path"].(string)
			if strings.TrimSpace(path) == "" {
				return &ValidationResult{
					Allowed: false,
					Reason:  "Invalid tool call: 'read_file' requires a non-empty 'path' string argument. Example: read_file(path='src/module.py').",
				}, nil
			}
		}
		if action.Tool == "delete_file" {
			path, _ := action.Args["path"].(string)
			if strings.TrimSpace(path) == "" {
				return &ValidationResult{
					Allowed: false,
					Reason:  "Invalid tool call: 'delete_file' requires a non-empty 'path' string argument. Example: delete_file(path='temp.txt').",
				}, nil
			}
		}
		if action.Tool == "apply_patch" {
			patch, hasPatch := action.Args["patch"].(string)
			if !hasPatch {
				patch, hasPatch = action.Args["diff"].(string)
			}
			if !hasPatch || strings.TrimSpace(patch) == "" {
				return &ValidationResult{
					Allowed: false,
					Reason:  "Invalid tool call: 'apply_patch' requires a non-empty 'patch' or 'diff' string argument containing a unified diff.",
				}, nil
			}
		}
		if action.Tool == "read_file" {
			path, _ := action.Args["path"].(string)
			if strings.TrimSpace(path) == "" {
				return &ValidationResult{
					Allowed: false,
					Reason:  "Invalid tool call: 'read_file' requires a non-empty 'path' string argument. Example: read_file(path='src/module.py').",
				}, nil
			}
		}
		if action.Tool == "delete_file" {
			path, _ := action.Args["path"].(string)
			if strings.TrimSpace(path) == "" {
				return &ValidationResult{
					Allowed: false,
					Reason:  "Invalid tool call: 'delete_file' requires a non-empty 'path' string argument. Example: delete_file(path='temp.txt').",
				}, nil
			}
		}

		path, ok := action.Args["path"].(string)
		if !ok && action.Tool == "apply_patch" {
			// apply_patch may specify file paths inside the patch payload itself
			path = "."
			ok = true
		}
		if !ok || strings.TrimSpace(path) == "" {
			return &ValidationResult{Allowed: false, Reason: fmt.Sprintf("Invalid tool call: '%s' requires a non-empty 'path' string argument.", action.Tool)}, nil
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
		if (action.Tool == "write_file" || action.Tool == "edit_file" || action.Tool == "delete_file" || action.Tool == "apply_patch") &&
			(strings.Contains(absPath, "tests/acceptance") || strings.Contains(absPath, "acceptance/")) && (role == "generator" || role == "tester") {
			return &ValidationResult{
				Allowed: false,
				Reason:  "Policy violation: independent QA acceptance tests in 'tests/acceptance/' are immutable and cannot be modified or deleted by worker agents (generator/tester). Workers must author unit tests in 'tests/unit/'.",
			}, nil
		}

		rel, err := filepath.Rel(cleanProj, absPath)
		if err == nil {
			parts := strings.Split(rel, string(filepath.Separator))
			for _, part := range parts {
				if part == ".noctifab" || part == ".git" {
					return &ValidationResult{
						Allowed: false,
						Reason:  fmt.Sprintf("Sandbox violation: path '%s' targets blacklisted directory '%s'", path, part),
					}, nil
				}
				for _, exp := range v.ExcludePaths {
					cleanExp := strings.Trim(exp, "/")
					if cleanExp != "" && part == cleanExp {
						return &ValidationResult{
							Allowed: false,
							Reason:  fmt.Sprintf("Sandbox violation: path '%s' targets excluded path segment '%s'. Do NOT attempt to create files in this excluded directory, and UPDATE any test files to omit assertions on excluded paths.", path, cleanExp),
						}, nil
					}
				}
			}
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

	case "run_tests", "run_e2e_tests":
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
