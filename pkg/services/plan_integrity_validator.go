package services

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/diegojromerolopez/noctifab/pkg/domain"
)

// StoryDependencyEntry represents a story and its declared prerequisite story IDs.
type StoryDependencyEntry struct {
	StoryID      string
	Dependencies []string
}

// PlanIntegrityViolation records a deterministic flaw in a generated roadmap or story plan.
type PlanIntegrityViolation struct {
	Stage   string `json:"stage"` // "DEPENDENCY", "CONTRACT", "TRACEABILITY"
	StoryID string `json:"story_id,omitempty"`
	Message string `json:"message"`
}

// PlanIntegrityValidator enforces deterministic correctness across generated roadmaps,
// story dependency graphs, contract regular expressions, and spec requirements coverage.
type PlanIntegrityValidator struct{}

// NewPlanIntegrityValidator creates an instance of PlanIntegrityValidator.
func NewPlanIntegrityValidator() *PlanIntegrityValidator {
	return &PlanIntegrityValidator{}
}

// ValidateStoryDependencies verifies referential integrity and acyclicity across all stories.
func (v *PlanIntegrityValidator) ValidateStoryDependencies(entries []StoryDependencyEntry) error {
	storySet := make(map[string]bool)
	inDegree := make(map[string]int)
	adj := make(map[string][]string)

	for _, entry := range entries {
		normID := strings.ToUpper(strings.TrimSpace(entry.StoryID))
		if normID == "" {
			return fmt.Errorf("invalid plan: story has empty StoryID")
		}
		if storySet[normID] {
			return fmt.Errorf("duplicate story ID detected: %s", normID)
		}
		storySet[normID] = true
		inDegree[normID] = 0
	}

	for _, entry := range entries {
		normID := strings.ToUpper(strings.TrimSpace(entry.StoryID))
		for _, dep := range entry.Dependencies {
			normDep := strings.ToUpper(strings.TrimSpace(dep))
			if normDep == "" {
				continue
			}
			if normDep == normID {
				return fmt.Errorf("self-referential dependency detected on story %s", normID)
			}
			if !storySet[normDep] {
				return fmt.Errorf("hallucinated dependency: story %s depends on non-existent %s", normID, normDep)
			}
			adj[normDep] = append(adj[normDep], normID)
			inDegree[normID]++
		}
	}

	// Kahn's algorithm for topological sorting / cycle detection
	queue := make([]string, 0, len(entries))
	for id, deg := range inDegree {
		if deg == 0 {
			queue = append(queue, id)
		}
	}

	visitedCount := 0
	for len(queue) > 0 {
		curr := queue[0]
		queue = queue[1:]
		visitedCount++

		for _, neighbor := range adj[curr] {
			inDegree[neighbor]--
			if inDegree[neighbor] == 0 {
				queue = append(queue, neighbor)
			}
		}
	}

	if visitedCount != len(entries) {
		return fmt.Errorf("circular dependency detected in roadmap user story graph")
	}

	return nil
}

// ValidateContractRegexes verifies that all regex patterns in public contracts compile cleanly.
func (v *PlanIntegrityValidator) ValidateContractRegexes(contracts []domain.PublicContract) error {
	for _, pc := range contracts {
		for _, pat := range pc.StdoutContains {
			if _, err := regexp.Compile(pat); err != nil {
				return fmt.Errorf("invalid stdout_contains regex in contract %q: %w", pc.ID, err)
			}
		}
		for _, pat := range pc.StderrPrefixes {
			if _, err := regexp.Compile(pat); err != nil {
				return fmt.Errorf("invalid stderr_prefixes regex in contract %q: %w", pc.ID, err)
			}
		}
		if pc.Interface == "cli" && len(pc.AllowedExecutables) == 0 {
			return fmt.Errorf("contract %q defines CLI interface but provides no allowed_executables", pc.ID)
		}
	}
	return nil
}

// ValidateSpecTraceability checks that key requirement tags or major section headers
// from SPEC.md are explicitly referenced or mapped across the roadmap story contents.
func (v *PlanIntegrityValidator) ValidateSpecTraceability(storyContents []string, specContent string) ([]string, error) {
	tagRegex := regexp.MustCompile(`(?i)\[(REQ-[A-Za-z0-9_-]+)\]`)
	matches := tagRegex.FindAllStringSubmatch(specContent, -1)

	var missingTags []string
	if len(matches) > 0 {
		requiredTags := make(map[string]bool)
		for _, m := range matches {
			if len(m) > 1 {
				requiredTags[strings.ToUpper(m[1])] = true
			}
		}

		combinedStories := strings.ToUpper(strings.Join(storyContents, "\n"))
		for tag := range requiredTags {
			if !strings.Contains(combinedStories, tag) {
				missingTags = append(missingTags, tag)
			}
		}
	}

	if len(missingTags) > 0 {
		return missingTags, fmt.Errorf("spec traceability violation: missing coverage for requirement tags: %s", strings.Join(missingTags, ", "))
	}
	return nil, nil
}
