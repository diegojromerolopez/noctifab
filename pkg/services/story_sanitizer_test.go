package services

import (
	"testing"

	"github.com/diegojromerolopez/noctifab/pkg/domain"
	"github.com/stretchr/testify/assert"
)

func TestIsSmallCLIProject(t *testing.T) {
	cliSpec := `# Todo CLI Tool
Specification for a single-binary command-line todo management tool.
The application reads stdin, writes stdout, supports --help, --version flags, and exit code 0 on success.
Complexity: Tier 0, CU < 35.`

	assert.True(t, IsSmallCLIProject(cliSpec, 0))
	assert.True(t, IsSmallCLIProject("Short spec", 2))

	complexSpec := `# Enterprise Microservices Portal
This system uses React SPA frontend, Kubernetes, Spring Boot backend, and PostgreSQL with vector search.`
	assert.False(t, IsSmallCLIProject(complexSpec, 0))
}

func TestIsHardeningStory(t *testing.T) {
	assert.True(t, IsHardeningStory("US-003", "Project Hardening and Maintenance", "Format code and verify docker compose"))
	assert.True(t, IsHardeningStory("US-FINAL", "Production Readiness", "Run e2e test suite"))
	assert.True(t, IsHardeningStory("roadmap/user-stories/US-003-hardening.md", "", ""))
	assert.False(t, IsHardeningStory("US-001", "Core Domain Logic", "Implement account aggregates"))
}

func TestSanitizeAndCapStories_DuplicateStoryIDs(t *testing.T) {
	raw := []RawStoryItem{
		{
			Filename: "roadmap/user-stories/US-001-legacy-codebase-characterization.md",
			Content: `# User Story 001: Legacy Characterization
- **Depends On**: []
` + "```noctifab-contract\n" + `{"story_id": "US-001"}` + "\n```",
		},
		{
			Filename: "roadmap/user-stories/US-001-foundational-scaffolding.md",
			Content: `# User Story 001: Foundational Scaffolding
- **Depends On**: ["US-001"]
` + "```noctifab-contract\n" + `{"story_id": "US-001"}` + "\n```",
		},
		{
			Filename: "roadmap/user-stories/US-002-project-hardening.md",
			Content: `# User Story 002: Project Hardening
- **Depends On**: ["US-001"]
` + "```noctifab-contract\n" + `{"story_id": "US-002"}` + "\n```",
		},
	}

	sanitized := SanitizeAndCapStories("/tmp", raw, "Large Spec with complex backend", 4)
	assert.Len(t, sanitized, 3)

	// Story 1: US-001 (Legacy)
	assert.Equal(t, "roadmap/user-stories/US-001-legacy-codebase-characterization.md", sanitized[0].Filename)
	assert.Contains(t, sanitized[0].Content, `{"story_id": "US-001"}`)

	// Story 2: US-002 (Renumbered from duplicate US-001!)
	assert.Equal(t, "roadmap/user-stories/US-002-foundational-scaffolding.md", sanitized[1].Filename)
	assert.Contains(t, sanitized[1].Content, "# User Story 002: Foundational Scaffolding")
	assert.Contains(t, sanitized[1].Content, `{"story_id": "US-002"}`)

	// Story 3: US-003 (Renumbered from US-002!)
	assert.Equal(t, "roadmap/user-stories/US-003-project-hardening.md", sanitized[2].Filename)
	assert.Contains(t, sanitized[2].Content, "# User Story 003: Project Hardening")
	assert.Contains(t, sanitized[2].Content, `{"story_id": "US-003"}`)
}

func TestSanitizeAndCapStories_SmallCLICapping(t *testing.T) {
	raw := []RawStoryItem{
		{
			Filename: "roadmap/user-stories/US-001-core-commands.md",
			Content:  `# User Story 001: Core Commands`,
		},
		{
			Filename: "roadmap/user-stories/US-002-secondary-commands.md",
			Content:  `# User Story 002: Secondary Commands`,
		},
		{
			Filename: "roadmap/user-stories/US-003-tertiary-commands.md",
			Content:  `# User Story 003: Tertiary Commands`,
		},
		{
			Filename: "roadmap/user-stories/US-004-project-hardening-and-maintenance.md",
			Content:  `# User Story 004: Project Hardening`,
		},
	}

	smallCLISpec := `# Word Count CLI
A small CLI tool with stdin, stdout, flags, exit code 0. CU < 35.`

	sanitized := SanitizeAndCapStories("/tmp", raw, smallCLISpec, 4)
	// Small CLI must cap feature stories to 1 + hardening = 2 stories total
	assert.Len(t, sanitized, 2)
	assert.Equal(t, "roadmap/user-stories/US-001-core-commands.md", sanitized[0].Filename)
	assert.Equal(t, "roadmap/user-stories/US-002-project-hardening-and-maintenance.md", sanitized[1].Filename)
}

func TestCapPlannedTasks(t *testing.T) {
	tasks := []domain.Task{
		{ID: "US-001-TASK-001", Title: "Scaffold", Description: "Desc 1", TargetFiles: []string{"main.go"}},
		{ID: "US-001-TASK-002", Title: "Entity", Description: "Desc 2", TargetFiles: []string{"entity.go"}},
		{ID: "US-001-TASK-003", Title: "Repo", Description: "Desc 3", TargetFiles: []string{"repo.go"}},
		{ID: "US-001-TASK-004", Title: "CLI", Description: "Desc 4", TargetFiles: []string{"cli.go"}},
		{ID: "US-001-TASK-005", Title: "Verify", Description: "Desc 5", TargetFiles: []string{"e2e_test.go"}},
	}

	capped := CapPlannedTasks(tasks, 3)
	assert.Len(t, capped, 3)
	assert.Equal(t, "US-001-TASK-001", capped[0].ID)
	assert.Equal(t, "US-001-TASK-002", capped[1].ID)
	assert.Equal(t, "US-001-TASK-005", capped[2].ID)

	// Middle task contains merged descriptions and files
	assert.Contains(t, capped[1].Description, "Desc 2")
	assert.Contains(t, capped[1].Description, "Desc 3")
	assert.Contains(t, capped[1].Description, "Desc 4")
	assert.Contains(t, capped[1].TargetFiles, "entity.go")
	assert.Contains(t, capped[1].TargetFiles, "repo.go")
	assert.Contains(t, capped[1].TargetFiles, "cli.go")
}
