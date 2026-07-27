package cli

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"sync"
	"sync/atomic"
	"time"

	"github.com/diegojromerolopez/noctifab/pkg/domain"
	"github.com/diegojromerolopez/noctifab/pkg/services"
	"github.com/spf13/cobra"
	"golang.org/x/term"
)

var dashboardCmd = &cobra.Command{
	Use:   "dashboard",
	Short: "Launch the real-time terminal user interface progress dashboard",
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := ensureDaemonRunning(); err != nil {
			return fmt.Errorf("noctifab daemon is not running and could not be auto-started: %w", err)
		}
		client := services.NewDaemonClient()

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		fd := int(os.Stdin.Fd())
		if !term.IsTerminal(fd) {
			// Non-interactive fallback: periodically dump status to stdout every 5 seconds
			ticker := time.NewTicker(5 * time.Second)
			defer ticker.Stop()
			for {
				select {
				case <-ticker.C:
					states, err := client.GetStatusAll(ctx)
					if err != nil {
						fmt.Fprintf(os.Stderr, "Error fetching status: %v\n", err)
						continue
					}
					fmt.Print(renderDashboard(states))

					// Auto-exit if stories exist and all are completed
					allFinished := true
					for _, st := range states {
						if st.StoryStatus == domain.StoryRunning || st.StoryStatus == domain.StoryPaused || st.StoryStatus == domain.StoryIdle {
							allFinished = false
							break
						}
					}
					if allFinished && len(states) > 0 {
						time.Sleep(2 * time.Second)
						cancel()
						return nil
					}
				case <-ctx.Done():
					return nil
				}
			}
		}

		// Raw mode keyboard interaction
		oldState, err := term.MakeRaw(fd)
		if err != nil {
			return fmt.Errorf("failed to put terminal in raw mode: %w", err)
		}
		defer func() { _ = term.Restore(fd, oldState) }()

		var mu sync.Mutex
		promptActive := false

		// 1-second ticker loop to update dashboard
		go func() {
			ticker := time.NewTicker(1 * time.Second)
			defer ticker.Stop()
			for {
				select {
				case <-ticker.C:
					mu.Lock()
					if promptActive {
						mu.Unlock()
						continue
					}
					states, err := client.GetStatusAll(ctx)
					if err != nil {
						// Print error to terminal
						fmt.Print("\033[H\033[J")
						fmt.Printf("Error fetching dashboard status: %v\n", err)
						mu.Unlock()
						continue
					}
					fmt.Print(renderDashboard(states))

					// Auto-exit if stories exist and all are completed
					allFinished := true
					for _, st := range states {
						if st.StoryStatus == domain.StoryRunning || st.StoryStatus == domain.StoryPaused || st.StoryStatus == domain.StoryIdle {
							allFinished = false
							break
						}
					}
					if allFinished && len(states) > 0 {
						mu.Unlock()
						time.Sleep(2 * time.Second)
						cancel()
						return
					}
					mu.Unlock()
				case <-ctx.Done():
					return
				}
			}
		}()

		// Read keystrokes
		buf := make([]byte, 1)
		for {
			n, err := os.Stdin.Read(buf)
			if err != nil || n == 0 {
				break
			}
			char := buf[0]
			if char == 'q' {
				// Check if any stories are still running/paused/idle
				states, err := client.GetStatusAll(ctx)
				stillRunning := false
				if err == nil {
					for _, st := range states {
						if st.StoryStatus == domain.StoryRunning || st.StoryStatus == domain.StoryPaused || st.StoryStatus == domain.StoryIdle {
							stillRunning = true
							break
						}
					}
				}
				cancel()
				if stillRunning {
					return fmt.Errorf("dashboard quit by user while stories are still active")
				}
				return nil
			}

			if char == 'p' {
				mu.Lock()
				promptActive = true
				mu.Unlock()

				// Print confirmation banner
				fmt.Print("\r\n\033[1;33m⚠️  Are you sure you want to pause/resume execution? (y/n):\033[0m ")
				confirmBuf := make([]byte, 1)
				_, _ = os.Stdin.Read(confirmBuf)
				if confirmBuf[0] == 'y' || confirmBuf[0] == 'Y' {
					states, err := client.GetStatusAll(ctx)
					if err == nil && len(states) > 0 {
						st := states[0]
						if st.StoryStatus == domain.StoryPaused {
							_ = client.ResumeStory(ctx)
						} else {
							_ = client.PauseStory(ctx)
						}
					}
				}

				mu.Lock()
				promptActive = false
				mu.Unlock()
			}

			if char == 'x' {
				mu.Lock()
				promptActive = true
				mu.Unlock()

				// Print confirmation banner
				fmt.Print("\r\n\033[1;31m⚠️  Are you sure you want to cancel the active execution? (y/n):\033[0m ")
				confirmBuf := make([]byte, 1)
				_, _ = os.Stdin.Read(confirmBuf)
				if confirmBuf[0] == 'y' || confirmBuf[0] == 'Y' {
					_ = client.CancelStory(ctx)
				}

				mu.Lock()
				promptActive = false
				mu.Unlock()
			}

			if char == 'n' || char == 'N' || char == 'a' || char == 'A' {
				mu.Lock()
				promptActive = true
				mu.Unlock()

				_ = HandleNewOrderPrompt(ctx, client, fd, oldState)

				mu.Lock()
				promptActive = false
				mu.Unlock()
			}

			if char == 'c' || char == 'C' {
				mu.Lock()
				promptActive = true
				mu.Unlock()

				_ = HandleClarificationPrompt(ctx, client, fd, oldState)

				mu.Lock()
				promptActive = false
				mu.Unlock()
			}
		}

		return nil
	},
}

