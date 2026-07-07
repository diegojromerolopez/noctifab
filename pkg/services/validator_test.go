package services

import (
	"context"
	"testing"

	"github.com/diegojromerolopez/noctifab/pkg/domain"
)

func TestPolicyValidator_GlobalChecks(t *testing.T) {
	validator := NewPolicyValidator([]string{"go", "npm"}, "main", nil)
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
	profiles := map[string]ProfileConfig{
		"generator": {
			AllowedTools:    []string{"read_file", "write_file", "run_tests"},
			AllowedCommands: []string{"go", "npm"},
		},
	}

	validator := NewPolicyValidator([]string{"*"}, "main", profiles)
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

func TestRaceCommand_GoTest(t *testing.T) {
	cmd := raceCommand("go test -v ./...")
	if cmd != "go test -race -v ./..." {
		t.Errorf("expected 'go test -race -v ./...', got %q", cmd)
	}
}

func TestRaceCommand_NonGo(t *testing.T) {
	cmd := raceCommand("python -m unittest discover")
	if cmd != "python -m unittest discover" {
		t.Errorf("expected unchanged command, got %q", cmd)
	}
}

func TestRaceCommand_Empty(t *testing.T) {
	cmd := raceCommand("")
	if cmd != "" {
		t.Errorf("expected empty, got %q", cmd)
	}
}

func TestLastFailureOutput(t *testing.T) {
	results := []TestRunResult{
		{RunID: 1, Passed: true, Output: "first pass"},
		{RunID: 2, Passed: false, Output: "first fail"},
		{RunID: 3, Passed: false, Output: "last fail"},
	}
	out := lastFailureOutput(results)
	if out != "last fail" {
		t.Errorf("expected 'last fail', got %q", out)
	}
}

func TestLastFailureOutput_AllPass(t *testing.T) {
	results := []TestRunResult{
		{RunID: 1, Passed: true, Output: "pass"},
		{RunID: 2, Passed: true, Output: "pass"},
	}
	out := lastFailureOutput(results)
	if out != "" {
		t.Errorf("expected empty, got %q", out)
	}
}

func TestPolicyValidator_ForbiddenPatterns(t *testing.T) {
	validator := NewPolicyValidator([]string{"*"}, "main", nil)
	validator.SetForbiddenPatterns([]string{`\bunsafe\s*\{`})
	state := &domain.State{ProjectPath: "/workspace"}

	t.Run("write_file with unsafe block is blocked", func(t *testing.T) {
		action := domain.Action{
			Tool: "write_file",
			Args: map[string]any{
				"path":    "src/app/counter.rs",
				"content": "fn main() { unsafe { std::slice::from_raw_parts(p, n) } }",
			},
		}
		res, err := validator.Validate(context.Background(), action, state)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if res.Allowed {
			t.Errorf("expected unsafe content to be blocked; reason: %s", res.Reason)
		}
		if res.Reason == "" {
			t.Error("expected a SPEC violation reason")
		}
	})

	t.Run("write_file with safe content is allowed", func(t *testing.T) {
		action := domain.Action{
			Tool: "write_file",
			Args: map[string]any{
				"path":    "src/app/counter.rs",
				"content": "fn main() { let v = vec![1, 2, 3]; }",
			},
		}
		res, err := validator.Validate(context.Background(), action, state)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !res.Allowed {
			t.Errorf("expected safe content to be allowed; reason: %s", res.Reason)
		}
	})

	t.Run("edit_file replacement_content with unsafe block is blocked", func(t *testing.T) {
		action := domain.Action{
			Tool: "edit_file",
			Args: map[string]any{
				"path":                "src/app/counter.rs",
				"replacement_content": "unsafe { Ok(Some(slice)) }",
			},
		}
		res, err := validator.Validate(context.Background(), action, state)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if res.Allowed {
			t.Error("expected unsafe replacement to be blocked")
		}
	})

	t.Run("read_file is not content-checked", func(t *testing.T) {
		action := domain.Action{
			Tool: "read_file",
			Args: map[string]any{
				"path":    "src/app/counter.rs",
				"content": "unsafe { x }",
			},
		}
		res, err := validator.Validate(context.Background(), action, state)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !res.Allowed {
			t.Errorf("expected read_file to skip content check; reason: %s", res.Reason)
		}
	})

	t.Run("empty forbidden patterns allows everything", func(t *testing.T) {
		emptyValidator := NewPolicyValidator([]string{"*"}, "main", nil)
		action := domain.Action{
			Tool: "write_file",
			Args: map[string]any{
				"path":    "x.rs",
				"content": "unsafe { dangerous() }",
			},
		}
		res, err := emptyValidator.Validate(context.Background(), action, state)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !res.Allowed {
			t.Errorf("expected empty patterns to allow all; reason: %s", res.Reason)
		}
	})

	t.Run("invalid regex is skipped not fatal", func(t *testing.T) {
		badValidator := NewPolicyValidator([]string{"*"}, "main", nil)
		badValidator.SetForbiddenPatterns([]string{"[invalid", `\bunsafe\s*\{`})
		action := domain.Action{
			Tool: "write_file",
			Args: map[string]any{
				"path":    "x.rs",
				"content": "unsafe { x }",
			},
		}
		res, err := badValidator.Validate(context.Background(), action, state)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if res.Allowed {
			t.Error("expected the valid pattern to still block")
		}
	})
}
