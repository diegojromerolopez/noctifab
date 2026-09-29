package services

import (
	"testing"

	"github.com/diegojromerolopez/noctifab/pkg/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPlanIntegrityValidator_Dependencies(t *testing.T) {
	v := NewPlanIntegrityValidator()

	t.Run("valid linear DAG passes", func(t *testing.T) {
		entries := []StoryDependencyEntry{
			{StoryID: "US-001", Dependencies: nil},
			{StoryID: "US-002", Dependencies: []string{"US-001"}},
			{StoryID: "US-003", Dependencies: []string{"US-002"}},
		}
		err := v.ValidateStoryDependencies(entries)
		require.NoError(t, err)
	})

	t.Run("empty story id fails", func(t *testing.T) {
		entries := []StoryDependencyEntry{
			{StoryID: "", Dependencies: nil},
		}
		err := v.ValidateStoryDependencies(entries)
		assert.ErrorContains(t, err, "empty StoryID")
	})

	t.Run("duplicate story id fails", func(t *testing.T) {
		entries := []StoryDependencyEntry{
			{StoryID: "US-001"},
			{StoryID: "US-001"},
		}
		err := v.ValidateStoryDependencies(entries)
		assert.ErrorContains(t, err, "duplicate story ID")
	})

	t.Run("self-referential dependency fails", func(t *testing.T) {
		entries := []StoryDependencyEntry{
			{StoryID: "US-001", Dependencies: []string{"US-001"}},
		}
		err := v.ValidateStoryDependencies(entries)
		assert.ErrorContains(t, err, "self-referential")
	})

	t.Run("hallucinated non-existent dependency fails", func(t *testing.T) {
		entries := []StoryDependencyEntry{
			{StoryID: "US-001", Dependencies: []string{"US-999"}},
		}
		err := v.ValidateStoryDependencies(entries)
		assert.ErrorContains(t, err, "hallucinated dependency")
	})

	t.Run("circular dependency fails", func(t *testing.T) {
		entries := []StoryDependencyEntry{
			{StoryID: "US-001", Dependencies: []string{"US-002"}},
			{StoryID: "US-002", Dependencies: []string{"US-001"}},
		}
		err := v.ValidateStoryDependencies(entries)
		assert.ErrorContains(t, err, "circular dependency")
	})
}

func TestPlanIntegrityValidator_ContractRegexes(t *testing.T) {
	v := NewPlanIntegrityValidator()

	t.Run("valid contract passes", func(t *testing.T) {
		contracts := []domain.PublicContract{
			{
				ID:                 "C-01",
				Interface:          "cli",
				AllowedExecutables: []string{"bin/app"},
				StdoutContains:     []string{"(?i)hello\\s+world"},
				StderrPrefixes:     []string{"ERROR:"},
			},
		}
		err := v.ValidateContractRegexes(contracts)
		require.NoError(t, err)
	})

	t.Run("invalid stdout regex fails", func(t *testing.T) {
		contracts := []domain.PublicContract{
			{
				ID:                 "C-02",
				Interface:          "cli",
				AllowedExecutables: []string{"bin/app"},
				StdoutContains:     []string{"(?invalid["},
			},
		}
		err := v.ValidateContractRegexes(contracts)
		assert.ErrorContains(t, err, "invalid stdout_contains regex")
	})

	t.Run("invalid stderr regex fails", func(t *testing.T) {
		contracts := []domain.PublicContract{
			{
				ID:                 "C-03",
				Interface:          "cli",
				AllowedExecutables: []string{"bin/app"},
				StderrPrefixes:     []string{"[unclosed"},
			},
		}
		err := v.ValidateContractRegexes(contracts)
		assert.ErrorContains(t, err, "invalid stderr_prefixes regex")
	})

	t.Run("cli missing executables fails", func(t *testing.T) {
		contracts := []domain.PublicContract{
			{
				ID:                 "C-04",
				Interface:          "cli",
				AllowedExecutables: nil,
			},
		}
		err := v.ValidateContractRegexes(contracts)
		assert.ErrorContains(t, err, "provides no allowed_executables")
	})
}

func TestPlanIntegrityValidator_SpecTraceability(t *testing.T) {
	v := NewPlanIntegrityValidator()

	spec := `# System Spec
Requirements:
- [REQ-AUTH-01] User login via token
- [REQ-DB-02] SQLite persistence
`

	t.Run("full coverage passes", func(t *testing.T) {
		stories := []string{
			"Story 1: Implements [REQ-AUTH-01] token authentication",
			"Story 2: Implements [REQ-DB-02] storage layer",
		}
		missing, err := v.ValidateSpecTraceability(stories, spec)
		require.NoError(t, err)
		assert.Empty(t, missing)
	})

	t.Run("missing requirement tag fails", func(t *testing.T) {
		stories := []string{
			"Story 1: Implements [REQ-AUTH-01] token authentication",
		}
		missing, err := v.ValidateSpecTraceability(stories, spec)
		assert.Error(t, err)
		assert.Contains(t, missing, "REQ-DB-02")
	})
}
