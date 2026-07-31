package cli

import (
	"testing"
	"time"

	"github.com/diegojromerolopez/noctifab/pkg/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDashboard_RenderDashboard(t *testing.T) {
	t.Run("when states slice is empty, it returns message", func(t *testing.T) {
		out := renderDashboard(nil)
		assert.Contains(t, out, "No active user stories found in the daemon.")
	})

	t.Run("when states slice is populated, it renders layout panels", func(t *testing.T) {
		now := time.Now()
		state := &domain.State{
			ID:          "state-123",
			ProjectPath: "/Users/dev/project",
			StoryStatus: domain.StoryRunning,
			Metadata: domain.StateMetadata{
				FeatureName:       "Feature X",
				TotalCostUSD:      "0.015",
				TotalTokensUsed:   500,
				IntegrationBranch: "noctifab/story-feature-x",
			},
			Tasks: []domain.Task{
				{
					ID:         "task-1",
					Title:      "Write dashboard test",
					Status:     domain.TaskSuccess,
					Progress:   100,
					AssignedTo: "generator",
				},
				{
					ID:         "task-2",
					Title:      "Fix bugs",
					Status:     domain.TaskInProgress,
					Progress:   50,
					AssignedTo: "generator",
				},
			},
			LastActions: []domain.Action{
				{
					Timestamp: now,
					Tool:      "write_file",
					Success:   true,
					Result:    "file written successfully",
				},
			},
		}

		out := renderDashboard([]*domain.State{state})

		// Part 1: Header panel assertions
		assert.Contains(t, out, "NOCTIFAB TERMINAL DASHBOARD")
		assert.Contains(t, out, "/Users/dev/project")
		assert.Contains(t, out, "Global Status: RUNNING")
		assert.Contains(t, out, "Elapsed Time:")
		assert.Contains(t, out, "Cost: $0.015")
		assert.Contains(t, out, "Tokens Used: 500")

		// Part 2: Main progress board assertions
		assert.Contains(t, out, "Feature X")
		assert.Contains(t, out, "Progress: [███████░░░] 75%")
		// All tasks are now shown regardless of status
		assert.Contains(t, out, "✅ Write dashboard test (100%)")
		assert.Contains(t, out, "🔄 Fix bugs (50%)")

		// Part 4: Interactive Footer assertions
		assert.Contains(t, out, "Quit")
		assert.Contains(t, out, "Pause/Resume")
		assert.Contains(t, out, "Cancel")
	})

	t.Run("when story status is SUCCESS, all tasks are shown with their final status", func(t *testing.T) {
		state := &domain.State{
			ID:          "state-456",
			ProjectPath: "/Users/dev/project",
			StoryStatus: domain.StorySuccess,
			Metadata: domain.StateMetadata{
				FeatureName:     "Feature Y",
				TotalCostUSD:    "0.010",
				TotalTokensUsed: 200,
			},
			Tasks: []domain.Task{
				{
					ID:         "task-3",
					Title:      "Implement feature",
					Status:     domain.TaskSuccess,
					Progress:   100,
					AssignedTo: "generator",
				},
			},
		}

		out := renderDashboard([]*domain.State{state})

		assert.Contains(t, out, "Global Status: SUCCESS ✅")
		assert.Contains(t, out, "✅ Implement feature (100%)")
	})

	t.Run("when story status is FAILED, failed tasks show the failure reason inline", func(t *testing.T) {
		state := &domain.State{
			ID:          "state-789",
			ProjectPath: "/Users/dev/project",
			StoryStatus: domain.StoryFailed,
			Metadata: domain.StateMetadata{
				FeatureName:     "Feature Z",
				TotalCostUSD:    "0.020",
				TotalTokensUsed: 300,
			},
			Tasks: []domain.Task{
				{
					ID:         "task-4",
					Title:      "Build binary",
					Status:     domain.TaskFailed,
					Progress:   0,
					AssignedTo: "generator",
					FailureLog: "Linter validation failed. Command: cargo fmt --check.\nDiff in src/lib.rs:38:\n+\n",
				},
			},
		}

		out := renderDashboard([]*domain.State{state})

		assert.Contains(t, out, "Global Status: FAILED ❌")
		assert.Contains(t, out, "Build binary")
		// Last non-blank line of the log is the actual diff content (+), not the header
		assert.Contains(t, out, "❌ Build binary (0%) — +")
		assert.NotContains(t, out, "Linter validation failed")
	})

	t.Run("when active agents and actions log exist, it renders agent visibility and completed work", func(t *testing.T) {
		now := time.Now()
		state := &domain.State{
			ID:          "state-active-agents",
			ProjectPath: "/Users/dev/project",
			StoryStatus: domain.StoryRunning,
			ActiveAgents: []domain.Agent{
				{
					ID:        "agent-gen-1",
					Name:      "generator-task-1",
					Role:      domain.AgentRoleGenerator,
					Status:    domain.AgentWorking,
					TaskID:    "task-1",
					StartedAt: now,
				},
			},
			LastActions: []domain.Action{
				{
					Timestamp: now,
					Tool:      "write_file",
					Success:   true,
					Result:    "Created main.go",
				},
			},
			Tasks: []domain.Task{
				{
					ID:       "task-1",
					Title:    "Write core application",
					Status:   domain.TaskInProgress,
					Progress: 50,
				},
			},
		}

		out := renderDashboard([]*domain.State{state})

		assert.Contains(t, out, "ACTIVE AGENT WORKERS:")
		assert.Contains(t, out, "GENERATOR")
		assert.Contains(t, out, "generator-task-1")
		assert.Contains(t, out, "RECENT COMPLETED ACTIONS LOG (WHAT'S BEEN DONE):")
		assert.Contains(t, out, "write_file")
		assert.Contains(t, out, "Created main.go")
		assert.Contains(t, out, "New Order/Prompt")
		assert.Contains(t, out, "Resolve Clarifications")
	})
}

func TestDashboardCmd_Configuration(t *testing.T) {
	t.Run("dashboard command exists in root command", func(t *testing.T) {
		found := false
		for _, cmd := range RootCmd.Commands() {
			if cmd.Name() == "dashboard" {
				found = true
				assert.Equal(t, "Launch the real-time terminal user interface progress dashboard", cmd.Short)
				break
			}
		}
		require.True(t, found, "dashboard command not found in RootCmd")
	})
}

func TestStatusEmoji(t *testing.T) {
	t.Run("when status is RUNNING it returns a hourglass frame", func(t *testing.T) {
		e := statusEmoji(domain.StoryRunning)
		assert.True(t, e == "⏳" || e == "⌛", "expected hourglass frame, got %q", e)
	})
	t.Run("when status is SUCCESS it returns a green tick", func(t *testing.T) {
		assert.Equal(t, "✅", statusEmoji(domain.StorySuccess))
	})
	t.Run("when status is FAILED it returns a red X", func(t *testing.T) {
		assert.Equal(t, "❌", statusEmoji(domain.StoryFailed))
	})
	t.Run("when status is PAUSED it returns a stop sign", func(t *testing.T) {
		assert.Equal(t, "🛑", statusEmoji(domain.StoryPaused))
	})
	t.Run("when status is CANCELLED it returns a no-entry sign", func(t *testing.T) {
		assert.Equal(t, "⛔", statusEmoji(domain.StoryCancelled))
	})
	t.Run("when status is idle/unknown it returns an empty string", func(t *testing.T) {
		assert.Equal(t, "", statusEmoji(domain.StoryIdle))
	})
}
