package services

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/diegojromerolopez/noctifab/pkg/domain"
	"github.com/diegojromerolopez/noctifab/pkg/infrastructure/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type mockSpikeLLMClient struct {
	completeFn func(ctx context.Context, prompt string) (*domain.LLMResponse, error)
}

func (m *mockSpikeLLMClient) Complete(ctx context.Context, prompt string) (*domain.LLMResponse, error) {
	if m.completeFn != nil {
		return m.completeFn(ctx, prompt)
	}
	return &domain.LLMResponse{}, nil
}

func TestExecuteSpike_Disabled(t *testing.T) {
	tempDir := t.TempDir()
	cfg := &config.Config{
		Agents: config.AgentsConfig{
			Spike: config.SpikeConfig{Enabled: false},
		},
	}
	mockLLM := &mockSpikeLLMClient{}
	executed, err := ExecuteSpike(context.Background(), tempDir, cfg, mockLLM, nil, nil)
	require.NoError(t, err)
	assert.False(t, executed)
}

func TestExecuteSpike_ExistingCodeSkips(t *testing.T) {
	tempDir := t.TempDir()
	// Create existing source code exceeding MinLegacyTotalLineCount (>= 50 lines)
	var codeLines []string
	codeLines = append(codeLines, "package main", "import \"fmt\"")
	for i := 1; i <= 50; i++ {
		codeLines = append(codeLines, fmt.Sprintf("func fn%d() { fmt.Println(%d) }", i, i))
	}
	require.NoError(t, os.WriteFile(filepath.Join(tempDir, "main.go"), []byte(strings.Join(codeLines, "\n")), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(tempDir, "SPEC.md"), []byte("# Spec"), 0644))

	cfg := &config.Config{
		Agents: config.AgentsConfig{
			Spike: config.SpikeConfig{Enabled: true},
		},
	}
	mockLLM := &mockSpikeLLMClient{
		completeFn: func(ctx context.Context, prompt string) (*domain.LLMResponse, error) {
			t.Fatal("LLM should not be called when code already exists")
			return nil, nil
		},
	}
	executed, err := ExecuteSpike(context.Background(), tempDir, cfg, mockLLM, nil, nil)
	require.NoError(t, err)
	assert.False(t, executed)
}

func TestExecuteSpike_GreenfieldSuccess(t *testing.T) {
	tempDir := t.TempDir()
	// Init git repo so commitSpikeToGit works
	initCmd := exec.Command("git", "init")
	initCmd.Dir = tempDir
	require.NoError(t, initCmd.Run())

	gitConfigName := exec.Command("git", "config", "user.name", "Tester")
	gitConfigName.Dir = tempDir
	_ = gitConfigName.Run()
	gitConfigEmail := exec.Command("git", "config", "user.email", "test@noctifab.local")
	gitConfigEmail.Dir = tempDir
	_ = gitConfigEmail.Run()

	require.NoError(t, os.WriteFile(filepath.Join(tempDir, "SPEC.md"), []byte("# Test Project Spec"), 0644))

	cfg := &config.Config{
		Agents: config.AgentsConfig{
			Spike: config.SpikeConfig{
				Enabled:        true,
				TimeoutSeconds: 30,
			},
		},
	}

	mockLLM := &mockSpikeLLMClient{
		completeFn: func(ctx context.Context, prompt string) (*domain.LLMResponse, error) {
			return &domain.LLMResponse{
				Actions: []domain.LLMAction{
					{
						Tool: "write_files",
						Args: map[string]any{
							"files": map[string]any{
								"main.c":   "int main() { return 0; }\n",
								"Makefile": "all:\n\tgcc main.c -o app\n",
							},
						},
					},
				},
			}, nil
		},
	}

	executed, err := ExecuteSpike(context.Background(), tempDir, cfg, mockLLM, nil, nil)
	require.NoError(t, err)
	assert.True(t, executed)

	// Verify files written
	assert.FileExists(t, filepath.Join(tempDir, "main.c"))
	assert.FileExists(t, filepath.Join(tempDir, "Makefile"))

	// Verify git commit was made
	logCmd := exec.Command("git", "log", "-n", "1", "--oneline")
	logCmd.Dir = tempDir
	out, err := logCmd.CombinedOutput()
	require.NoError(t, err)
	assert.Contains(t, string(out), "initial spike solution walking skeleton")
}
