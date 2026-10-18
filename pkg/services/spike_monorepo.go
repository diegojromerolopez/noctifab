package services

import (
	"strings"
)

// IsMonorepoSpec detects whether a specification describes a multi-tier, full-stack,
// or monorepo architecture (e.g. frontend + backend, client + server).
// Generating an entire fullstack monorepo in a single 180s spike turn routinely exceeds
// LLM context deadlines; detecting it allows graceful bypass to streaming Product Manager user stories.
func IsMonorepoSpec(specContent string) bool {
	lower := strings.ToLower(specContent)

	hasClient := strings.Contains(lower, "client/") ||
		strings.Contains(lower, "frontend/") ||
		strings.Contains(lower, "ui/") ||
		strings.Contains(lower, "web/") ||
		strings.Contains(lower, "frontend architecture") ||
		strings.Contains(lower, "react") ||
		strings.Contains(lower, "vue") ||
		strings.Contains(lower, "svelte")

	hasServer := strings.Contains(lower, "server/") ||
		strings.Contains(lower, "backend/") ||
		strings.Contains(lower, "api/") ||
		strings.Contains(lower, "backend architecture") ||
		strings.Contains(lower, "fastify") ||
		strings.Contains(lower, "express") ||
		strings.Contains(lower, "spring boot") ||
		strings.Contains(lower, "django")

	if hasClient && hasServer {
		return true
	}

	// Explicit monorepo markers
	if strings.Contains(lower, "monorepo") ||
		strings.Contains(lower, "pnpm-workspace") ||
		strings.Contains(lower, "workspaces:") ||
		strings.Contains(lower, "turborepo") ||
		strings.Contains(lower, "nx ") {
		return true
	}

	return false
}
