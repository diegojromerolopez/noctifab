package notifier

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type errorNotifier struct {
	err error
}

func (e *errorNotifier) Notify(ctx context.Context, kind NotificationKind, title, message string) error {
	return e.err
}

func TestMultiNotifier_DispatchesToAll(t *testing.T) {
	mock1 := NewMockNotifier()
	mock2 := NewMockNotifier()

	multi := NewMultiNotifier(mock1, mock2, nil)
	assert.Len(t, multi.Notifiers(), 2)

	err := multi.Notify(context.Background(), NotifyStoryCompleted, "Story 1", "Done")
	require.NoError(t, err)

	require.Len(t, mock1.Notifications, 1)
	assert.Equal(t, "Story 1", mock1.Notifications[0].Title)

	require.Len(t, mock2.Notifications, 1)
	assert.Equal(t, "Story 1", mock2.Notifications[0].Title)
}

func TestMultiNotifier_AggregatesErrors(t *testing.T) {
	mock := NewMockNotifier()
	errNotif := &errorNotifier{err: errors.New("dispatch failed")}

	multi := NewMultiNotifier(mock, errNotif)
	err := multi.Notify(context.Background(), NotifyBuildFailed, "Error", "Details")

	require.Error(t, err)
	assert.Contains(t, err.Error(), "dispatch failed")
	require.Len(t, mock.Notifications, 1)
}
