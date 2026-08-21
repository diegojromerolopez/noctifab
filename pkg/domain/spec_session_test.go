package domain

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSpecSession_Revisions(t *testing.T) {
	session := &SpecSession{
		ID:          "spec-123",
		ProjectPath: "/test/project",
		TargetFile:  "/test/project/SPEC.md",
		CreatedAt:   time.Now().UTC(),
	}

	assert.Nil(t, session.LatestRevision())
	assert.Nil(t, session.ActiveRevision())
	assert.False(t, session.CanUndo())
	assert.False(t, session.CanRedo())
	assert.Equal(t, int64(0), session.TotalTokensUsed())

	rev1 := session.AddRevision("# SPEC v1", "Create a CLI tool", SpecTurnInitial, "", 100)
	assert.Equal(t, 1, rev1.Version)
	assert.NotEmpty(t, rev1.SHA256)
	assert.Equal(t, 0, rev1.ParentVer)
	assert.Equal(t, "# SPEC v1", session.CurrentSpec)
	assert.Equal(t, 1, len(session.Revisions))
	assert.Equal(t, &rev1, session.LatestRevision())
	assert.Equal(t, &rev1, session.ActiveRevision())
	assert.Equal(t, int64(100), session.TotalTokensUsed())
	assert.False(t, session.CanUndo())
	assert.False(t, session.CanRedo())

	rev2 := session.AddRevision("# SPEC v2", "Add TLS support", SpecTurnRefine, "+ TLS", 150)
	assert.Equal(t, 2, rev2.Version)
	assert.NotEmpty(t, rev2.SHA256)
	assert.Equal(t, 1, rev2.ParentVer)
	assert.Equal(t, "# SPEC v2", session.CurrentSpec)
	assert.Equal(t, 2, len(session.Revisions))
	assert.Equal(t, &rev2, session.LatestRevision())
	assert.Equal(t, &rev2, session.ActiveRevision())
	assert.Equal(t, int64(250), session.TotalTokensUsed())
	assert.True(t, session.CanUndo())
	assert.False(t, session.CanRedo())

	rev3 := session.AddRevision("# SPEC v3", "Add gRPC", SpecTurnRefine, "+ gRPC", 200)
	assert.Equal(t, 3, rev3.Version)
	assert.Equal(t, 3, len(session.Revisions))
	assert.True(t, session.CanUndo())
	assert.False(t, session.CanRedo())

	// Test Undo
	undone, err := session.Undo()
	require.NoError(t, err)
	assert.Equal(t, 2, undone.Version)
	assert.Equal(t, "# SPEC v2", session.CurrentSpec)
	assert.True(t, session.CanUndo())
	assert.True(t, session.CanRedo())

	// Undo again to v1
	undone1, err := session.Undo()
	require.NoError(t, err)
	assert.Equal(t, 1, undone1.Version)
	assert.Equal(t, "# SPEC v1", session.CurrentSpec)
	assert.False(t, session.CanUndo())
	assert.True(t, session.CanRedo())

	// Undo past beginning fails
	_, err = session.Undo()
	assert.Error(t, err)

	// Test Redo to v2
	redone, err := session.Redo()
	require.NoError(t, err)
	assert.Equal(t, 2, redone.Version)
	assert.Equal(t, "# SPEC v2", session.CurrentSpec)

	// Test Checkout directly to v3
	checkedOut, err := session.Checkout(3)
	require.NoError(t, err)
	assert.Equal(t, 3, checkedOut.Version)
	assert.Equal(t, "# SPEC v3", session.CurrentSpec)

	// Checkout out of range fails
	_, err = session.Checkout(0)
	assert.Error(t, err)
	_, err = session.Checkout(99)
	assert.Error(t, err)

	var nilSession *SpecSession
	assert.Nil(t, nilSession.LatestRevision())
	assert.Nil(t, nilSession.ActiveRevision())
	assert.False(t, nilSession.CanUndo())
	assert.False(t, nilSession.CanRedo())
	assert.Equal(t, int64(0), nilSession.TotalTokensUsed())
	_, err = nilSession.Checkout(1)
	assert.Error(t, err)
}
