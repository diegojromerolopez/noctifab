package services

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"
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

	if !strings.Contains(string(content), "| Command |") && !strings.Contains(string(content), "| :---") {
		t.Skip("pyedis/SPEC.md does not contain markdown command tables")
	}

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

func TestPartitionSpecWithCompaction_AllChunksCompacted(t *testing.T) {
	tempDir := t.TempDir()
	outDir := filepath.Join(tempDir, ".noctifab", "specs")

	sampleSpec := `# Sample Specification
<!-- Some internal author comment that should be stripped -->

## 1. Overview & Architecture
Please note that in order to establish a clean foundation, the system is configured to operate in memory.
---

## 2. Directory Layout
- src/main.py
- src/commands.py

## 6. Supported Commands & Parity Semantics

### 6.1 String Commands
<!-- Another comment inside slice -->
It should be noted that strings are binary-safe buffers.
| Command | Signature | Success | Error |
| :--- | :--- | :--- | :--- |
| ` + "`GET`" + ` | GET key | $len\r\nval\r\n | -ERR |
| ` + "`SET`" + ` | SET key val | +OK\r\n | -ERR |
`

	hash := sha256.Sum256([]byte(sampleSpec))
	specHash := hex.EncodeToString(hash[:])

	// 1. Partition with caveman compaction
	manifest, err := PartitionSpecWithCompaction(sampleSpec, outDir, specHash, "caveman")
	require.NoError(t, err)
	require.NotNil(t, manifest)
	assert.Equal(t, "caveman", manifest.CompactionMode)

	// Verify domain slice 01_string.md is compacted (no HTML comments)
	stringSliceBytes, err := os.ReadFile(filepath.Join(outDir, "01_string.md"))
	require.NoError(t, err)
	stringSlice := string(stringSliceBytes)
	assert.NotContains(t, stringSlice, "<!-- Another comment inside slice -->")
	assert.Contains(t, stringSlice, "| `GET` | GET key |")

	// Verify 00_core_invariants.md is compacted (no HTML comments, no divider lines ---)
	coreBytes, err := os.ReadFile(filepath.Join(outDir, "00_core_invariants.md"))
	require.NoError(t, err)
	coreStr := string(coreBytes)
	assert.NotContains(t, coreStr, "<!-- Some internal author comment")
	assert.NotContains(t, coreStr, "\n---\n")

	// 2. Cache invalidation on compaction mode change
	projDir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(projDir, "SPEC.md"), []byte(sampleSpec), 0644))

	mNone, err := PartitionSpecIfNeededWithCompaction(projDir, "none")
	require.NoError(t, err)
	assert.Equal(t, "none", mNone.CompactionMode)

	// Changing to simple_english should trigger re-partitioning
	mSimple, err := PartitionSpecIfNeededWithCompaction(projDir, "simple_english")
	require.NoError(t, err)
	assert.Equal(t, "simple_english", mSimple.CompactionMode)

	// Changing to aggressive should trigger re-partitioning
	mAggressive, err := PartitionSpecIfNeededWithCompaction(projDir, "aggressive")
	require.NoError(t, err)
	assert.Equal(t, "aggressive", mAggressive.CompactionMode)
}

func TestPartitionSpecWithCompaction_MultipleSlicesAllCompacted(t *testing.T) {
	specWithComments := `# Comprehensive Spec
<!-- Global header comment -->

## 1. Overview
Please note that in order to establish a durable store, all state persists to disk.
---

## 2. Directory Layout
- src/main.py

## 6. Supported Commands & Parity Semantics

### 6.1 String Commands
<!-- Slice 1 comment -->
Strings are simple key-value entries.
| Command | Signature |
| :--- | :--- |
| ` + "`GET`" + ` | GET k |

### 6.2 List Commands
<!-- Slice 2 comment -->
Lists are ordered sequences of strings.
| Command | Signature |
| :--- | :--- |
| ` + "`LPUSH`" + ` | LPUSH k v |

### 6.3 Hash Commands
<!-- Slice 3 comment -->
Hashes represent field-value maps.
| Command | Signature |
| :--- | :--- |
| ` + "`HSET`" + ` | HSET k f v |
`
	hash := sha256.Sum256([]byte(specWithComments))
	specHash := hex.EncodeToString(hash[:])

	strategies := []struct {
		name string
		mode string
	}{
		{name: "Caveman", mode: "caveman"},
		{name: "SimpleEnglish", mode: "simple_english"},
		{name: "Aggressive", mode: "aggressive"},
	}

	for _, tt := range strategies {
		t.Run(tt.name, func(t *testing.T) {
			outDir := filepath.Join(t.TempDir(), ".noctifab", "specs")
			manifest, err := PartitionSpecWithCompaction(specWithComments, outDir, specHash, tt.mode)
			require.NoError(t, err)
			require.NotNil(t, manifest)
			assert.Len(t, manifest.Sections, 3)

			// Verify 00_core_invariants.md is compacted
			coreBytes, err := os.ReadFile(filepath.Join(outDir, "00_core_invariants.md"))
			require.NoError(t, err)
			assert.NotContains(t, string(coreBytes), "<!-- Global header comment -->")
			if tt.mode == "caveman" || tt.mode == "aggressive" {
				assert.NotContains(t, string(coreBytes), "\n---\n")
			}

			// Verify ALL domain slice files are compacted
			expectedFiles := []string{"01_string.md", "02_list.md", "03_hash.md"}
			for idx, fname := range expectedFiles {
				slicePath := filepath.Join(outDir, fname)
				require.FileExists(t, slicePath)
				contentBytes, err := os.ReadFile(slicePath)
				require.NoError(t, err)
				contentStr := string(contentBytes)

				comment := fmt.Sprintf("<!-- Slice %d comment -->", idx+1)
				assert.NotContains(t, contentStr, comment, "expected slice %s to have comments stripped", fname)
				assert.NotEmpty(t, contentStr)
			}
		})
	}
}

