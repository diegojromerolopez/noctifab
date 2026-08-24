package cli

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/diegojromerolopez/noctifab/pkg/infrastructure/config"
	"github.com/spf13/cobra"
)

var (
	WorkspaceDir    = "."
	ProfileFlag     = ""
	initSpecPrompt  = ""
	initInteractive = false
)

var initCmd = &cobra.Command{
	Use:           "init [target_dir]",
	Short:         "Initialize noctifab workspace, config, and SPEC.md template",
	SilenceErrors: true,
	SilenceUsage:  true,
	RunE: func(cmd *cobra.Command, args []string) error {
		targetDir := WorkspaceDir
		if targetDir == "" {
			targetDir = "."
		}
		if len(args) > 0 {
			targetDir = args[0]
		}
		_, err := EnsureWorkspaceInitializedWithProfile(targetDir, ProfileFlag)
		if err != nil {
			return err
		}
		if ProfileFlag != "" {
			fmt.Printf("noctifab workspace initialized successfully in %s with profile %q\n", targetDir, ProfileFlag)
		} else {
			fmt.Printf("noctifab workspace initialized successfully in %s\n", targetDir)
		}

		if initSpecPrompt != "" || initInteractive {
			return runSpecSession(targetDir, initSpecPrompt, "SPEC.md", false, true)
		}
		return nil
	},
}

func init() {
	initCmd.Flags().StringVar(&ProfileFlag, "profile", "", "Pre-configured LLM profile (e.g. ollama-qwen, ollama-deepseek, vllm-local)")
	initCmd.Flags().StringVar(&initSpecPrompt, "spec", "", "Initial prompt to bootstrap and interactively refine SPEC.md")
	initCmd.Flags().BoolVarP(&initInteractive, "interactive", "i", false, "Launch interactive spec generator wizard upon initialization")
}

// EnsureWorkspaceInitialized ensures .noctifab/ directory, config.yaml, secrets.yaml, database, and SPEC.md exist.
// Returns createdSpec=true if a new SPEC.md template was written.
func EnsureWorkspaceInitialized(targetDir string) (bool, error) {
	return EnsureWorkspaceInitializedWithProfile(targetDir, "")
}

