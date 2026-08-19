package storage

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

// SpecSnapshotManager manages immutable specification snapshots and diffs on disk.
type SpecSnapshotManager struct {
	baseDir string
}

// NewSpecSnapshotManager creates a new snapshot manager anchored at baseDir.
func NewSpecSnapshotManager(baseDir string) *SpecSnapshotManager {
	if baseDir == "" {
		baseDir = "."
	}
	return &SpecSnapshotManager{baseDir: baseDir}
}

// RevisionsDir returns the path to .noctifab/data/specs/revisions/
func (m *SpecSnapshotManager) RevisionsDir() string {
	return filepath.Join(m.baseDir, ".noctifab", "data", "specs", "revisions")
}

// DiffsDir returns the path to .noctifab/data/specs/diffs/
func (m *SpecSnapshotManager) DiffsDir() string {
	return filepath.Join(m.baseDir, ".noctifab", "data", "specs", "diffs")
}

// SaveSnapshot saves an immutable snapshot of a spec version and optional diff patch.
func (m *SpecSnapshotManager) SaveSnapshot(version int, content string, diff string) (string, string, error) {
	revDir := m.RevisionsDir()
	if err := os.MkdirAll(revDir, 0755); err != nil {
		return "", "", fmt.Errorf("failed to create revisions dir: %w", err)
	}

	snapshotFile := filepath.Join(revDir, fmt.Sprintf("SPEC.v%d.md", version))
	if err := os.WriteFile(snapshotFile, []byte(content), 0644); err != nil {
		return "", "", fmt.Errorf("failed to write snapshot %s: %w", snapshotFile, err)
	}

	hasher := sha256.New()
	hasher.Write([]byte(content))
	sha := hex.EncodeToString(hasher.Sum(nil))

	if diff != "" && version > 1 {
		diffsDir := m.DiffsDir()
		if err := os.MkdirAll(diffsDir, 0755); err == nil {
			patchFile := filepath.Join(diffsDir, fmt.Sprintf("diff_v%d_to_v%d.patch", version-1, version))
			_ = os.WriteFile(patchFile, []byte(diff), 0644)
		}
	}

	return snapshotFile, sha, nil
}

// LoadSnapshot reads a specific specification revision by version number.
func (m *SpecSnapshotManager) LoadSnapshot(version int) (string, error) {
	snapshotFile := filepath.Join(m.RevisionsDir(), fmt.Sprintf("SPEC.v%d.md", version))
	data, err := os.ReadFile(snapshotFile)
	if err != nil {
		return "", fmt.Errorf("failed to read spec revision v%d: %w", version, err)
	}
	return string(data), nil
}

// ListSnapshots returns all available snapshot version numbers sorted in ascending order.
func (m *SpecSnapshotManager) ListSnapshots() ([]int, error) {
	revDir := m.RevisionsDir()
	entries, err := os.ReadDir(revDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	var versions []int
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if strings.HasPrefix(name, "SPEC.v") && strings.HasSuffix(name, ".md") {
			numStr := strings.TrimSuffix(strings.TrimPrefix(name, "SPEC.v"), ".md")
			if v, err := strconv.Atoi(numStr); err == nil {
				versions = append(versions, v)
			}
		}
	}

	sort.Ints(versions)
	return versions, nil
}
