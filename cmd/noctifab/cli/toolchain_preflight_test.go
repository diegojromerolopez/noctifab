package cli

import (
	"os"
	"path/filepath"
	"testing"
)

func TestHasToolSuffix(t *testing.T) {
	tests := []struct {
		name     string
		expected bool
	}{
		{"arm-none-eabi-gcc", true},
		{"x86_64-linux-gnu-as", true},
		{"custom-clang", true},
		{"cross-g++", true},
		{"tool-ld", true},
		{"bin-nasm", true},
		{"echo-cli", false},
		{"todo-cli", false},
		{"wc-cli", false},
		{"calc-repl", false},
		{"ninline-engine", false},
		{"gcc", false}, // hasToolSuffix checks suffixes like "-gcc"; base "gcc" is handled directly
	}

	for _, tt := range tests {
		got := hasToolSuffix(tt.name)
		if got != tt.expected {
			t.Errorf("hasToolSuffix(%q) = %v; expected %v", tt.name, got, tt.expected)
		}
	}
}

func TestDetectRequiredProjectToolchains_FiltersProjectBinaries(t *testing.T) {
	tempDir := t.TempDir()

	// 1. Create a Makefile with CC = gcc
	makefilePath := filepath.Join(tempDir, "Makefile")
	if err := os.WriteFile(makefilePath, []byte("CC = gcc\n"), 0644); err != nil {
		t.Fatalf("failed to write Makefile: %v", err)
	}

	// 2. Create a roadmap user story with AllowedExecutables
	roadmapDir := filepath.Join(tempDir, "roadmap", "user-stories")
	if err := os.MkdirAll(roadmapDir, 0755); err != nil {
		t.Fatalf("failed to create roadmap dir: %v", err)
	}

	storyContent := `# User Story 001: Baseline CLI

` + "```noctifab-contract\n" + `{
  "story_id": "US-001",
  "public_contracts": [
    {
      "id": "c1",
      "interface": "cli",
      "allowed_executables": [
        "echo-cli",
        "todo-cli",
        "valgrind",
        "qemu-arm",
        "./bin/runner"
      ],
      "exit_codes": [0]
    }
  ]
}
` + "```\n"

	storyPath := filepath.Join(roadmapDir, "US-001.md")
	if err := os.WriteFile(storyPath, []byte(storyContent), 0644); err != nil {
		t.Fatalf("failed to write story file: %v", err)
	}

	tools := DetectRequiredProjectToolchains(tempDir)
	toolMap := make(map[string]bool)
	for _, tool := range tools {
		toolMap[tool] = true
	}

	// gcc from Makefile should be included
	if !toolMap["gcc"] {
		t.Errorf("expected 'gcc' to be detected from Makefile, got: %v", tools)
	}

	// valgrind and qemu-arm from allowed_executables should be included
	if !toolMap["valgrind"] {
		t.Errorf("expected 'valgrind' to be detected as toolchain, got: %v", tools)
	}
	if !toolMap["qemu-arm"] {
		t.Errorf("expected 'qemu-arm' to be detected as toolchain, got: %v", tools)
	}

	// echo-cli and todo-cli must NOT be detected as required host toolchains
	if toolMap["echo-cli"] {
		t.Errorf("false positive: 'echo-cli' was detected as required toolchain! tools: %v", tools)
	}
	if toolMap["todo-cli"] {
		t.Errorf("false positive: 'todo-cli' was detected as required toolchain! tools: %v", tools)
	}
	if toolMap["runner"] || toolMap["./bin/runner"] {
		t.Errorf("false positive: './bin/runner' was detected as required toolchain! tools: %v", tools)
	}
}
