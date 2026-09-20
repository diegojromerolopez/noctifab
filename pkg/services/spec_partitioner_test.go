package services

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPartitionSpec_EndToEnd(t *testing.T) {
	tempDir := t.TempDir()
	outDir := filepath.Join(tempDir, ".noctifab", "specs")

	sampleSpec := `# Sample Project Specification

## 1. Overview & Architecture
This is the core architecture overview.

## 2. Directory Layout
- src/main.py
- src/commands/

## 3. Public API Signatures
Core signatures and interfaces.

## 4. Toolchain & Environment
Python 3.14, standard library unittest.

## 5. Wire Protocol
RESP protocol definitions.

## 6. Supported Commands & Parity Semantics

### 6.1 String Commands
| Command | Signature | Success | Error |
| :--- | :--- | :--- | :--- |
| ` + "`GET`" + ` | GET key | $len\r\nval\r\n | -ERR |
| ` + "`SET`" + ` | SET key val | +OK\r\n | -ERR |
| ` + "`INCR`" + ` | INCR key | :val\r\n | -ERR |

### 6.2 List Commands
| Command | Signature | Success | Error |
| :--- | :--- | :--- | :--- |
| ` + "`LPUSH`" + ` | LPUSH key val | :count\r\n | -ERR |
| ` + "`LPOP`" + ` | LPOP key | $len\r\nval\r\n | -ERR |

## 7. Storage Semantics & Expiration
In-memory dict with TTL.

## 8. Persistence Architecture
AOF persistence engine.

## 9. Definition of Done
All tests pass.
`

	hash := sha256.Sum256([]byte(sampleSpec))
	specHash := hex.EncodeToString(hash[:])

	manifest, err := PartitionSpec(sampleSpec, outDir, specHash)
	require.NoError(t, err)
	require.NotNil(t, manifest)

	assert.Equal(t, specHash, manifest.SpecSHA256)
	assert.Equal(t, "00_core_invariants.md", manifest.CoreFile)
	assert.Len(t, manifest.Sections, 2)

	// Verify Section 1: Strings
	sec1 := manifest.Sections[0]
	assert.Equal(t, "string", sec1.ID)
	assert.Equal(t, "6.1 String Commands", sec1.Title)
	assert.Equal(t, 3, sec1.CommandCount)
	assert.Equal(t, []string{"GET", "SET", "INCR"}, sec1.Commands)
	assert.Equal(t, "src/commands/strings.py", sec1.SuggestedTarget)

	// Verify Section 2: Lists
	sec2 := manifest.Sections[1]
	assert.Equal(t, "list", sec2.ID)
	assert.Equal(t, "6.2 List Commands", sec2.Title)
	assert.Equal(t, 2, sec2.CommandCount)
	assert.Equal(t, []string{"LPUSH", "LPOP"}, sec2.Commands)
	assert.Equal(t, "src/commands/lists.py", sec2.SuggestedTarget)

	// Check files on disk
	coreContent, err := os.ReadFile(filepath.Join(outDir, "00_core_invariants.md"))
	require.NoError(t, err)
	assert.Contains(t, string(coreContent), "## 1. Overview & Architecture")
	assert.Contains(t, string(coreContent), "## 2. Directory Layout")
	assert.Contains(t, string(coreContent), "Detailed command matrix partitioned into `.noctifab/specs/01_string.md`")

	stringSlice, err := os.ReadFile(filepath.Join(outDir, "01_string.md"))
	require.NoError(t, err)
	assert.Contains(t, string(stringSlice), "| `GET` | GET key |")
	assert.Contains(t, string(stringSlice), "| `SET` | SET key val |")

	listSlice, err := os.ReadFile(filepath.Join(outDir, "02_list.md"))
	require.NoError(t, err)
	assert.Contains(t, string(listSlice), "| `LPUSH` | LPUSH key val |")

	manifestDisk, err := os.ReadFile(filepath.Join(outDir, "manifest.json"))
	require.NoError(t, err)
	assert.Contains(t, string(manifestDisk), `"spec_sha256": "`+specHash+`"`)
}

func TestPartitionSpecIfNeeded_CachingAndMissing(t *testing.T) {
	tempDir := t.TempDir()

	// Case 1: SPEC.md does not exist -> returns nil, nil
	m, err := PartitionSpecIfNeeded(tempDir)
	require.NoError(t, err)
	assert.Nil(t, m)

	// Case 2: Create SPEC.md -> partitions and writes manifest
	specFile := filepath.Join(tempDir, "SPEC.md")
	content := "# Minimal Spec\n## 1. Overview\nOverview text\n"
	require.NoError(t, os.WriteFile(specFile, []byte(content), 0644))

	m1, err := PartitionSpecIfNeeded(tempDir)
	require.NoError(t, err)
	require.NotNil(t, m1)
	assert.FileExists(t, filepath.Join(tempDir, ".noctifab", "specs", "manifest.json"))

	// Case 3: Call again without modifying SPEC.md -> cache hit
	m2, err := PartitionSpecIfNeeded(tempDir)
	require.NoError(t, err)
	require.NotNil(t, m2)
	assert.Equal(t, m1.SpecSHA256, m2.SpecSHA256)
}

func TestPartitionSpec_RealPyedisSpec(t *testing.T) {
	pyedisSpec := "/Users/diegoj/repos/pyedis/SPEC.md"
	if _, err := os.Stat(pyedisSpec); err != nil {
		t.Skip("pyedis/SPEC.md not available on host")
	}

	content, err := os.ReadFile(pyedisSpec)
	require.NoError(t, err)

	tempDir := t.TempDir()
	outDir := filepath.Join(tempDir, ".noctifab", "specs")

	hash := sha256.Sum256(content)
	specHash := hex.EncodeToString(hash[:])

	manifest, err := PartitionSpec(string(content), outDir, specHash)
	require.NoError(t, err)
	require.NotNil(t, manifest)

	// In pyedis, there are 14+ command categories in Section 6
	assert.GreaterOrEqual(t, len(manifest.Sections), 14)

	// Sum total commands extracted from tables
	totalCommands := 0
	for _, sec := range manifest.Sections {
		totalCommands += sec.CommandCount
	}
	assert.GreaterOrEqual(t, totalCommands, 200)

	// Verify core invariants file was generated and is compact
	coreBytes, err := os.ReadFile(filepath.Join(outDir, manifest.CoreFile))
	require.NoError(t, err)
	t.Logf("Pyedis partitioned into %d sections with %d total commands across tables (Core file: %d bytes)", len(manifest.Sections), totalCommands, len(coreBytes))
	assert.Less(t, len(coreBytes), 50000, "Core file should be substantially compacted compared to original 88KB")
	assert.Contains(t, string(coreBytes), "Detailed command matrix partitioned into")
}