func TestBuildBasicAndFeatureSpec_And_RoadmapOutlineSpec(t *testing.T) {
	tempDir := t.TempDir()
	outDir := filepath.Join(tempDir, ".noctifab", "specs")

	sampleSpec := `# Project Spec
## 1. Core Architecture
Global server and protocol invariants.

## 2. String Commands
| Command | Signature |
| :--- | :--- |
| ` + "`GET`" + ` | GET key |
| ` + "`SET`" + ` | SET key val |

## 3. List Commands
| Command | Signature |
| :--- | :--- |
| ` + "`LPUSH`" + ` | LPUSH key val |
| ` + "`LPOP`" + ` | LPOP key |
`
	specHash := "testhash123"
	manifest, err := PartitionSpec(sampleSpec, outDir, specHash)
	require.NoError(t, err)
	require.NotNil(t, manifest)
	assert.Len(t, manifest.Sections, 2)

	// 1. Test BuildRoadmapOutlineSpec
	outlineSpec := BuildRoadmapOutlineSpec(tempDir, "fallback")
	assert.Contains(t, outlineSpec, "Global server and protocol invariants")
	assert.Contains(t, outlineSpec, "Specification Domain Subsystems Index")
	assert.Contains(t, outlineSpec, "String Commands")
	assert.Contains(t, outlineSpec, "List Commands")
	assert.NotContains(t, outlineSpec, "fallback")

	// 2. Test BuildBasicAndFeatureSpec matching by ID/Title
	listSpec := BuildBasicAndFeatureSpec(tempDir, "List Commands", "fallback")
	assert.Contains(t, listSpec, "Basic System Invariants & Core Architecture")
	assert.Contains(t, listSpec, "Global server and protocol invariants")
	assert.Contains(t, listSpec, "Target Feature Specification:")
	assert.Contains(t, listSpec, "List Commands")
	assert.Contains(t, listSpec, "LPUSH")
	assert.NotContains(t, listSpec, "fallback")

	// 3. Test BuildBasicAndFeatureSpec matching by command keyword
	getSpec := BuildBasicAndFeatureSpec(tempDir, "Implement GET and SET", "fallback")
	assert.Contains(t, getSpec, "Basic System Invariants & Core Architecture")
	assert.Contains(t, getSpec, "Target Feature Specification:")
	assert.Contains(t, getSpec, "String Commands")
	assert.Contains(t, getSpec, "GET")

	// 4. Test fallback when manifest doesn't exist
	emptyDir := t.TempDir()
	fbSpec := BuildBasicAndFeatureSpec(emptyDir, "Strings", "my-fallback")
	assert.Equal(t, "my-fallback", fbSpec)

	fbOutline := BuildRoadmapOutlineSpec(emptyDir, "my-fallback-outline")
	assert.Equal(t, "my-fallback-outline", fbOutline)
}

func TestPartitionSpec_AuxiliarySectionsExcludedFromCore(t *testing.T) {
	tempDir := t.TempDir()
	outDir := filepath.Join(tempDir, ".noctifab", "specs")

	sampleSpec := `# Large Architecture Spec

## 1. Core Overview
Core system architecture and operational contracts.

## 2. Command Family
Command details and wire formats.

## 7. Testing Strategy and Verification Matrix
Extensive test matrices and test case inventories that should not bloat core invariants.
- Test case 1
- Test case 2

## 8. Documentation Guidelines
User manuals and doc instructions.
`
	manifest, err := PartitionSpec(sampleSpec, outDir, "hash999")
	require.NoError(t, err)
	require.NotNil(t, manifest)

	// Verify 00_core_invariants.md does not contain auxiliary section bodies
	coreBytes, err := os.ReadFile(filepath.Join(outDir, "00_core_invariants.md"))
	require.NoError(t, err)
	coreStr := string(coreBytes)
	assert.Contains(t, coreStr, "Core system architecture")
	assert.NotContains(t, coreStr, "Extensive test matrices and test case inventories")
	assert.NotContains(t, coreStr, "User manuals and doc instructions")

	// Verify auxiliary files were written
	auxTestBytes, err := os.ReadFile(filepath.Join(outDir, "aux_7_testing_strategy_and_verification_matrix.md"))
	require.NoError(t, err)
	assert.Contains(t, string(auxTestBytes), "Extensive test matrices and test case inventories")

	auxDocBytes, err := os.ReadFile(filepath.Join(outDir, "aux_8_documentation_guidelines.md"))
	require.NoError(t, err)
	assert.Contains(t, string(auxDocBytes), "User manuals and doc instructions")
}
