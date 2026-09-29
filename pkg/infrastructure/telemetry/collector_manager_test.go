package telemetry

import (
	"context"
	"net"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestIsPortReachable(t *testing.T) {
	// Start a dummy TCP server
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	defer func() { _ = ln.Close() }()

	addr := ln.Addr().String()

	// Should be reachable
	assert.True(t, IsPortReachable(addr, 500*time.Millisecond))
	assert.True(t, IsPortReachable("http://"+addr, 500*time.Millisecond))

	// Close listener
	_ = ln.Close()

	// Should not be reachable now
	assert.False(t, IsPortReachable(addr, 50*time.Millisecond))
}

func TestEnsureCollectorOnline_AlreadyReachable(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	defer func() { _ = ln.Close() }()

	addr := ln.Addr().String()
	res, err := EnsureCollectorOnline(context.Background(), addr)
	assert.NoError(t, err)
	assert.Equal(t, addr, res)
}

func TestEnsureCollectorOnline_UnreachableWithCancelledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately

	// Unused random local address that is unreachable
	_, err := EnsureCollectorOnline(ctx, "127.0.0.1:59999")
	// Should fail cleanly and not hang
	assert.Error(t, err)
}
