package services

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/diegojromerolopez/noctifab/pkg/domain"
	"github.com/diegojromerolopez/noctifab/pkg/infrastructure/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type mockSovereignLLM struct {
	response *domain.LLMResponse
	err      error
}

func (m *mockSovereignLLM) Complete(ctx context.Context, prompt string) (*domain.LLMResponse, error) {
	return m.response, m.err
}

type mockSovereignSandbox struct {
	cmdRun string
	output string
	err    error
}

func (m *mockSovereignSandbox) RunCommand(ctx context.Context, projectPath, cmd, stdin string) (string, error) {
	m.cmdRun = cmd
	return m.output, m.err
}

func TestSovereignQAAuditor_DetectProbeCommand(t *testing.T) {
	tmpDir := t.TempDir()
	auditor := NewSovereignQAAuditor(nil)

	// No probe script initially
	assert.Empty(t, auditor.DetectProbeCommand(tmpDir))

	// Create tests/e2e/run_tests.sh
	e2eDir := filepath.Join(tmpDir, "tests", "e2e")
	require.NoError(t, os.MkdirAll(e2eDir, 0755))
	scriptPath := filepath.Join(e2eDir, "run_tests.sh")
	require.NoError(t, os.WriteFile(scriptPath, []byte("#!/bin/sh\nexit 0\n"), 0755))

	cmd := auditor.DetectProbeCommand(tmpDir)
	assert.Equal(t, "sh tests/e2e/run_tests.sh", cmd)
}

func TestSovereignQAAuditor_ImmediateFailure(t *testing.T) {
	tmpDir := t.TempDir()
	state := &domain.State{
		ProjectPath: tmpDir,
	}

	runner := &mockSovereignSandbox{
		output: "Connection reset by peer at test_connection:42",
		err:    assert.AnError,
	}
	auditor := NewSovereignQAAuditor(nil, runner)

	res, err := auditor.Audit(context.Background(), state, "python3 probe.py", runner.output, runner.err)
	require.NoError(t, err)
	assert.False(t, res.Passed)
	assert.NotEmpty(t, res.ImmediateErrors)
	assert.Contains(t, res.ImmediateErrors[0], "Connection reset by peer")
}

func TestSovereignQAAuditor_PredictiveFailureDetection(t *testing.T) {
	tmpDir := t.TempDir()
	srcDir := filepath.Join(tmpDir, "src")
	require.NoError(t, os.MkdirAll(srcDir, 0755))

	// Author code with latent buffer mutation bug (like the pyedis bytearray vs bytes bug)
	buggyCode := `
def handle_client(sock):
    buffer = bytearray()
    while True:
        chunk = sock.recv(1024)
        buffer.extend(chunk)
        cmd, remaining = decode_resp_stream(buffer)
        buffer = remaining  # BUG: remaining is bytes, so buffer.extend() fails on command 2!
`
	require.NoError(t, os.WriteFile(filepath.Join(srcDir, "main.py"), []byte(buggyCode), 0644))

	state := &domain.State{
		ProjectPath: tmpDir,
	}

	auditor := NewSovereignQAAuditor(nil)
	// Probe itself passed on first run!
	res, err := auditor.Audit(context.Background(), state, "pytest tests/", "1 passed in 0.05s", nil)
	require.NoError(t, err)

	// But predictive analysis flags the latent buffer mutation risk!
	assert.False(t, res.Passed, "should fail audit due to predicted critical buffer mutation risk")
	require.NotEmpty(t, res.PredictedFailures)
	foundBufferMutation := false
	for _, p := range res.PredictedFailures {
		if p.Category == "buffer_mutation" && p.RiskLevel == "critical" {
			foundBufferMutation = true
			assert.Contains(t, p.AffectedFile, "main.py")
			assert.NotEmpty(t, p.TriggerScenario)
			assert.NotEmpty(t, p.Mitigation)
		}
	}
	assert.True(t, foundBufferMutation, "expected critical buffer_mutation failure prediction")
}

func TestSovereignQAAuditor_LLMPredictionParsing(t *testing.T) {
	tmpDir := t.TempDir()
	state := &domain.State{
		ProjectPath: tmpDir,
	}

	mockLLM := &mockSovereignLLM{
		response: &domain.LLMResponse{
			Actions: []domain.LLMAction{
				{
					Tool: "submit_sovereign_qa",
					Args: map[string]any{
						"passed":           false,
						"summary":          "Interface probe passed, but code analysis predicts future connection drops and memory leaks under load.",
						"immediate_errors": []any{},
						"predicted_failures": []any{
							map[string]any{
								"risk_level":       "high",
								"category":         "connection_lifecycle",
								"description":      "Server socket does not set keepalive and closes connection on transient timeouts",
								"affected_file":    "src/server.py",
								"trigger_scenario": "Long-running client connections under intermittent network latency",
								"mitigation":       "Add SO_KEEPALIVE socket options and wrap read timeouts in reconnect guards",
							},
						},
						"actionable_requirements": []any{
							"Configure SO_KEEPALIVE on accepted client sockets in src/server.py",
						},
						"fixes": []any{
							map[string]any{
								"file":        "src/server.py",
								"action":      "modify",
								"description": "Add socket keepalive and robust reconnect handling",
							},
						},
					},
				},
			},
		},
	}

	auditor := NewSovereignQAAuditor(mockLLM)
	res, err := auditor.Audit(context.Background(), state, "sh run_tests.sh", "OK", nil)
	require.NoError(t, err)

	assert.False(t, res.Passed)
	assert.Equal(t, 1, len(res.PredictedFailures))
	assert.Equal(t, "connection_lifecycle", res.PredictedFailures[0].Category)
	assert.Equal(t, "high", res.PredictedFailures[0].RiskLevel)
	assert.Equal(t, "src/server.py", res.PredictedFailures[0].AffectedFile)
	assert.Equal(t, 1, len(res.Fixes))
	assert.Equal(t, "src/server.py", res.Fixes[0].File)
}

