package services

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/diegojromerolopez/noctifab/pkg/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type mockAuditorLLM struct {
	response *domain.LLMResponse
	err      error
}

func (m *mockAuditorLLM) Complete(ctx context.Context, prompt string) (*domain.LLMResponse, error) {
	return m.response, m.err
}

func TestAcceptanceAuditor(t *testing.T) {
	t.Run("passes when no project path or state", func(t *testing.T) {
		auditor := NewAcceptanceAuditor(nil, nil)
		res, err := auditor.AuditProjectAcceptance(context.Background(), nil)
		require.NoError(t, err)
		assert.True(t, res.Passed)
	})

	t.Run("passes when SPEC.md does not exist", func(t *testing.T) {
		tmpDir := t.TempDir()
		auditor := NewAcceptanceAuditor(nil, nil)
		state := &domain.State{ProjectPath: tmpDir}
		res, err := auditor.AuditProjectAcceptance(context.Background(), state)
		require.NoError(t, err)
		assert.True(t, res.Passed)
		assert.Contains(t, res.Summary, "not found")
	})

	t.Run("passes when SPEC.md is empty", func(t *testing.T) {
		tmpDir := t.TempDir()
		require.NoError(t, os.WriteFile(filepath.Join(tmpDir, "SPEC.md"), []byte("   \n"), 0600))
		auditor := NewAcceptanceAuditor(nil, nil)
		state := &domain.State{ProjectPath: tmpDir}
		res, err := auditor.AuditProjectAcceptance(context.Background(), state)
		require.NoError(t, err)
		assert.True(t, res.Passed)
	})

	t.Run("passes when no LLM client configured", func(t *testing.T) {
		tmpDir := t.TempDir()
		require.NoError(t, os.WriteFile(filepath.Join(tmpDir, "SPEC.md"), []byte("# Spec\nRequirements"), 0600))
		auditor := NewAcceptanceAuditor(nil, nil)
		state := &domain.State{ProjectPath: tmpDir}
		res, err := auditor.AuditProjectAcceptance(context.Background(), state)
		require.NoError(t, err)
		assert.True(t, res.Passed)
	})

	t.Run("returns passed audit when LLM returns submit_acceptance_audit passed", func(t *testing.T) {
		tmpDir := t.TempDir()
		require.NoError(t, os.WriteFile(filepath.Join(tmpDir, "SPEC.md"), []byte("# Spec\nCommands: GET, SET"), 0600))
		require.NoError(t, os.WriteFile(filepath.Join(tmpDir, "main.go"), []byte("package main"), 0600))

		mock := &mockAuditorLLM{
			response: &domain.LLMResponse{
				Actions: []domain.LLMAction{
					{
						Tool: "submit_acceptance_audit",
						Args: map[string]any{
							"passed":  true,
							"summary": "All specification requirements verified and implemented.",
							"gaps":    []any{},
						},
					},
				},
			},
		}

		auditor := NewAcceptanceAuditor(mock, nil)
		state := &domain.State{
			ProjectPath: tmpDir,
			Tasks: []domain.Task{
				{ID: "task-1", Title: "Implement Core", Status: domain.TaskSuccess, TargetFiles: []string{"main.go"}},
			},
		}
		res, err := auditor.AuditProjectAcceptance(context.Background(), state)
		require.NoError(t, err)
		assert.True(t, res.Passed)
		assert.Contains(t, res.Summary, "All specification requirements")
		assert.Empty(t, res.Gaps)
	})

	t.Run("returns failed audit when LLM reports missing commands/gaps", func(t *testing.T) {
		tmpDir := t.TempDir()
		require.NoError(t, os.WriteFile(filepath.Join(tmpDir, "SPEC.md"), []byte("# Spec\nCommands: PING, ECHO, GET, SET, EXPIRE, TTL, KEYS"), 0600))

		mock := &mockAuditorLLM{
			response: &domain.LLMResponse{
				Actions: []domain.LLMAction{
					{
						Tool: "submit_acceptance_audit",
						Args: map[string]any{
							"passed":  false,
							"summary": "Missing several required Redis commands specified in SPEC.md Section 6.",
							"gaps":    []any{"PING command missing", "EXPIRE and TTL commands missing", "KEYS pattern command missing"},
						},
					},
				},
			},
		}

		auditor := NewAcceptanceAuditor(mock, nil)
		state := &domain.State{
			ProjectPath: tmpDir,
			Tasks: []domain.Task{
				{ID: "task-1", Title: "Basic GET/SET", Status: domain.TaskSuccess, TargetFiles: []string{"commands.py"}},
			},
		}
		res, err := auditor.AuditProjectAcceptance(context.Background(), state)
		require.NoError(t, err)
		assert.False(t, res.Passed)
		assert.Contains(t, res.Summary, "Missing several required Redis commands")
		assert.Len(t, res.Gaps, 3)
		assert.Contains(t, res.Gaps[0], "PING")
	})

	t.Run("parses fallback JSON reasoning from LLM", func(t *testing.T) {
		tmpDir := t.TempDir()
		require.NoError(t, os.WriteFile(filepath.Join(tmpDir, "SPEC.md"), []byte("# Spec"), 0600))

		mock := &mockAuditorLLM{
			response: &domain.LLMResponse{
				Reasoning: `I evaluated the codebase. {"passed": false, "summary": "Missing CLI flags", "gaps": ["--verbose flag missing"]}`,
			},
		}

		auditor := NewAcceptanceAuditor(mock, nil)
		state := &domain.State{ProjectPath: tmpDir}
		res, err := auditor.AuditProjectAcceptance(context.Background(), state)
		require.NoError(t, err)
		assert.False(t, res.Passed)
		assert.Equal(t, "Missing CLI flags", res.Summary)
		assert.Equal(t, []string{"--verbose flag missing"}, res.Gaps)
	})

	t.Run("fails audit when deprecated docker-compose or tooling gaps are reported", func(t *testing.T) {
		tmpDir := t.TempDir()
		require.NoError(t, os.WriteFile(filepath.Join(tmpDir, "SPEC.md"), []byte("# Spec\nDocker deployment required"), 0600))
		require.NoError(t, os.WriteFile(filepath.Join(tmpDir, "Makefile"), []byte("e2e:\n\tdocker-compose up\n"), 0600))

		mock := &mockAuditorLLM{
			response: &domain.LLMResponse{
				Actions: []domain.LLMAction{
					{
						Tool: "submit_acceptance_audit",
						Args: map[string]any{
							"passed":  false,
							"summary": "Deprecated tooling detected: Makefile uses legacy docker-compose instead of docker compose.",
							"gaps":    []any{"Makefile: contains deprecated 'docker-compose' invocation; must use 'docker compose'"},
						},
					},
				},
			},
		}

		auditor := NewAcceptanceAuditor(mock, nil)
		state := &domain.State{ProjectPath: tmpDir}
		res, err := auditor.AuditProjectAcceptance(context.Background(), state)
		require.NoError(t, err)
		assert.False(t, res.Passed)
		assert.Contains(t, res.Summary, "docker-compose")
		assert.Len(t, res.Gaps, 1)
		assert.Contains(t, res.Gaps[0], "docker-compose")
	})
}
