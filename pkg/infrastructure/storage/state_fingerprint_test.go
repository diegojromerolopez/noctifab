package storage

import (
	"testing"
	"time"

	"github.com/diegojromerolopez/noctifab/pkg/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func fingerprintFixtureState() *domain.State {
	now := time.Date(2026, 8, 4, 12, 0, 0, 0, time.UTC)
	return &domain.State{
		ID:          "state-fp",
		ProjectPath: "/workspace",
		Tasks: []domain.Task{
			{ID: "t1", Title: "Task 1", Status: domain.TaskPending, DependsOn: []string{}, CreatedAt: now, UpdatedAt: now},
			{ID: "t2", Title: "Task 2", Status: domain.TaskSuccess, DependsOn: []string{"t1"}, CreatedAt: now, UpdatedAt: now},
		},
		Clarifications:     []domain.Clarification{{Question: "Q?", Answer: "A", Resolved: true, AskedAt: now}},
		LastActions:        []domain.Action{{Timestamp: now, Tool: "write", Args: map[string]any{"b": 2.0, "a": 1.0}, Reasoning: "r", Result: "ok", Success: true}},
		Files:              []domain.FileInfo{{Path: "main.go", Size: 10, LastModified: now}},
		ValidationCriteria: []domain.ValidationCriterion{{ID: "c1", Type: domain.ValidationCommand, Expression: "go test"}},
		ActiveAgents:       []domain.Agent{{ID: "a1", Name: "gen", Role: domain.AgentRoleGenerator, Status: domain.AgentIdle}},
	}
}

func TestComputeStateFingerprints(t *testing.T) {
	t.Run("when computed twice over the same state, it is deterministic", func(t *testing.T) {
		first, err := computeStateFingerprints(fingerprintFixtureState())
		require.NoError(t, err)
		second, err := computeStateFingerprints(fingerprintFixtureState())
		require.NoError(t, err)
		assert.Equal(t, first, second)
	})

	t.Run("when computed, it covers every relation group", func(t *testing.T) {
		fps, err := computeStateFingerprints(fingerprintFixtureState())
		require.NoError(t, err)
		assert.Len(t, fps, len(stateRelationGroups))
		for _, group := range stateRelationGroups {
			_, ok := fps[group]
			assert.True(t, ok, "missing fingerprint for group %s", group)
		}
	})

	t.Run("when a field changes in one group, it changes only that group's hash", func(t *testing.T) {
		base, err := computeStateFingerprints(fingerprintFixtureState())
		require.NoError(t, err)

		mutations := map[string]func(*domain.State){
			groupTasks:              func(s *domain.State) { s.Tasks[0].Progress = 55 },
			groupClarifications:     func(s *domain.State) { s.Clarifications[0].Answer = "changed" },
			groupActions:            func(s *domain.State) { s.LastActions[0].Result = "changed" },
			groupWorkspaceFiles:     func(s *domain.State) { s.Files[0].Size = 999 },
			groupValidationCriteria: func(s *domain.State) { s.ValidationCriteria[0].Passed = true },
			groupActiveAgents:       func(s *domain.State) { s.ActiveAgents[0].Status = domain.AgentWorking },
		}

		for mutatedGroup, mutate := range mutations {
			state := fingerprintFixtureState()
			mutate(state)
			fps, err := computeStateFingerprints(state)
			require.NoError(t, err)
			for _, group := range stateRelationGroups {
				if group == mutatedGroup {
					assert.NotEqual(t, base[group], fps[group], "expected %s hash to change", group)
				} else {
					assert.Equal(t, base[group], fps[group], "expected %s hash to stay stable when mutating %s", group, mutatedGroup)
				}
			}
		}
	})

	t.Run("when a group's rows are unserializable, it returns an error", func(t *testing.T) {
		state := fingerprintFixtureState()
		state.LastActions[0].Args = map[string]any{"bad": make(chan int)}
		_, err := computeStateFingerprints(state)
		assert.Error(t, err)
	})
}

func TestIsGroupClean(t *testing.T) {
	fresh, err := computeStateFingerprints(fingerprintFixtureState())
	require.NoError(t, err)

	t.Run("when the cache is nil, it reports every group dirty", func(t *testing.T) {
		assert.False(t, isGroupClean(nil, fresh, groupTasks))
	})

	t.Run("when the cached hash matches, it reports the group clean", func(t *testing.T) {
		assert.True(t, isGroupClean(fresh, fresh, groupTasks))
	})

	t.Run("when the cached hash differs, it reports the group dirty", func(t *testing.T) {
		cached := stateFingerprints{groupTasks: groupFingerprint{0xFF}}
		assert.False(t, isGroupClean(cached, fresh, groupTasks))
	})

	t.Run("when the cached map lacks the group, it reports the group dirty", func(t *testing.T) {
		assert.False(t, isGroupClean(stateFingerprints{}, fresh, groupTasks))
	})
}

func TestFingerprintCache(t *testing.T) {
	t.Run("when the cache is zero-valued, get returns nil and invalidate is safe", func(t *testing.T) {
		var cache fingerprintCache
		assert.Nil(t, cache.get("unknown"))
		cache.invalidate("unknown")
	})

	t.Run("when fingerprints are set, get returns them", func(t *testing.T) {
		var cache fingerprintCache
		fps := stateFingerprints{groupTasks: groupFingerprint{1}}
		cache.set("s1", fps)
		assert.Equal(t, fps, cache.get("s1"))
	})

	t.Run("when a state is invalidated, its fingerprints are gone but others remain", func(t *testing.T) {
		var cache fingerprintCache
		cache.set("s1", stateFingerprints{groupTasks: groupFingerprint{1}})
		cache.set("s2", stateFingerprints{groupTasks: groupFingerprint{2}})
		cache.invalidate("s1")
		assert.Nil(t, cache.get("s1"))
		assert.NotNil(t, cache.get("s2"))
	})
}
