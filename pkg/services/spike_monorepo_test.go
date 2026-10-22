package services_test

import (
	"testing"

	"github.com/diegojromerolopez/noctifab/pkg/services"
	"github.com/stretchr/testify/assert"
)

func TestIsMonorepoSpec(t *testing.T) {
	t.Run("Single-tier CLI returns false", func(t *testing.T) {
		spec := "# Echo CLI\nA simple command-line utility in Go that echoes input arguments."
		assert.False(t, services.IsMonorepoSpec(spec))
	})

	t.Run("Fullstack React + Fastify monorepo returns true", func(t *testing.T) {
		spec := "# Notebook\nFrontend client in React and backend API server in Fastify with PostgreSQL."
		assert.True(t, services.IsMonorepoSpec(spec))
	})

	t.Run("Explicit monorepo workspace returns true", func(t *testing.T) {
		spec := "# Monorepo Platform\nConfigured with pnpm-workspace.yaml across packages."
		assert.True(t, services.IsMonorepoSpec(spec))
	})
}
