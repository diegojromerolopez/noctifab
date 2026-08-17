package notifier

import (
	"context"
	"fmt"
	"os/exec"
	"runtime"
	"strings"
	"time"
)

// NotificationKind represents the event category triggering an alert.
type NotificationKind string

const (
	NotifyStoryCompleted    NotificationKind = "STORY_COMPLETED"
	NotifyClarificationNeed NotificationKind = "CLARIFICATION_REQUIRED"
	NotifyBuildFailed       NotificationKind = "BUILD_FAILED"
	NotifyTaskStarted       NotificationKind = "TASK_STARTED"
)

// DesktopNotifier sends cross-platform desktop notifications.
type DesktopNotifier interface {
	Notify(ctx context.Context, kind NotificationKind, title, message string) error
}

// OSDesktopNotifier dispatches native desktop alerts using platform tools.
type OSDesktopNotifier struct {
	SoundEnabled bool
	ExecCommand  func(ctx context.Context, name string, args ...string) error
}

// NewOSDesktopNotifier creates a new platform-native desktop notifier.
func NewOSDesktopNotifier(sound bool) *OSDesktopNotifier {
	return &OSDesktopNotifier{
		SoundEnabled: sound,
		ExecCommand: func(ctx context.Context, name string, args ...string) error {
			cmd := exec.CommandContext(ctx, name, args...)
			return cmd.Run()
		},
	}
}

// Notify triggers a desktop notification.
func (n *OSDesktopNotifier) Notify(ctx context.Context, kind NotificationKind, title, message string) error {
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()

	switch runtime.GOOS {
	case "darwin":
		soundSnippet := ""
		if n.SoundEnabled {
			soundSnippet = ` sound name "Glass"`
		}
		script := fmt.Sprintf(`display notification "%s" with title "%s"%s`, escapeAppleScript(message), escapeAppleScript(title), soundSnippet)
		return n.ExecCommand(ctx, "osascript", "-e", script)

	case "linux":
		args := []string{"-a", "Noctifab", title, message}
		if n.SoundEnabled {
			args = append(args, "-u", "normal")
		}
		return n.ExecCommand(ctx, "notify-send", args...)

	case "windows":
		psSafeTitle := escapePowerShell(title)
		psSafeMessage := escapePowerShell(message)
		psScript := fmt.Sprintf(`[reflection.assembly]::loadwithpartialname('System.Windows.Forms'); [System.Windows.Forms.MessageBox]::Show('%s', '%s')`, psSafeMessage, psSafeTitle)
		return n.ExecCommand(ctx, "powershell", "-Command", psScript)

	default:
		return nil
	}
}

// NoopNotifier silently discards notifications (useful for headless CI/CD).
type NoopNotifier struct{}

func (n *NoopNotifier) Notify(ctx context.Context, kind NotificationKind, title, message string) error {
	return nil
}

// MockNotifier captures notifications in memory for unit testing.
type MockNotifier struct {
	Notifications []MockNotification
}

type MockNotification struct {
	Kind    NotificationKind
	Title   string
	Message string
}

func NewMockNotifier() *MockNotifier {
	return &MockNotifier{
		Notifications: make([]MockNotification, 0),
	}
}

func (m *MockNotifier) Notify(ctx context.Context, kind NotificationKind, title, message string) error {
	m.Notifications = append(m.Notifications, MockNotification{
		Kind:    kind,
		Title:   title,
		Message: message,
	})
	return nil
}

func escapeAppleScript(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, `"`, `\"`)
	return s
}

func escapePowerShell(s string) string {
	return strings.ReplaceAll(s, `'`, `''`)
}
