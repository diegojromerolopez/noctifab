package cli

import (
	"context"
	"fmt"
	"os"
	"strings"
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
		client := services.NewDaemonClient()
		if !client.IsAlive() {
			return fmt.Errorf("noctifab daemon is not running. Please run 'noctifab serve' first")
		}

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
	var sb strings.Builder
	// Double-buffering: cursor reset and screen clear
	sb.WriteString("\033[H\033[J")

	if len(states) == 0 {
		sb.WriteString("No active user stories found in the daemon.")
		return sb.String()
	}

	// Deduplicate: per FeatureName keep only the most advanced state.
	// "More advanced" = non-idle > idle, and more tasks > fewer tasks.
	// This guards against orphan stub rows that may exist in the DB.
	deduped := deduplicateStates(states)

	primary := deduped[0]
	sb.WriteString("NOCTIFAB TERMINAL DASHBOARD - SYSTEM PORT\r\n")
	fmt.Fprintf(&sb, "Path: %s\r\n", primary.ProjectPath)
	fmt.Fprintf(&sb, "Global Status: %s %s\r\n", primary.StoryStatus, statusEmoji(primary.StoryStatus))
	fmt.Fprintf(&sb, "Cost: $%s\r\n", primary.Metadata.TotalCostUSD)
	fmt.Fprintf(&sb, "Tokens Used: %d\r\n\r\n", primary.Metadata.TotalTokensUsed)

	sb.WriteString("ACTIVE USER STORIES:\r\n")
	for _, st := range deduped {
		totalProgress := 0
		if len(st.Tasks) > 0 {
			for _, t := range st.Tasks {
				totalProgress += t.Progress
			}
			totalProgress = totalProgress / len(st.Tasks)
		}

		// Progress bar (10 blocks)
		barLen := totalProgress / 10
		var barRunes []rune
		for i := 0; i < 10; i++ {
			if i < barLen {
				barRunes = append(barRunes, '█')
			} else {
				barRunes = append(barRunes, '░')
			}
		}

		fmt.Fprintf(&sb, "• Story: %s | Status: %s | Progress: [%s] %d%%\r\n", st.Metadata.FeatureName, st.StoryStatus, string(barRunes), totalProgress)

		for _, t := range st.Tasks {
			emoji := taskEmoji(t.Status)
			if t.Status == domain.TaskFailed && t.FailureLog != "" {
				// Show the last non-blank line: the header is always first,
				// the actual error or diff line is at the tail of the log.
				reason := ""
				for _, line := range strings.Split(t.FailureLog, "\n") {
					if trimmed := strings.TrimSpace(line); trimmed != "" {
						reason = trimmed
					}
				}
				fmt.Fprintf(&sb, "  %s %s (%d%%) — %s\r\n", emoji, t.Title, t.Progress, reason)
			} else {
				fmt.Fprintf(&sb, "  %s %s (%d%%)\r\n", emoji, t.Title, t.Progress)
			}
		}
		sb.WriteString("\r\n")
	}

	sb.WriteString("[q] Quit | [p] Pause/Resume | [x] Cancel")

	return sb.String()
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