func TestSovereignQAAuditor_AcceptanceAuditorIntegration(t *testing.T) {
	tmpDir := t.TempDir()
	state := &domain.State{
		ProjectPath: tmpDir,
	}

	specPath := filepath.Join(tmpDir, "SPEC.md")
	require.NoError(t, os.WriteFile(specPath, []byte("# Test Spec\nFeatures here\n"), 0644))

	mockLLM := &mockSovereignLLM{
		response: &domain.LLMResponse{
			Actions: []domain.LLMAction{
				{
					Tool: "submit_acceptance_audit",
					Args: map[string]any{
						"passed":  true,
						"summary": "SPEC features look implemented",
						"gaps":    []any{},
					},
				},
			},
		},
	}

	acceptanceAuditor := NewAcceptanceAuditor(mockLLM, nil)
	sqaMockLLM := &mockSovereignLLM{
		response: &domain.LLMResponse{
			Actions: []domain.LLMAction{
				{
					Tool: "submit_sovereign_qa",
					Args: map[string]any{
						"passed":  false,
						"summary": "Empirical check passed, but predictive failure analysis found unclosed file handles",
						"predicted_failures": []any{
							map[string]any{
								"risk_level":       "critical",
								"category":         "resource_leak",
								"description":      "AOF log file handle is never closed on shutdown",
								"affected_file":    "src/storage.py",
								"trigger_scenario": "Repeated server restarts or long-running daemons exhaust file descriptors",
								"mitigation":       "Implement graceful shutdown closing aof file descriptor",
							},
						},
						"fixes": []any{
							map[string]any{
								"file":        "src/storage.py",
								"action":      "modify",
								"description": "Add close() method to AOF manager",
							},
						},
					},
				},
			},
		},
	}
	acceptanceAuditor.SetSovereignQA(NewSovereignQAAuditor(sqaMockLLM))

	res, err := acceptanceAuditor.AuditProjectAcceptance(context.Background(), state)
	require.NoError(t, err)

	// Must fail acceptance because Sovereign QA predicted a critical failure!
	assert.False(t, res.Passed)
	assert.NotEmpty(t, res.Gaps)
	assert.Contains(t, res.Gaps[0], "Predictive QA Failure Risk [critical]")
	assert.Equal(t, 1, len(res.Fixes))
	assert.Equal(t, "src/storage.py", res.Fixes[0].File)
	assert.Equal(t, 1, len(res.PredictedFailures))
}

func TestSovereignQAAuditor_RemediationTaskFormatting(t *testing.T) {
	state := &domain.State{
		ProjectPath: "/fake/repo",
		Metadata: domain.StateMetadata{
			FeatureName: "US-001",
		},
		Tasks: []domain.Task{
			{ID: "T-001", Status: domain.TaskSuccess},
		},
	}

	orch := &Orchestrator{
		acceptanceAuditor: NewAcceptanceAuditor(nil, nil),
	}

	auditResult := &AcceptanceAuditResult{
		Passed:  false,
		Summary: "Auditor rejected release due to predicted failure risks",
		Gaps:    []string{"Predictive QA Failure Risk [critical] (src/main.py): buffer mutation will crash"},
		PredictedFailures: []FailurePrediction{
			{
				RiskLevel:       "critical",
				Category:        "buffer_mutation",
				Description:     "buffer mutation will crash on subsequent commands",
				AffectedFile:    "src/main.py",
				TriggerScenario: "Client sends multiple commands",
				Mitigation:      "Wrap slice in bytearray()",
			},
		},
		Fixes: []ProposedFix{
			{
				File:        "src/main.py",
				Action:      "modify",
				Description: "Fix buffer bytearray wrap",
			},
		},
	}

	queued := orch.queueAcceptanceRemediationTask(context.Background(), state, auditResult)
	assert.True(t, queued)
	require.Equal(t, 2, len(state.Tasks))

	remediationTask := state.Tasks[1]
	assert.Equal(t, "spec-remediation-1", remediationTask.ID)
	assert.Contains(t, remediationTask.Description, "SOVEREIGN QA PREDICTED FUTURE FAILURE VECTORS")
	assert.Contains(t, remediationTask.Description, "Trigger Scenario: Client sends multiple commands")
	assert.Contains(t, remediationTask.Description, "Mitigate all predicted future failure vectors")
	assert.Contains(t, remediationTask.TargetFiles, "src/main.py")
}

func TestSovereignQAAuditor_ConfigureFromConfig(t *testing.T) {
	auditor := NewSovereignQAAuditor(nil)
	cfg := &config.Config{
		Sandbox: config.SandboxConfig{
			TestCommand:     "pytest",
			AllowedCommands: []string{"pytest", "sh"},
			E2E: config.E2EConfig{
				Mode:    "native",
				Command: "pytest tests/e2e",
			},
		},
	}
	auditor.ConfigureFromConfig(cfg)
	assert.Equal(t, "pytest tests/e2e", auditor.e2eCmd)
	assert.Equal(t, "native", auditor.e2eMode)
	assert.Equal(t, "pytest", auditor.defaultTestCmd)
	assert.Equal(t, []string{"pytest", "sh"}, auditor.allowedCommands)
}
