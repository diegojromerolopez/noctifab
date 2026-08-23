package services

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/diegojromerolopez/noctifab/pkg/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetLogWindowLines(t *testing.T) {
	assert.Equal(t, 50, getLogWindowLines(0))
	assert.Equal(t, 500, getLogWindowLines(1))
	assert.Equal(t, 5000, getLogWindowLines(2))
	assert.Equal(t, 5000, getLogWindowLines(5))
}

func TestUnblocker_FastPathAndEscalation(t *testing.T) {
	t.Run("fast-path regex intercepts interactive stdin prompt and sends ResetTaskCmd with directive", func(t *testing.T) {
		tmpDir := t.TempDir()
		taskLogDir := filepath.Join(tmpDir, ".noctifab", "logs", "tasks")
		require.NoError(t, os.MkdirAll(taskLogDir, 0755))

		// Create log with interactive prompt
		logContent := "npm WARN deprecated\n? Do you want to proceed? (Y/n)\n"
		require.NoError(t, os.WriteFile(filepath.Join(taskLogDir, "task-1.log"), []byte(logContent), 0644))

		// Change Cwd context by creating file in relative path
		currDir, _ := os.Getwd()
		defer func() { _ = os.Chdir(currDir) }()
		_ = os.Chdir(tmpDir)

		state := &domain.State{
			StoryStatus: domain.StoryRunning,
			Tasks: []domain.Task{
				{
					ID:        "task-1",
					Title:     "Install Deps",
					Status:    domain.TaskInProgress,
					UpdatedAt: time.Now().Add(-15 * time.Minute),
				},
			},
			ActiveAgents: []domain.Agent{
				{ID: "agent-1", TaskID: "task-1", Status: domain.AgentWorking, StartedAt: time.Now().Add(-15 * time.Minute)},
			},
		}

		repo := &inMemoryRepo{state: state}
		mailbox := NewCommandMailbox(repo)
		unblocker := NewUnblockerAgent(repo, nil, mailbox, 10*time.Millisecond, 3, 5*time.Minute, 10*time.Minute, false)

		ctx := context.Background()
		unblocker.checkAndUnblock(ctx)

		cmds := mailbox.PopAll()
		require.Len(t, cmds, 1)
		resetCmd, ok := cmds[0].(*ResetTaskCmd)
		require.True(t, ok)
		assert.Equal(t, "task-1", resetCmd.TaskID)
		assert.Equal(t, "interactive_stdin_prompt_wait", resetCmd.Reason)
		assert.Contains(t, resetCmd.Directive, "non-interactively")
	})

	t.Run("fast-path regex intercepts missing toolchain binary and sends ResetTaskCmd with fallback directive", func(t *testing.T) {
		tmpDir := t.TempDir()
		taskLogDir := filepath.Join(tmpDir, ".noctifab", "logs", "tasks")
		require.NoError(t, os.MkdirAll(taskLogDir, 0755))

		logContent := "sh: 1: pytest: not found\nexit status 127\n"
		require.NoError(t, os.WriteFile(filepath.Join(taskLogDir, "task-pytest.log"), []byte(logContent), 0644))

		currDir, _ := os.Getwd()
		defer func() { _ = os.Chdir(currDir) }()
		_ = os.Chdir(tmpDir)

		state := &domain.State{
			StoryStatus: domain.StoryRunning,
			Tasks: []domain.Task{
				{
					ID:        "task-pytest",
					Title:     "Run Pytest",
					Status:    domain.TaskInProgress,
					UpdatedAt: time.Now().Add(-15 * time.Minute),
				},
			},
			ActiveAgents: []domain.Agent{
				{ID: "agent-pytest", TaskID: "task-pytest", Status: domain.AgentWorking, StartedAt: time.Now().Add(-15 * time.Minute)},
			},
		}

		repo := &inMemoryRepo{state: state}
		mailbox := NewCommandMailbox(repo)
		unblocker := NewUnblockerAgent(repo, nil, mailbox, 10*time.Millisecond, 5, 5*time.Minute, 10*time.Minute, false)

		ctx := context.Background()
		unblocker.checkAndUnblock(ctx)

		cmds := mailbox.PopAll()
		require.Len(t, cmds, 1)
		resetCmd, ok := cmds[0].(*ResetTaskCmd)
		require.True(t, ok)
		assert.Equal(t, "task-pytest", resetCmd.TaskID)
		assert.Equal(t, "missing_toolchain_binary", resetCmd.Reason)
		assert.Contains(t, resetCmd.Directive, "standard library")
	})

	t.Run("hard stop fails task when StallCount >= 5", func(t *testing.T) {
		state := &domain.State{
			StoryStatus: domain.StoryRunning,
			Tasks: []domain.Task{
				{
					ID:         "task-stuck",
					Title:      "Stuck Task",
					Status:     domain.TaskInProgress,
					StallCount: 5,
					UpdatedAt:  time.Now().Add(-15 * time.Minute),
				},
			},
		}

		repo := &inMemoryRepo{state: state}
		mailbox := NewCommandMailbox(repo)
		unblocker := NewUnblockerAgent(repo, nil, mailbox, 10*time.Millisecond, 5, 5*time.Minute, 10*time.Minute, false)

		ctx := context.Background()
		unblocker.checkAndUnblock(ctx)

		cmds := mailbox.PopAll()
		require.Len(t, cmds, 1)
		failCmd, ok := cmds[0].(*FailTaskCmd)
		require.True(t, ok)
		assert.Equal(t, "task-stuck", failCmd.TaskID)
		assert.Contains(t, failCmd.Reason, "max stall escalations")
	})

	t.Run("pre-hard-stop escalates to Last-Resort Agent when StallCount >= 4", func(t *testing.T) {
		state := &domain.State{
			StoryStatus: domain.StoryRunning,
			Tasks: []domain.Task{
				{
					ID:         "task-lra-stall",
					Title:      "Stalling Task",
					Status:     domain.TaskInProgress,
					StallCount: 4,
					UpdatedAt:  time.Now().Add(-15 * time.Minute),
				},
			},
		}

		repo := &inMemoryRepo{state: state}
		mailbox := NewCommandMailbox(repo)
		unblocker := NewUnblockerAgent(repo, nil, mailbox, 10*time.Millisecond, 5, 5*time.Minute, 10*time.Minute, false)

		ctx := context.Background()
		unblocker.checkAndUnblock(ctx)

		cmds := mailbox.PopAll()
		require.Len(t, cmds, 1)
		resetCmd, ok := cmds[0].(*ResetTaskCmd)
		require.True(t, ok)
		assert.Equal(t, "task-lra-stall", resetCmd.TaskID)
		assert.Contains(t, resetCmd.Reason, "escalating to Last-Resort Agent")
		assert.Contains(t, resetCmd.Directive, "SOVEREIGN REPAIR DIRECTIVE")
	})
}