var animationFrame int64

// statusEmoji returns an emoji that reflects the story lifecycle state.
// When the story is running the hourglass alternates on each call to convey
// activity; for every other terminal state a fixed, meaningful symbol is used.
func statusEmoji(status domain.StoryStatus) string {
	switch status {
	case domain.StoryRunning:
		frames := []string{"⏳", "⌛"}
		frame := atomic.AddInt64(&animationFrame, 1)
		return frames[int(frame%int64(len(frames)))]
	case domain.StorySuccess:
		return "✅"
	case domain.StoryFailed:
		return "❌"
	case domain.StoryPaused:
		return "🛑"
	case domain.StoryCancelled:
		return "⛔"
	default:
		return ""
	}
}

// taskEmoji maps a task status to a single fixed emoji for compact display.
func taskEmoji(status domain.TaskStatus) string {
	switch status {
	case domain.TaskSuccess:
		return "✅"
	case domain.TaskInProgress:
		return "🔄"
	case domain.TaskFailed, domain.TaskConflictFailed:
		return "❌"
	case domain.TaskConflictBlocked, domain.TaskInterrupted:
		return "⚠️"
	default: // PENDING and anything else
		return "⏸"
	}
}

func renderDashboard(states []*domain.State) string {
	return renderEnhancedDashboard(states)
}

// deduplicateStates collapses multiple state rows that share the same
// FeatureName into a single entry. When duplicates exist, the most
// "advanced" state is kept: any non-idle state beats an idle one; among
// non-idle states the one with more tasks wins (i.e. the planner row).
func deduplicateStates(states []*domain.State) []*domain.State {
	seen := make(map[string]*domain.State, len(states))
	// Preserve original order for the first occurrence of each name.
	order := make([]string, 0, len(states))
	for _, st := range states {
		key := st.Metadata.FeatureName
		if key == "" {
			key = st.ID // fallback: keep unnamed rows under their own key
		}
		existing, ok := seen[key]
		if !ok {
			seen[key] = st
			order = append(order, key)
			continue
		}
		// Prefer any non-idle state over an idle one.
		existingIdle := existing.StoryStatus == domain.StoryIdle || existing.StoryStatus == ""
		newIdle := st.StoryStatus == domain.StoryIdle || st.StoryStatus == ""
		if existingIdle && !newIdle {
			seen[key] = st
			continue
		}
		// Among equally-idle states, prefer the one with more tasks.
		if len(st.Tasks) > len(existing.Tasks) {
			seen[key] = st
		}
	}
	result := make([]*domain.State, 0, len(order))
	for _, key := range order {
		result = append(result, seen[key])
	}
	return result
}

func init() {
	RootCmd.AddCommand(dashboardCmd)
}

func ensureDaemonRunning() error {
	client := services.NewDaemonClient()
	if client.IsAlive() {
		return nil
	}

	execPath, err := os.Executable()
	if err != nil {
		execPath = "noctifab"
	}

	cmd := exec.Command(execPath, "serve")
	cmd.Stdout = nil
	cmd.Stderr = nil
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("could not launch daemon process: %w", err)
	}

	// Wait up to 3 seconds for daemon to respond to health checks
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if client.IsAlive() {
			return nil
		}
		time.Sleep(100 * time.Millisecond)
	}

	if !client.IsAlive() {
		return fmt.Errorf("daemon started (PID %d) but health check timed out", cmd.Process.Pid)
	}
	return nil
}
