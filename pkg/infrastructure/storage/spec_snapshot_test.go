package storage

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSpecSnapshotManager(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "spec-snapshot-test-*")
	require.NoError(t, err)
	defer func() { _ = os.RemoveAll(tempDir) }()

	mgr := NewSpecSnapshotManager(tempDir)

	// Save revision 1
	path1, sha1, err := mgr.SaveSnapshot(1, "# Version 1 Spec Content", "")
	require.NoError(t, err)
	assert.NotEmpty(t, path1)
	assert.NotEmpty(t, sha1)

	// Verify file exists
	data1, err := os.ReadFile(path1)
	require.NoError(t, err)
	assert.Equal(t, "# Version 1 Spec Content", string(data1))

	// Save revision 2 with diff
	path2, sha2, err := mgr.SaveSnapshot(2, "# Version 2 Spec Content with TLS", "+ TLS")
	require.NoError(t, err)
	assert.NotEmpty(t, path2)
	assert.NotEmpty(t, sha2)
	assert.NotEqual(t, sha1, sha2)

	// Load by version
	loaded1, err := mgr.LoadSnapshot(1)
	require.NoError(t, err)
	assert.Equal(t, "# Version 1 Spec Content", loaded1)

	loaded2, err := mgr.LoadSnapshot(2)
	require.NoError(t, err)
	assert.Equal(t, "# Version 2 Spec Content with TLS", loaded2)

	// Load non-existent version fails
	_, err = mgr.LoadSnapshot(99)
	assert.Error(t, err)

	// List snapshots
	versions, err := mgr.ListSnapshots()
	require.NoError(t, err)
	assert.Equal(t, []int{1, 2}, versions)

	// Verify diff patch file exists
	diffPath := filepath.Join(mgr.DiffsDir(), "diff_v1_to_v2.patch")
	diffData, err := os.ReadFile(diffPath)
	require.NoError(t, err)
	assert.Equal(t, "+ TLS", string(diffData))
}

func TestSpecSnapshotManager_EmptyDir(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "spec-snapshot-empty-*")
	require.NoError(t, err)
	defer func() { _ = os.RemoveAll(tempDir) }()

	mgr := NewSpecSnapshotManager(tempDir)
	versions, err := mgr.ListSnapshots()
	require.NoError(t, err)
	assert.Empty(t, versions)
}
