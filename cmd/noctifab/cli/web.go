package cli

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/diegojromerolopez/noctifab/pkg/infrastructure/storage"
	"github.com/diegojromerolopez/noctifab/pkg/interfaces/web"
	"github.com/diegojromerolopez/noctifab/pkg/services"
	"github.com/spf13/cobra"
)

var (
	webHost     string
	webPort     int
	webReadOnly bool
)

var webCmd = &cobra.Command{
	Use:   "web [workspace_dir]",
	Short: "Launch the real-time visual web dashboard and DAG explorer",
	RunE: func(cmd *cobra.Command, args []string) error {
		targetDir := WorkspaceDir
		if targetDir == "" {
			targetDir = "."
		}
		if len(args) > 0 {
			targetDir = args[0]
		}

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
			Host:     webHost,
			Port:     webPort,
			ReadOnly: webReadOnly,
		}

		server := web.NewWebServer(serverCfg, repo, mailbox, nil)
		if err := server.Start(); err != nil {
			return fmt.Errorf("failed to start web dashboard server: %w", err)
		}

		fmt.Printf("🌐 Noctifab Visual Web Dashboard running at: http://%s:%d\n", webHost, webPort)
		fmt.Println("Press Ctrl+C to stop.")

		sigChan := make(chan os.Signal, 1)
		signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
		<-sigChan

		fmt.Println("\nShutting down web dashboard...")
		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer shutdownCancel()
		return server.Shutdown(shutdownCtx)
	},
}

func init() {
	RootCmd.AddCommand(webCmd)
	webCmd.Flags().StringVar(&webHost, "host", "127.0.0.1", "Host address to bind the web dashboard")
	webCmd.Flags().IntVar(&webPort, "port", 8080, "Port for the web dashboard")
	webCmd.Flags().BoolVar(&webReadOnly, "readonly", false, "Run in read-only mode (disable steering and order mutations)")
}
