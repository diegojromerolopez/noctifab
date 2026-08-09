package storage

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"sync"

	"github.com/diegojromerolopez/noctifab/pkg/domain"
)

// Relation group names shared by the SQLite and PostgreSQL repositories.
// They match the underlying relation table names.
const (
	groupTasks              = "tasks"
	groupClarifications     = "clarifications"
	groupActions            = "actions"
	groupWorkspaceFiles     = "workspace_files"
	groupValidationCriteria = "validation_criteria"
	groupActiveAgents       = "active_agents"
	groupQAReviews          = "qa_reviews"
)

// stateRelationGroups lists every relation group persisted alongside the
// state row, in the canonical write order used by Save and by prune deletes.
var stateRelationGroups = []string{
	groupTasks,
	groupClarifications,
	groupActions,
	groupWorkspaceFiles,
	groupValidationCriteria,
	groupActiveAgents,
	groupQAReviews,
}

// stateRelationTables lists physical child tables in foreign-key-safe delete order.
var stateRelationTables = []string{
	"qa_findings",
	"qa_scenarios",
	"review_phases",
	"story_contracts",
	groupTasks,
	groupClarifications,
	groupActions,
	groupWorkspaceFiles,
	groupValidationCriteria,
	groupActiveAgents,
}

// groupFingerprint is a content hash of a deterministic serialization of one
// relation group's rows.
type groupFingerprint [sha256.Size]byte

// stateFingerprints maps a relation group name to its content fingerprint.
type stateFingerprints map[string]groupFingerprint

// computeStateFingerprints hashes each relation group of the given state
// using a deterministic JSON serialization (encoding/json sorts map keys, so
// Action.Args maps serialize deterministically).
func computeStateFingerprints(state *domain.State) (stateFingerprints, error) {
	groups := map[string]any{
		groupTasks:              state.Tasks,
		groupClarifications:     state.Clarifications,
		groupActions:            state.LastActions,
		groupWorkspaceFiles:     state.Files,
		groupValidationCriteria: state.ValidationCriteria,
		groupActiveAgents:       state.ActiveAgents,
		groupQAReviews: struct {
			StoryContracts []domain.StoryContract
			ReviewPhases   []domain.ReviewPhase
			QAScenarios    []domain.QAScenario
			QAFindings     []domain.QAFinding
		}{state.StoryContracts, state.ReviewPhases, state.QAScenarios, state.QAFindings},
	}
	fingerprints := make(stateFingerprints, len(groups))
	for name, rows := range groups {
		data, err := json.Marshal(rows)
		if err != nil {
			return nil, fmt.Errorf("failed to fingerprint relation group %s: %w", name, err)
		}
		fingerprints[name] = sha256.Sum256(data)
	}
	return fingerprints, nil
}

// isGroupClean reports whether the given relation group is unchanged with
// respect to the cached fingerprints. A nil cache means everything is dirty.
func isGroupClean(cached, fresh stateFingerprints, group string) bool {
	if cached == nil {
		return false
	}
	cachedFP, ok := cached[group]
	if !ok {
		return false
	}
	return cachedFP == fresh[group]
}

// fingerprintCache is a mutex-protected per-repository cache mapping a state
// ID to the per-group fingerprints last known to be committed to the
// database. The zero value is ready to use.
type fingerprintCache struct {
	mu        sync.Mutex
	byStateID map[string]stateFingerprints
}

// get returns the cached fingerprints for a state ID, or nil when unknown.
func (c *fingerprintCache) get(stateID string) stateFingerprints {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.byStateID[stateID]
}

// set records the fingerprints for a state ID. Callers must only invoke set
// AFTER the corresponding transaction has committed successfully.
func (c *fingerprintCache) set(stateID string, fingerprints stateFingerprints) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.byStateID == nil {
		c.byStateID = make(map[string]stateFingerprints)
	}
	c.byStateID[stateID] = fingerprints
}

// invalidate removes the cached fingerprints for a state ID. It must be
// called on any Save failure (especially version conflicts) because another
// writer may have changed rows and the cache no longer reflects DB content.
func (c *fingerprintCache) invalidate(stateID string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.byStateID, stateID)
}
