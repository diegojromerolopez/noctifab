package browser

import (
	"context"
	"fmt"
	"os/exec"
	"runtime"
	"time"
)

// Opener opens a given URL in the user's default system web browser.
type Opener interface {
	Open(ctx context.Context, url string) error
}

// OSBrowserOpener dispatches system commands to launch the default browser.
type OSBrowserOpener struct {
	ExecCommand func(ctx context.Context, name string, args ...string) error
}

// NewOSBrowserOpener creates a new platform-native browser opener.
func NewOSBrowserOpener() *OSBrowserOpener {
	return &OSBrowserOpener{
		ExecCommand: func(ctx context.Context, name string, args ...string) error {
			cmd := exec.CommandContext(ctx, name, args...)
			return cmd.Start()
		},
	}
}

// Open launches the target URL in the default browser.
func (b *OSBrowserOpener) Open(ctx context.Context, url string) error {
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()

	switch runtime.GOOS {
	case "darwin":
		return b.ExecCommand(ctx, "open", url)
	case "linux":
		return b.ExecCommand(ctx, "xdg-open", url)
	case "windows":
		return b.ExecCommand(ctx, "rundll32", "url.dll,FileProtocolHandler", url)
	default:
		return fmt.Errorf("unsupported platform for auto-opening browser: %s", runtime.GOOS)
	}
}

// NoopOpener silently ignores open requests (for headless CI/CD environments).
type NoopOpener struct{}

func (n *NoopOpener) Open(ctx context.Context, url string) error {
	return nil
}
