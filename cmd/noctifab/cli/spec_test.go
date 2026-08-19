package cli

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSpecCmd_Flags(t *testing.T) {
	assert.NotNil(t, specCmd)
	assert.Equal(t, "spec [path_or_prompt...]", specCmd.Use)

	outFlag := specCmd.Flag("output")
	assert.NotNil(t, outFlag)
	assert.Equal(t, "SPEC.md", outFlag.DefValue)

	nonIntFlag := specCmd.Flag("non-interactive")
	assert.NotNil(t, nonIntFlag)
	assert.Equal(t, "false", nonIntFlag.DefValue)

	consensusFlag := specCmd.Flag("consensus")
	assert.NotNil(t, consensusFlag)
	assert.Equal(t, "true", consensusFlag.DefValue)
}

func TestParseSpecArgs(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "parse-spec-args-*")
	require.NoError(t, err)
	defer func() { _ = os.RemoveAll(tempDir) }()

	// Case 1: Empty args
	dir, prompt := parseSpecArgs([]string{})
	assert.Equal(t, ".", dir)
	assert.Equal(t, "", prompt)

	// Case 2: Only prompt
	dir, prompt = parseSpecArgs([]string{"Build", "a", "KV", "store"})
	assert.Equal(t, ".", dir)
	assert.Equal(t, "Build a KV store", prompt)

	// Case 3: First arg is directory + prompt
	dir, prompt = parseSpecArgs([]string{tempDir, "Build", "a", "web", "server"})
	assert.Equal(t, tempDir, dir)
	assert.Equal(t, "Build a web server", prompt)
}

func TestLoadWorkspaceConfigOrDefault(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "spec-cli-test-*")
	require.NoError(t, err)
	defer func() { _ = os.RemoveAll(tempDir) }()

	// In empty directory, returns DefaultConfig
	cfg, err := loadWorkspaceConfigOrDefault(tempDir)
	require.NoError(t, err)
	assert.NotNil(t, cfg)
	assert.Equal(t, "SPEC.md", cfg.Spec.GetOutputFile())

	// In directory with .noctifab/config.yaml
	noctifabDir := filepath.Join(tempDir, ".noctifab")
	require.NoError(t, os.MkdirAll(noctifabDir, 0755))
	cfgContent := `config_version: "2.0"
spec:
  output_file: "CUSTOM_SPEC.md"
`
	require.NoError(t, os.WriteFile(filepath.Join(noctifabDir, "config.yaml"), []byte(cfgContent), 0644))

	cfg2, err2 := loadWorkspaceConfigOrDefault(tempDir)
	require.NoError(t, err2)
	assert.NotNil(t, cfg2)
	assert.Equal(t, "CUSTOM_SPEC.md", cfg2.Spec.GetOutputFile())
}