// EnsureWorkspaceInitializedWithProfile initializes workspace with an optional configuration profile preset.
func EnsureWorkspaceInitializedWithProfile(targetDir string, profileName string) (bool, error) {
	if targetDir == "" {
		targetDir = "."
	}
	if err := os.MkdirAll(targetDir, 0755); err != nil {
		return false, fmt.Errorf("failed to create target directory %s: %w", targetDir, err)
	}

	// 1. Directory cleanliness check
	entries, err := os.ReadDir(targetDir)
	if err != nil {
		return false, fmt.Errorf("failed to read target directory: %w", err)
	}

	hasNoctifab := false
	hasGit := false
	hasOtherFiles := false
	for _, entry := range entries {
		name := entry.Name()
		if name == ".noctifab" {
			hasNoctifab = true
		} else if name == ".git" {
			hasGit = true
		} else if name != ".tool-versions" && name != "." && name != ".." && name != "SPEC.md" && name != "roadmap" && name != "secrets.yaml" && name != "README.md" {
			hasOtherFiles = true
		}
	}

	// If no .noctifab folder AND it's not a git repository AND contains other project files, abort with code 4
	if !hasNoctifab && !hasGit && hasOtherFiles {
		return false, &ExitError{
			Code: 4,
			Msg:  "Security Warning: Target directory contains existing project files but is not initialized with .noctifab or .git. Aborting to prevent overwrite.",
		}
	}

	// 2. Create directory structure
	noctifabDir := filepath.Join(targetDir, ".noctifab")
	subDirs := []string{
		"data",
		"logs",
	}
	for _, sub := range subDirs {
		p := filepath.Join(noctifabDir, sub)
		if err := os.MkdirAll(p, 0755); err != nil {
			return false, fmt.Errorf("failed to create directory %s: %w", p, err)
		}
	}

	// 3. Generate default or profile config.yaml if it doesn't exist
	cfgPath := filepath.Join(noctifabDir, "config.yaml")
	if _, err := os.Stat(cfgPath); os.IsNotExist(err) {
		if profileName != "" {
			preset, err := config.GetProfile(profileName)
			if err != nil {
				return false, err
			}
			if err := os.WriteFile(cfgPath, []byte(preset.ConfigYAML), 0644); err != nil {
				return false, fmt.Errorf("failed to write profile config: %w", err)
			}
		} else {
			if err := config.WriteDefaultConfig(cfgPath); err != nil {
				return false, fmt.Errorf("failed to write default config: %w", err)
			}
		}
	}

	// 4. Generate default secrets.yaml only if missing locally AND no global secrets file exists in $HOME/.noctifab/secrets.yaml
	secretsPath := filepath.Join(noctifabDir, "secrets.yaml")
	if !config.HasGlobalSecrets() {
		if _, err := os.Stat(secretsPath); os.IsNotExist(err) {
			secretsContent := `# Noctifab Secrets & API Keys Configuration
OPENAI_API_KEY: ""
ANTHROPIC_API_KEY: ""
GEMINI_API_KEY: ""
OPENCODE_API_KEY: ""
KIMI_API_KEY: ""
MOONSHOT_API_KEY: ""
GROQ_API_KEY: ""
OPENROUTER_API_KEY: ""
QWEN_API_KEY: ""
DASHSCOPE_API_KEY: ""
TOGETHER_API_KEY: ""
LLAMA_API_KEY: ""
HUGGINGFACE_API_KEY: ""
MISTRAL_API_KEY: ""
DEEPSEEK_API_KEY: ""
HERMES_API_KEY: ""
OLLAMA_API_KEY: ""
XAI_API_KEY: ""
PERPLEXITY_API_KEY: ""
FIREWORKS_API_KEY: ""
SAMBANOVA_API_KEY: ""
COHERE_API_KEY: ""
CEREBRAS_API_KEY: ""
NVIDIA_API_KEY: ""
AI21_API_KEY: ""
UPSTAGE_API_KEY: ""
GITHUB_TOKEN: ""
`
			if err := os.WriteFile(secretsPath, []byte(secretsContent), 0644); err != nil {
				return false, fmt.Errorf("failed to create secrets.yaml: %w", err)
			}
		}
	}

	// 5. Initialize local SQLite database file if it doesn't exist
	dbPath := filepath.Join(noctifabDir, "data", "noctifab.db")
	if _, err := os.Stat(dbPath); os.IsNotExist(err) {
		f, err := os.OpenFile(dbPath, os.O_CREATE|os.O_WRONLY, 0644)
		if err != nil {
			return false, fmt.Errorf("failed to initialize database file: %w", err)
		}
		_ = f.Close()
	}

	// 6. Create local VCS ignore file (.noctifab/.gitignore)
	gitIgnorePath := filepath.Join(noctifabDir, ".gitignore")
	if _, err := os.Stat(gitIgnorePath); os.IsNotExist(err) {
		ignoreContent := "data/noctifab.db\ndata/noctifab.db-shm\ndata/noctifab.db-wal\nlogs/\nworktrees/\nnoctifab.pid\nsecrets.yaml\n"
		if err := os.WriteFile(gitIgnorePath, []byte(ignoreContent), 0644); err != nil {
			return false, fmt.Errorf("failed to create .gitignore: %w", err)
		}
	}

	// 7. Generate default SPEC.md template if missing
	createdSpec := false
	specPath := filepath.Join(targetDir, "SPEC.md")
	if _, err := os.Stat(specPath); os.IsNotExist(err) {
		specContent := `# Specification: New Project

## 1. Overview
Describe the high-level goal, architecture, and purpose of the software project.

## 2. Technology Stack & Language Guidelines
- **Primary Language**: Go / C / Python / TypeScript / Rust
- **Target Runtime**: Native binary / Node.js / Python 3.11
- **Testing Framework**: Native unit tests

## 3. Core Domain Models & Schemas
Define key entities, data structures, and database schemas.

## 4. Interfaces & Command Contracts
- **CLI Commands**:
  - ` + "`my-app --help`" + `
- **HTTP Endpoints (if applicable)**:
  - ` + "`GET /healthz`" + `

## 5. Acceptance Criteria & Quality Gates
- All unit tests must pass cleanly.
- Zero linter warnings or static analysis issues.
`
		if err := os.WriteFile(specPath, []byte(specContent), 0644); err != nil {
			return false, fmt.Errorf("failed to create SPEC.md template: %w", err)
		}
		fmt.Printf("Created SPEC.md template at %s\n", specPath)
		createdSpec = true
	}

	// 8. Generate roadmap/user-stories/US-001.md template only when SPEC.md was also just created
	// (i.e. a completely fresh project). If SPEC.md already exists, the roadmap is
	// either user-managed or will be generated by the PM Agent at start time.
	storiesDir := filepath.Join(targetDir, "roadmap", "user-stories")
	if err := os.MkdirAll(storiesDir, 0755); err != nil {
		return false, fmt.Errorf("failed to create roadmap user-stories directory: %w", err)
	}

	us1Path := filepath.Join(storiesDir, "US-001.md")
	if createdSpec {
		if _, err := os.Stat(us1Path); os.IsNotExist(err) {
			us1Content := `# User Story: US-001 - Initial Feature Specification

## Metadata

| Field | Value |
|---------|---------|
| ID | US-001 |
| Priority | High |
| Status | Draft |
| Epic | Core Functionality |
| Author | Product Manager |
| Created | 2026-07-28 |
| Updated | 2026-07-28 |
| depends_on | None |
| change_type | new |

---

## Objective

Describe the high-level goal of this user story.

---

## User Story

As a user,
I want to execute a feature,
So that I achieve the intended capability.

---

## Business Value

- Outlines key benefits and goals of the feature.

---

## Context

### Existing Behavior

Describe existing behavior before this feature.

### Desired Behavior

Describe desired behavior after this feature is implemented.

---

## Requirements

### Functional Requirements

#### FR-1
Define functional requirement 1.

#### FR-2
Define functional requirement 2.

### Acceptance Criteria

#### AC-1
Define acceptance criteria 1.

#### AC-2
Define acceptance criteria 2.

## Definition of Done (DoD)

- Replace the example interface, executable, paths, and observable expectations below with exact public behavior for this story.
- All configured tests and linters complete with zero failures.

` + "```noctifab-contract" + `
{
  "story_id": "US-001",
  "public_contracts": [
    {
      "id": "cli.example",
      "interface": "CLI ./dist/my-app",
      "applicable_path_prefixes": ["cmd/", "pkg/"],
      "allowed_executables": ["./dist/my-app"],
      "exit_codes": [0],
      "stdout_contains": ["expected output"],
      "stderr_prefixes": []
    }
  ]
}
` + "```" + `
`
			if err := os.WriteFile(us1Path, []byte(us1Content), 0644); err != nil {
				return false, fmt.Errorf("failed to create roadmap/US-001.md: %w", err)
			}
			fmt.Printf("Created user story template at %s\n", us1Path)
		}
	}

	return createdSpec, nil
}

func init() {
	initCmd.Flags().String("vcs-clone-protocol", "https", "VCS clone protocol: https, ssh")
	RootCmd.AddCommand(initCmd)
}
