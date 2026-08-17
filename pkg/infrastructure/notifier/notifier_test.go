package notifier

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestOSDesktopNotifier_DarwinExecution(t *testing.T) {
	var executedName string
	var executedArgs []string

	mockExec := func(ctx context.Context, name string, args ...string) error {
		executedName = name
		executedArgs = args
		return nil
	}

	notifier := &OSDesktopNotifier{
		SoundEnabled: true,
		ExecCommand:  mockExec,
	}

	err := notifier.Notify(context.Background(), NotifyStoryCompleted, "Noctifab", "Feature build succeeded 3/3")
	require.NoError(t, err)

	assert.NotEmpty(t, executedName)
	assert.NotEmpty(t, executedArgs)
}

func TestOSDesktopNotifier_CommandFailureHandling(t *testing.T) {
	mockExec := func(ctx context.Context, name string, args ...string) error {
		return errors.New("command execution failed")
	}

	notifier := &OSDesktopNotifier{
		SoundEnabled: false,
		ExecCommand:  mockExec,
	}

	err := notifier.Notify(context.Background(), NotifyClarificationNeed, "Clarification Needed", "Please answer question")
	assert.Error(t, err)
}

func TestNoopNotifier(t *testing.T) {
	n := &NoopNotifier{}
	err := n.Notify(context.Background(), NotifyStoryCompleted, "Title", "Message")
	assert.NoError(t, err)
}

func TestMockNotifier(t *testing.T) {
	m := NewMockNotifier()
	err := m.Notify(context.Background(), NotifyStoryCompleted, "Story 1", "Passed")
	require.NoError(t, err)

	require.Len(t, m.Notifications, 1)
	assert.Equal(t, NotifyStoryCompleted, m.Notifications[0].Kind)
	assert.Equal(t, "Story 1", m.Notifications[0].Title)
	assert.Equal(t, "Passed", m.Notifications[0].Message)
}

func TestEscapeAppleScript(t *testing.T) {
	input := `Hello "World" and 'Friends' \ Path`
	escaped := escapeAppleScript(input)
	assert.Contains(t, escaped, `\"World\"`)
	assert.Contains(t, escaped, `\\ Path`)
	assert.Contains(t, escaped, `'Friends'`)
}

func TestEscapePowerShell(t *testing.T) {
	input := `it's a "test" with 'single quotes'`
	escaped := escapePowerShell(input)
	assert.Equal(t, `it''s a "test" with ''single quotes''`, escaped)
}
