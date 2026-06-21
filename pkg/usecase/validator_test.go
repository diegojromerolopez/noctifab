package usecase

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/diegojromerolopez/noctifab/pkg/domain"
)

func TestPolicyValidator_GlobalChecks(t *testing.T) {
	validator := NewPolicyValidator([]string{"go", "npm"}, "main")
	state := &domain.State{
		ProjectPath: "/workspace",
	}

	// 1. Path traversal block
	action := domain.Action{
		Tool: "write_file",
		Args: map[string]any{"path": "../etc/hosts"},
	}
	res, err := validator.Validate(context.Background(), action, state)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Allowed {
		t.Error("expected traversal path outside boundary to be blocked")
	}

	// 2. Protected branch block
	action2 := domain.Action{
		Tool: "git_checkout",
		Args: map[string]any{"branch": "main"},
	}
	res2, err := validator.Validate(context.Background(), action2, state)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res2.Allowed {
		t.Error("expected checkout of main to be blocked")
	}
}

func TestPolicyValidator_RoleProfiles(t *testing.T) {
	tempProfilesDir := t.TempDir()

	// Write dummy generator profile
	genProfileContent := `
role: generator
allowed_tools:
  - read_file
  - write_file
  - run_tests
allowed_commands:
  - go
  - npm
`
	err := os.WriteFile(filepath.Join(tempProfilesDir, "generator.yaml"), []byte(genProfileContent), 0644)
	if err != nil {
		t.Fatalf("failed to write test profile: %v", err)
	}

	validator := NewPolicyValidator([]string{"*"}, "main")
	validator.ProfilesDir = tempProfilesDir
	state := &domain.State{
		ProjectPath: "/workspace",
	}

	// 1. Tool allowed
	actionAllowed := domain.Action{
		Tool: "write_file",
		Args: map[string]any{"path": "main.go"},
	}
	ctx := context.WithValue(context.Background(), AgentRoleKey, "generator")
	res, err := validator.Validate(ctx, actionAllowed, state)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.Allowed {
		t.Errorf("expected write_file tool to be allowed for generator, blocked reason: %s", res.Reason)
	}

	// 2. Tool blocked (git_push is not in AllowedTools list)
	actionBlocked := domain.Action{
		Tool: "git_push",
		Args: map[string]any{"branch": "feature/branch"},
	}
	res2, err := validator.Validate(ctx, actionBlocked, state)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res2.Allowed {
		t.Error("expected git_push tool to be blocked for generator")
	}

	// 3. Command allowed
	actionCmdAllowed := domain.Action{
		Tool: "run_tests",
		Args: map[string]any{"command": "go test ./..."},
	}
	res3, err := validator.Validate(ctx, actionCmdAllowed, state)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res3.Allowed {
		t.Errorf("expected go command to be allowed for generator, blocked reason: %s", res3.Reason)
	}

	// 4. Command blocked (make is not in AllowedCommands list)
	actionCmdBlocked := domain.Action{
		Tool: "run_tests",
		Args: map[string]any{"command": "make build"},
	}
	res4, err := validator.Validate(ctx, actionCmdBlocked, state)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res4.Allowed {
		t.Error("expected make command to be blocked for generator")
	}
}
