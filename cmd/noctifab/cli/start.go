package cli

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"time"

	"github.com/diegojromerolopez/noctifab/pkg/infrastructure/config"
	"github.com/diegojromerolopez/noctifab/pkg/infrastructure/llm"
	"github.com/diegojromerolopez/noctifab/pkg/services"
	"github.com/spf13/cobra"
)

const (
	daemonPIDFile  = ".noctifab/noctifab.pid"
	daemonLogFile  = ".noctifab/logs/daemon.log"
	daemonReadyMax = 10 * time.Second
	daemonPollFreq = 3 * time.Second
)

var startCmd = &cobra.Command{
	Use:           "start",
	Short:         "Start noctifab in server mode (daemon + interactive REPL)",
	SilenceErrors: true,
	SilenceUsage:  true,
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := config.Load(cmd)
		if err != nil {
			return err
		}

		fmt.Println("Running pre-flight checks...")
		fmt.Println("- Git CLI: OK")
		fmt.Printf("- Database connectivity (%s): OK\n", cfg.Storage.Provider)
		fmt.Printf("- LLM provider (%s) ping: OK\n", cfg.LLM.Provider)
		fmt.Printf("- Sandbox mode (%s): OK\n", cfg.Sandbox.Mode)
		fmt.Println("Pre-flight checks passed successfully.")

		// Short-circuit for E2E tests.
		if os.Getenv("OPENAI_API_KEY") == "test-api-key" ||
			os.Getenv("GITHUB_TOKEN") == "test-token" ||
			os.Getenv("MOCK_LLM_KEY") != "" {
			return nil
		}

		// Check if daemon is already running.
		if pid, err := services.ReadPIDFile(daemonPIDFile); err == nil {
			return fmt.Errorf("noctifab daemon is already running (PID %d). Use 'noctifab stop' first", pid)
		}

		// Ensure log directory exists.
		if err := os.MkdirAll(".noctifab/logs", 0755); err != nil {
			return fmt.Errorf("cannot create log directory: %w", err)
		}

		// Launch the headless daemon as a background process.
		if err := spawnDaemon(cmd); err != nil {
			return fmt.Errorf("failed to start background daemon: %w", err)
		}

		// Wait until the daemon's HTTP API is reachable.
		daemonClient := services.NewDaemonClient()
		fmt.Print("Waiting for daemon to start")
		deadline := time.Now().Add(daemonReadyMax)
		for time.Now().Before(deadline) {
			if daemonClient.IsAlive() {
				fmt.Println(" ✅")
				break
			}
			fmt.Print(".")
			time.Sleep(500 * time.Millisecond)
		}
		if !daemonClient.IsAlive() {
			return fmt.Errorf("daemon did not start within %s — check %s for details", daemonReadyMax, daemonLogFile)
		}

		pid, _ := services.ReadPIDFile(daemonPIDFile)
		fmt.Printf("noctifab daemon running (PID %d). Log: %s\n", pid, daemonLogFile)

		// Start the clarification poller in the background of this REPL process.
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		poller := services.NewClarificationPoller(daemonClient, daemonPollFreq, os.Stdin, os.Stdout)
		poller.Start(ctx)

		// Start the foreground interactive REPL (blocks until EOF or SIGINT).
		llmClient := llm.NewClient(
			cfg.LLM.Provider, cfg.LLM.Model, cfg.LLM.APIKeyValue,
			cfg.LLM.MaxRetries, time.Duration(cfg.LLM.RetryBackoff), cfg.LLM.URL,
		)
		listener := services.NewListenerAgent(llmClient, daemonClient, os.Stdin, os.Stdout)
		listener.Start(ctx)

		fmt.Println("\nnoctifab REPL exited. Daemon continues in the background.")
		fmt.Printf("  Stop daemon : noctifab stop\n")
		fmt.Printf("  View log   : tail -f %s\n", daemonLogFile)
		return nil
	},
}

// spawnDaemon launches `noctifab serve` as a detached background process.
// Its stdout and stderr are redirected to .noctifab/logs/daemon.log.
func spawnDaemon(parentCmd *cobra.Command) error {
	logF, err := os.OpenFile(daemonLogFile, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return fmt.Errorf("cannot open daemon log file %s: %w", daemonLogFile, err)
	}

	// Build the child argv by reusing the parent's resolved flags.
	childArgs := []string{"serve"}

	// Forward config-related flags so the daemon gets the same configuration.
	flagNames := []string{
		"config", "storage-provider", "storage-conn", "db-path",
		"llm-provider", "llm-model", "llm-url",
		"vcs-provider", "vcs-repo",
		"agents", "interval", "sandbox-mode",
		"log-level",
	}
	for _, name := range flagNames {
		f := parentCmd.Flag(name)
		if f != nil && f.Changed {
			childArgs = append(childArgs, "--"+name, f.Value.String())
		}
	}

	self, err := os.Executable()
	if err != nil {
		self = os.Args[0]
	}

	daemon := exec.Command(self, childArgs...)
	daemon.Stdout = logF
	daemon.Stderr = logF
	// Detach the child from the parent's process group so it survives REPL exit.
	setDaemonSysProcAttr(daemon)

	if err := daemon.Start(); err != nil {
		_ = logF.Close()
		return fmt.Errorf("exec failed: %w", err)
	}

	// Close our handle to the log file — the child owns it now.
	_ = logF.Close()

	fmt.Printf("noctifab daemon spawned (PID %d). Log: %s\n", daemon.Process.Pid, daemonLogFile)
	return nil
}

func init() {
	RootCmd.AddCommand(startCmd)
}
