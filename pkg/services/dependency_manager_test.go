package services

import (
	"context"
	"testing"
)

func TestDetectMissingTool(t *testing.T) {
	dm := NewDependencyManager([]string{"pip", "go", "brew", "curl"})

	tests := []struct {
		name      string
		output    string
		wantTool  string
		wantFound bool
	}{
		{name: "executable file not found - cargo", output: `exec: "cargo": executable file not found`, wantTool: "cargo", wantFound: true},
		{name: "command not found - pytest", output: "bash: pytest: command not found", wantTool: "pytest", wantFound: true},
		{name: "no such file - golangci-lint", output: "/bin/sh: golangci-lint: no such file or directory", wantTool: "golangci-lint", wantFound: true},
		{name: "node not found", output: "exec: \"node\": executable file not found", wantTool: "node", wantFound: true},
		{name: "npm not found", output: "exec: \"npm\": executable file not found", wantTool: "npm", wantFound: true},
		{name: "coverage not found", output: "bash: coverage: command not found", wantTool: "coverage", wantFound: true},
		{name: "vitest not found", output: "exec: \"vitest\": executable file not found", wantTool: "vitest", wantFound: true},
		{name: "jest not found", output: "sh: jest: not found", wantTool: "jest", wantFound: true},
		{name: "ts-node not found", output: "exec: \"ts-node\": executable file not found", wantTool: "ts-node", wantFound: true},
		{name: "rspec not found", output: "bash: rspec: command not found", wantTool: "rspec", wantFound: true},
		{name: "dune not found", output: "bash: dune: command not found", wantTool: "dune", wantFound: true},
		{name: "no match", output: "unrelated output", wantTool: "", wantFound: false},
		{name: "empty output", output: "", wantTool: "", wantFound: false},
		{name: "tool not in map", output: "exec: \"some-unknown-tool\": executable file not found", wantTool: "", wantFound: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tool, found := dm.DetectMissingTool(tt.output)
			if found != tt.wantFound {
				t.Errorf("DetectMissingTool() found = %v, want %v", found, tt.wantFound)
			}
			if tool != tt.wantTool {
				t.Errorf("DetectMissingTool() tool = %q, want %q", tool, tt.wantTool)
			}
		})
	}
}

func TestIsAllowed(t *testing.T) {
	dm := NewDependencyManager([]string{"pip", "go"})

	tests := []struct {
		manager string
		allowed bool
	}{
		{"pip", true},
		{"go", true},
		{"brew", false},
		{"curl", false},
		{"npm", false},
		{"", false},
	}
	for _, tt := range tests {
		t.Run(tt.manager, func(t *testing.T) {
			if got := dm.IsAllowed(tt.manager); got != tt.allowed {
				t.Errorf("IsAllowed(%q) = %v, want %v", tt.manager, got, tt.allowed)
			}
		})
	}
}

func TestInstallTool_UnknownTool(t *testing.T) {
	dm := NewDependencyManager([]string{"pip"})
	err := dm.InstallTool(context.Background(), "some-unknown-tool")
	if err == nil {
		t.Fatal("expected error for unknown tool, got nil")
	}
}

func TestInstallTool_DisallowedManager(t *testing.T) {
	dm := NewDependencyManager([]string{"pip"})
	err := dm.InstallTool(context.Background(), "golangci-lint")
	if err == nil {
		t.Fatal("expected error for disallowed manager, got nil")
	}
}
