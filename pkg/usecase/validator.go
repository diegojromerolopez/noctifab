package usecase

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/diegojromerolopez/noctifab/pkg/domain"
	"gopkg.in/yaml.v3"
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

// RoleProfile defines the allowed permissions for a specific agent role.
type RoleProfile struct {
	Role            string   `yaml:"role"`
	AllowedTools    []string `yaml:"allowed_tools"`
	AllowedCommands []string `yaml:"allowed_commands"`
	Permissions     struct {
		AllowedTools    []string `yaml:"allowed_tools"`
		AllowedCommands []string `yaml:"allowed_commands"`
	} `yaml:"permissions"`
}

// PolicyValidator implements Validator interface.
type PolicyValidator struct {
	AllowedCommands []string
	ProtectedBranch string
	ProfilesDir     string
}

var _ Validator = (*PolicyValidator)(nil)

func NewPolicyValidator(allowedCommands []string, protectedBranch string) *PolicyValidator {
	if protectedBranch == "" {
		protectedBranch = "main"
	}
	return &PolicyValidator{
		AllowedCommands: allowedCommands,
		ProtectedBranch: protectedBranch,
		ProfilesDir:     filepath.Join(".noctifab", "profiles"),
	}
}

func (v *PolicyValidator) Validate(ctx context.Context, action domain.Action, state *domain.State) (*ValidationResult, error) {
	// 1. Role-based dynamic checks
	role, _ := ctx.Value(AgentRoleKey).(string)
	if role != "" {
		profilePath := filepath.Join(v.ProfilesDir, role+".yaml")
		if _, err := os.Stat(profilePath); err == nil {
			profileData, err := os.ReadFile(profilePath)
			if err == nil {
				var profile RoleProfile
				if err := yaml.Unmarshal(profileData, &profile); err == nil {
					// Check tool permissions
					allowedTools := profile.AllowedTools
					if len(allowedTools) == 0 && len(profile.Permissions.AllowedTools) > 0 {
						allowedTools = profile.Permissions.AllowedTools
					}
					toolAllowed := false
					for _, tool := range allowedTools {
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

					// Check specific run_tests commands if white-listed
					allowedCommands := profile.AllowedCommands
					if len(allowedCommands) == 0 && len(profile.Permissions.AllowedCommands) > 0 {
						allowedCommands = profile.Permissions.AllowedCommands
					}
					if action.Tool == "run_tests" && len(allowedCommands) > 0 {
						command, _ := action.Args["command"].(string)
						if command != "" {
							parts := strings.Fields(command)
							if len(parts) > 0 {
								binary := parts[0]
								cmdAllowed := false
								for _, c := range allowedCommands {
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
