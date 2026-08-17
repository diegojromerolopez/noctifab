package cli

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/diegojromerolopez/noctifab/pkg/domain"
	"github.com/diegojromerolopez/noctifab/pkg/infrastructure/notifier"
	"github.com/diegojromerolopez/noctifab/pkg/interfaces/web"
	"github.com/diegojromerolopez/noctifab/pkg/services"
	"github.com/spf13/cobra"
)

// startConcurrentWebServer parses flags and launches the concurrent web dashboard if enabled.
func startConcurrentWebServer(cmd *cobra.Command, repo domain.StateRepository, mailbox *services.CommandMailbox) (*web.WebServer, string, int, bool, func()) {
	webHost := "127.0.0.1"
	webPort := 8080

	if webFlag := cmd.Flags().Lookup("web"); webFlag != nil {
		if enabled, _ := cmd.Flags().GetBool("web"); enabled {
			if hostFlag := cmd.Flags().Lookup("web-host"); hostFlag != nil {
				if h, err := cmd.Flags().GetString("web-host"); err == nil && h != "" {
					webHost = h
				}
			}
			if portFlag := cmd.Flags().Lookup("web-port"); portFlag != nil {
				if p, err := cmd.Flags().GetInt("web-port"); err == nil && p > 0 {
					webPort = p
				}
			}
			serverCfg := web.WebServerConfig{
				Host: webHost,
				Port: webPort,
			}
			server := web.NewWebServer(serverCfg, repo, mailbox, nil)
			if err := server.Start(); err != nil {
				fmt.Fprintf(os.Stderr, "Warning: failed to start concurrent web dashboard: %v\n", err)
				return nil, webHost, webPort, false, func() {}
			}
			fmt.Printf("🌐 Visual Web Dashboard live at: http://%s:%d\n", webHost, webPort)
			cleanup := func() {
				shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 2*time.Second)
				defer shutdownCancel()
				_ = server.Shutdown(shutdownCtx)
			}
			return server, webHost, webPort, true, cleanup
		}
	}
	return nil, webHost, webPort, false, func() {}
}

// StandbyParams contains dependencies required to run the persistent standby loop.
type StandbyParams struct {
	Repo      domain.StateRepository
	Mailbox   *services.CommandMailbox
	WebServer *web.WebServer
	Executor  services.StoryExecutorFunc
	TargetDir string
	WebHost   string
	WebPort   int
}

// runStandbyMode starts the perpetual background event loop waiting for developer prompt orders.
func runStandbyMode(ctx context.Context, p StandbyParams) error {
	notif := notifier.NewOSDesktopNotifier(true)

	engine := services.NewStandbyEngine(services.StandbyEngineConfig{
		Repo:     p.Repo,
		Mailbox:  p.Mailbox,
		Notifier: notif,
		Executor: p.Executor,
		BaseDir:  p.TargetDir,
		WatchFS:  true,
	})

	if p.WebServer != nil {
		p.WebServer.SetStoryChannel(engine.StoryChannel())
	}

	fmt.Println()
	fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	fmt.Println("💤 [Standby Mode] All initial user stories completed.")
	fmt.Println("🤖 Noctifab Dark Factory is now standing by in the background.")
	if p.WebServer != nil {
		fmt.Printf("🌐 Submit prompt orders at: http://%s:%d\n", p.WebHost, p.WebPort)
	}
	fmt.Println("💻 Or use the CLI: noctifab order \"<your feature prompt>\"")
	fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	standbyCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	go func() {
		select {
		case <-sigChan:
			fmt.Println("\nReceived shutdown signal. Stopping standby daemon...")
			cancel()
		case <-standbyCtx.Done():
		}
	}()

	return engine.Run(standbyCtx)
}

func extractStoryTitle(filePath string) string {
	content, err := os.ReadFile(filePath)
	if err != nil {
		return ""
	}
	lines := strings.Split(string(content), "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "# ") {
			title := strings.TrimPrefix(line, "# ")
			return strings.TrimSpace(title)
		}
	}
	return ""
}
