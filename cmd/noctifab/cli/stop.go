package cli

import (
	"fmt"
	"os"
	"syscall"
	"time"

	"github.com/diegojromerolopez/noctifab/pkg/usecase"
	"github.com/spf13/cobra"
)

var stopCmd = &cobra.Command{
	Use:           "stop",
	Short:         "Gracefully stop the noctifab background daemon and save state",
	SilenceErrors: true,
	SilenceUsage:  true,
	RunE: func(cmd *cobra.Command, args []string) error {
		pid, err := usecase.ReadPIDFile(daemonPIDFile)
		if err != nil {
			fmt.Println("noctifab daemon is not running. Nothing to stop.")
			return nil
		}

		proc, err := os.FindProcess(pid)
		if err != nil {
			// Process not found; clean up stale PID file.
			_ = usecase.RemovePIDFile(daemonPIDFile)
			return fmt.Errorf("process %d not found (stale PID file removed): %w", pid, err)
		}

		fmt.Printf("Sending SIGTERM to noctifab daemon (PID %d)...\n", pid)
		if err := proc.Signal(syscall.SIGTERM); err != nil {
			return fmt.Errorf("failed to signal daemon: %w", err)
		}

		// Wait up to 30 s for the process to exit.
		deadline := time.Now().Add(30 * time.Second)
		for time.Now().Before(deadline) {
			// Check if process still exists by sending signal 0.
			if err := proc.Signal(syscall.Signal(0)); err != nil {
				// Process is gone.
				_ = usecase.RemovePIDFile(daemonPIDFile)
				fmt.Println("noctifab daemon stopped and state saved.")
				return nil
			}
			time.Sleep(500 * time.Millisecond)
		}

		// Grace period expired — force kill.
		fmt.Fprintf(os.Stderr, "Grace period expired; sending SIGKILL to PID %d\n", pid)
		_ = proc.Signal(syscall.SIGKILL)
		_ = usecase.RemovePIDFile(daemonPIDFile)
		return fmt.Errorf("daemon did not exit cleanly within 30s; force-killed")
	},
}

func init() {
	RootCmd.AddCommand(stopCmd)
}
