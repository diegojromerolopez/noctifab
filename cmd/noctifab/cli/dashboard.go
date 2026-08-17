package cli

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/diegojromerolopez/noctifab/pkg/domain"
	"github.com/diegojromerolopez/noctifab/pkg/infrastructure/browser"
	"github.com/diegojromerolopez/noctifab/pkg/infrastructure/storage"
	"github.com/diegojromerolopez/noctifab/pkg/interfaces/web"
	"github.com/diegojromerolopez/noctifab/pkg/services"
	"github.com/spf13/cobra"
	"golang.org/x/term"
)

var dashboardStartTime time.Time

var dashboardCmd = &cobra.Command{
	Use:   "dashboard [workspace_dir]",
	Short: "Launch the real-time progress dashboard (terminal TUI or visual web browser)",
	RunE: func(cmd *cobra.Command, args []string) error {
		webEnabled, _ := cmd.Flags().GetBool("web")
		if webEnabled {
			host, _ := cmd.Flags().GetString("host")
			if host == "" {
				host = "127.0.0.1"
			}
			port, _ := cmd.Flags().GetInt("port")
			if port == 0 {
				port = 8080
			}
			readOnly, _ := cmd.Flags().GetBool("readonly")
			openBrowser, _ := cmd.Flags().GetBool("open")
			targetDir := WorkspaceDir
			if targetDir == "" {
				targetDir = "."
			}
			if len(args) > 0 {
				targetDir = args[0]
			}
			return runWebDashboard(targetDir, host, port, readOnly, openBrowser)
		}

		if dashboardStartTime.IsZero() {
			dashboardStartTime = time.Now()
		}
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
		var activeOverlay string

		// 1-second ticker loop to update dashboard continuously without UI freezing
		go func() {
			ticker := time.NewTicker(1 * time.Second)
			defer ticker.Stop()
			for {
				select {
				case <-ticker.C:
					mu.Lock()
					overlay := activeOverlay
					mu.Unlock()

					states, err := client.GetStatusAll(ctx)
					if err != nil {
						fmt.Print("\033[H\033[J")
						fmt.Printf("Error fetching dashboard status: %v\n", err)
						continue
					}

					rendered := renderDashboard(states)
					if overlay != "" {
						rendered += "\r\n" + overlay + " "
					}
					fmt.Print(rendered)

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
						return
					}
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
				activeOverlay = "\033[1;33m⚠️  Are you sure you want to pause/resume execution? (y/n):\033[0m"
				mu.Unlock()

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
				activeOverlay = ""
				mu.Unlock()
			}

			if char == 'x' {
				mu.Lock()
				activeOverlay = "\033[1;31m⚠️  Are you sure you want to cancel the active execution? (y/n):\033[0m"
				mu.Unlock()

				confirmBuf := make([]byte, 1)
				_, _ = os.Stdin.Read(confirmBuf)
				if confirmBuf[0] == 'y' || confirmBuf[0] == 'Y' {
					_ = client.CancelStory(ctx)
				}

				mu.Lock()
				activeOverlay = ""
				mu.Unlock()
			}

			if char == 's' || char == 'S' {
				mu.Lock()
				activeOverlay = "\033[1;33m🎯 Inject mid-flight steering directive:\033[0m"
				mu.Unlock()

				_ = HandleSteerPrompt(ctx, client, fd, oldState)

				mu.Lock()
				activeOverlay = ""
				mu.Unlock()
			}

			if char == 'o' || char == 'O' || char == 'n' || char == 'N' || char == 'a' || char == 'A' {
				mu.Lock()
				activeOverlay = "\033[1;36m📝 Enter new feature specification order:\033[0m"
				mu.Unlock()

				_ = HandleNewOrderPrompt(ctx, client, fd, oldState)

				mu.Lock()
				activeOverlay = ""
				mu.Unlock()
			}

			if char == 'c' || char == 'C' {
				mu.Lock()
				activeOverlay = "\033[1;35m💬 Enter clarification response:\033[0m"
				mu.Unlock()

				_ = HandleClarificationPrompt(ctx, client, fd, oldState)

				mu.Lock()
				activeOverlay = ""
				mu.Unlock()
			}

			if char == 'd' || char == 'D' {
				mu.Lock()
				activeOverlay = "\033[1;36m🔍 Opening Failure Log Inspector...\033[0m"
				mu.Unlock()

				states, _ := client.GetStatusAll(ctx)
				_ = HandleLogInspectorModal(ctx, states, fd, oldState)

				mu.Lock()
				activeOverlay = ""
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

func runWebDashboard(targetDir string, host string, port int, readOnly bool, openBrowser bool) error {
	dbPath := filepath.Join(targetDir, ".noctifab", "data", "noctifab.db")
	repo, err := storage.NewSQLiteRepository(context.Background(), dbPath)
	if err != nil {
		return fmt.Errorf("failed to open state database at %s: %w", dbPath, err)
	}

	mailbox := services.NewCommandMailbox(repo)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go mailbox.Start(ctx)

	serverCfg := web.WebServerConfig{
		Host:     host,
		Port:     port,
		ReadOnly: readOnly,
	}

	server := web.NewWebServer(serverCfg, repo, mailbox, nil)
	if err := server.Start(); err != nil {
		return fmt.Errorf("failed to start web dashboard server: %w", err)
	}

	targetURL := fmt.Sprintf("http://%s:%d", host, port)
	modeDesc := "Read-Write (Steering & Orders Enabled)"
	if readOnly {
		modeDesc = "Read-Only (Monitoring only)"
	}

	fmt.Println()
	fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	fmt.Println("🚀 Noctifab Visual Web Dashboard")
	fmt.Printf("➜  Local:    %s\n", targetURL)
	fmt.Printf("➜  Database: %s\n", dbPath)
	fmt.Printf("➜  Mode:     %s\n", modeDesc)
	fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	fmt.Println("Press Ctrl+C to stop.")

	if openBrowser {
		_ = browser.NewOSBrowserOpener().Open(ctx, targetURL)
	}

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
	<-sigChan

	fmt.Println("\nShutting down web dashboard...")
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer shutdownCancel()
	return server.Shutdown(shutdownCtx)
}

func init() {
	RootCmd.AddCommand(dashboardCmd)
	dashboardCmd.Flags().BoolP("web", "w", false, "Launch the real-time visual web dashboard in browser instead of TUI")
	dashboardCmd.Flags().String("host", "127.0.0.1", "Host address to bind the visual web dashboard")
	dashboardCmd.Flags().Int("port", 8080, "Port for the visual web dashboard")
	dashboardCmd.Flags().Bool("readonly", false, "Run web dashboard in read-only mode (disable steering and orders)")
	dashboardCmd.Flags().BoolP("open", "o", false, "Automatically open the visual web dashboard in the default browser")
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
