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

	t.Run("detects E2E command across Makefile and docker compose manifests", func(t *testing.T) {
		tmpDir := t.TempDir()
		auditor := NewAcceptanceAuditor(nil, nil)

		assert.Empty(t, auditor.detectE2ECommand(tmpDir))

		// Makefile with e2e: target
		require.NoError(t, os.WriteFile(filepath.Join(tmpDir, "Makefile"), []byte("e2e:\n\tpytest tests/e2e\n"), 0600))
		assert.Equal(t, "make e2e", auditor.detectE2ECommand(tmpDir))

		// docker-compose.yml with e2e service
		require.NoError(t, os.WriteFile(filepath.Join(tmpDir, "docker-compose.yml"), []byte("services:\n  e2e:\n    image: test\n"), 0600))
		assert.Equal(t, "docker compose up --build --exit-code-from e2e", auditor.detectE2ECommand(tmpDir))

		// docker-compose.e2e.yml takes precedence
		require.NoError(t, os.WriteFile(filepath.Join(tmpDir, "docker-compose.e2e.yml"), []byte("services:\n  test-runner:\n    image: test\n"), 0600))
		assert.Equal(t, "docker compose -f docker-compose.e2e.yml up --build --exit-code-from test-runner", auditor.detectE2ECommand(tmpDir))
	})

	t.Run("collectWorkspaceSnapshot includes test files and key domain modules", func(t *testing.T) {
		tmpDir := t.TempDir()
		testsDir := filepath.Join(tmpDir, "tests", "unit")
		require.NoError(t, os.MkdirAll(testsDir, 0755))
		require.NoError(t, os.WriteFile(filepath.Join(testsDir, "test_store.py"), []byte("def test_ping(): pass"), 0600))
		require.NoError(t, os.WriteFile(filepath.Join(tmpDir, "store.py"), []byte("class Store: pass"), 0600))

		auditor := NewAcceptanceAuditor(nil, nil)
		snapshot := auditor.collectWorkspaceSnapshot(context.Background(), tmpDir)
		assert.Contains(t, snapshot, "test_store.py")
		assert.Contains(t, snapshot, "def test_ping")
		assert.Contains(t, snapshot, "store.py")
	})

	t.Run("shouldRemediateAcceptanceAudit respects max retry threshold", func(t *testing.T) {
		mock := &mockAuditorLLM{}
		auditor := NewAcceptanceAuditor(mock, nil)
		orch := &Orchestrator{acceptanceAuditor: auditor}

		state := &domain.State{
			Tasks: []domain.Task{
				{ID: "task-1", Status: domain.TaskSuccess},
			},
		}
		assert.True(t, orch.shouldRemediateAcceptanceAudit(state))

		// 1 remediation task
		state.Tasks = append(state.Tasks, domain.Task{ID: "spec-remediation-1", Status: domain.TaskSuccess})
		assert.True(t, orch.shouldRemediateAcceptanceAudit(state))

		// 2 remediation tasks - exhausted
		state.Tasks = append(state.Tasks, domain.Task{ID: "spec-remediation-2", Status: domain.TaskSuccess})
		assert.False(t, orch.shouldRemediateAcceptanceAudit(state))
	})

	t.Run("queueAcceptanceRemediationTask creates structured remediation task", func(t *testing.T) {
		mock := &mockAuditorLLM{}
		auditor := NewAcceptanceAuditor(mock, nil)
		orch := &Orchestrator{acceptanceAuditor: auditor}

		state := &domain.State{
			Metadata: domain.StateMetadata{FeatureName: "US-FINAL"},
			Tasks: []domain.Task{
				{ID: "US-001-TASK-001", Status: domain.TaskSuccess},
			},
		}

		auditResult := &AcceptanceAuditResult{
			Passed:  false,
			Summary: "Missing PING, SET, GET commands and black-box E2E test harness",
			Gaps: []string{
				"Missing protocol command PING",
				"Missing protocol command SET",
				"Missing E2E test harness tests/e2e/run_tests.sh",
			},
			Fixes: []ProposedFix{
				{
					File:        "src/commands/ping.py",
					Action:      "create",
					Description: "Implement PingCommand with PONG response",
				},
				{
					File:        "src/server.py",
					Action:      "wire_into_dispatcher",
					Description: "Register PingCommand in the command router",
				},
			},
		}

		queued := orch.queueAcceptanceRemediationTask(context.Background(), state, auditResult)
		assert.True(t, queued)
		require.Len(t, state.Tasks, 2)

		task := state.Tasks[1]
		assert.Equal(t, "spec-remediation-1", task.ID)
		assert.Contains(t, task.Title, "Specification & Contract Remediation")
		assert.Contains(t, task.Description, "Missing protocol command PING")
		assert.Contains(t, task.Description, "Missing protocol command SET")
		assert.Contains(t, task.Description, "AUDITOR PROPOSED FIXES & REMEDIATION BLUEPRINT")
		assert.Contains(t, task.Description, "src/commands/ping.py")
		assert.Contains(t, task.Description, "wire_into_dispatcher")
		assert.Equal(t, domain.TaskPending, task.Status)
		assert.Equal(t, []string{"US-001-TASK-001"}, task.DependsOn)
	})

	t.Run("parseAuditResponse extracts proposed fixes correctly", func(t *testing.T) {
		auditor := NewAcceptanceAuditor(nil, nil)
		resp := &domain.LLMResponse{
			Actions: []domain.LLMAction{
				{
					Tool: "submit_acceptance_audit",
					Args: map[string]any{
						"passed":  false,
						"summary": "Missing dispatch for key commands",
						"gaps":    []any{"Missing DEL command in server dispatcher"},
						"fixes": []any{
							map[string]any{
								"file":        "src/server.py",
								"action":      "wire_into_dispatcher",
								"description": "Register DelCommand in RESP dispatch loop",
							},
						},
					},
				},
			},
		}

		res := auditor.parseAuditResponse(resp)
		assert.False(t, res.Passed)
		assert.Len(t, res.Gaps, 1)
		require.Len(t, res.Fixes, 1)
		assert.Equal(t, "src/server.py", res.Fixes[0].File)
		assert.Equal(t, "wire_into_dispatcher", res.Fixes[0].Action)
		assert.Equal(t, "Register DelCommand in RESP dispatch loop", res.Fixes[0].Description)
	})
}
