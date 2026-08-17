package browser

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestOSBrowserOpener_DarwinLaunch(t *testing.T) {
	var capturedCmd string
	var capturedArgs []string

	mockExec := func(ctx context.Context, name string, args ...string) error {
		capturedCmd = name
		capturedArgs = args
		return nil
	}

	opener := &OSBrowserOpener{
		ExecCommand: mockExec,
	}

	err := opener.Open(context.Background(), "http://127.0.0.1:8080")
	require.NoError(t, err)
	assert.NotEmpty(t, capturedCmd)
	assert.Contains(t, capturedArgs, "http://127.0.0.1:8080")
}

func TestOSBrowserOpener_FailureHandling(t *testing.T) {
	mockExec := func(ctx context.Context, name string, args ...string) error {
		return errors.New("failed to launch browser process")
	}

	opener := &OSBrowserOpener{
		ExecCommand: mockExec,
	}

	err := opener.Open(context.Background(), "http://127.0.0.1:8080")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to launch browser process")
}

func TestNoopOpener(t *testing.T) {
	n := &NoopOpener{}
	err := n.Open(context.Background(), "http://127.0.0.1:8080")
	assert.NoError(t, err)
}
