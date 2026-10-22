package services_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/diegojromerolopez/noctifab/pkg/services"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestIsAutoResolvableConflictFile(t *testing.T) {
	assert.True(t, services.IsAutoResolvableConflictFile("Cargo.toml"))
	assert.True(t, services.IsAutoResolvableConflictFile("src/lib.rs"))
	assert.True(t, services.IsAutoResolvableConflictFile("package.json"))
	assert.True(t, services.IsAutoResolvableConflictFile("src/Dotchess/Dotchess.csproj"))
	assert.False(t, services.IsAutoResolvableConflictFile("src/main.c"))
}

func TestAutoResolveConflictFile_CargoToml(t *testing.T) {
	tempDir := t.TempDir()
	conflictedContent := `[package]
name = "myproject"
version = "0.1.0"

[dependencies]
<<<<<<< HEAD
serde = "1.0"
tokio = "1.0"
=======
serde = "1.0"
reqwest = "0.11"
>>>>>>> feat/other
`
	filePath := filepath.Join(tempDir, "Cargo.toml")
	err := os.WriteFile(filePath, []byte(conflictedContent), 0644)
	require.NoError(t, err)

	resolved, err := services.AutoResolveConflictFile(filePath)
	require.NoError(t, err)
	assert.True(t, resolved)

	content, err := os.ReadFile(filePath)
	require.NoError(t, err)
	merged := string(content)

	assert.NotContains(t, merged, "<<<<<<<")
	assert.NotContains(t, merged, "=======")
	assert.NotContains(t, merged, ">>>>>>>")
	assert.Contains(t, merged, "serde = \"1.0\"")
	assert.Contains(t, merged, "tokio = \"1.0\"")
	assert.Contains(t, merged, "reqwest = \"0.11\"")
}

func TestAutoResolveConflictFile_LibRs(t *testing.T) {
	tempDir := t.TempDir()
	conflictedContent := `//! Crate root
<<<<<<< HEAD
pub mod parser;
pub mod ast;
=======
pub mod parser;
pub mod safety;
>>>>>>> feat/task-2
`
	filePath := filepath.Join(tempDir, "lib.rs")
	err := os.WriteFile(filePath, []byte(conflictedContent), 0644)
	require.NoError(t, err)

	resolved, err := services.AutoResolveConflictFile(filePath)
	require.NoError(t, err)
	assert.True(t, resolved)

	content, err := os.ReadFile(filePath)
	require.NoError(t, err)
	merged := string(content)

	assert.NotContains(t, merged, "<<<<<<<")
	assert.Contains(t, merged, "pub mod parser;")
	assert.Contains(t, merged, "pub mod ast;")
	assert.Contains(t, merged, "pub mod safety;")
}
