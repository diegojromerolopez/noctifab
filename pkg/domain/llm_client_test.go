package domain

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestStreamLivenessTracker(t *testing.T) {
	tracker := NewStreamLivenessTracker()
	require.NotNil(t, tracker)

	assert.Equal(t, 0, tracker.ChunkCount())
	assert.False(t, tracker.IsActive(time.Now().Add(-1*time.Second)))

	// Record chunk
	before := time.Now()
	tracker.RecordChunk()
	assert.Equal(t, 1, tracker.ChunkCount())
	assert.True(t, tracker.IsActive(before))
	assert.False(t, tracker.IsActive(time.Now().Add(10*time.Second)))

	// Context propagation
	ctx := context.Background()
	assert.Nil(t, StreamLivenessTrackerFromContext(ctx))

	ctxWithTracker := WithStreamLivenessTracker(ctx, tracker)
	retrieved := StreamLivenessTrackerFromContext(ctxWithTracker)
	require.NotNil(t, retrieved)
	assert.Equal(t, 1, retrieved.ChunkCount())

	// Nil safety
	var nilTracker *StreamLivenessTracker
	nilTracker.RecordChunk()
	assert.False(t, nilTracker.IsActive(before))
	assert.Equal(t, 0, nilTracker.ChunkCount())
}

func TestContextHelpers(t *testing.T) {
	ctx := context.Background()

	// Role
	assert.Equal(t, "", GetRoleFromContext(ctx))
	ctxWithRole := WithRoleContext(ctx, "generator")
	assert.Equal(t, "generator", GetRoleFromContext(ctxWithRole))

	// Also check string key fallback
	ctxWithStringRole := context.WithValue(ctx, "agent_role", "tester") //nolint:staticcheck
	assert.Equal(t, "tester", GetRoleFromContext(ctxWithStringRole))

	ctxWithStringRole2 := context.WithValue(ctx, "role", "spec") //nolint:staticcheck
	assert.Equal(t, "spec", GetRoleFromContext(ctxWithStringRole2))

	// UncompactableTail
	assert.Equal(t, 0, UncompactableTailLen(ctx))
	ctxWithTail := WithUncompactableTail(ctx, 42)
	assert.Equal(t, 42, UncompactableTailLen(ctxWithTail))

	// CacheablePrefixLen
	assert.Equal(t, 0, CacheablePrefixLen(ctx))
	ctxWithCache := WithCacheablePrefix(ctx, 100)
	assert.Equal(t, 100, CacheablePrefixLen(ctxWithCache))
}
