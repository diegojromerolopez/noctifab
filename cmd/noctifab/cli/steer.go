package cli

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/diegojromerolopez/noctifab/pkg/services"
	"github.com/spf13/cobra"
)

var (
	steerTaskID string
)

var steerCmd = &cobra.Command{
	Use:   "steer [directive]",
	Short: "Inject a real-time steering directive into an active task or session",
	Args:  cobra.MinimumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		directive := strings.Join(args, " ")
		if strings.TrimSpace(directive) == "" {
			return fmt.Errorf("steering directive cannot be empty")
		}

		if err := ensureDaemonRunning(); err != nil {
			return fmt.Errorf("noctifab daemon is not running: %w", err)
		}

		client := services.NewDaemonClient()
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		if err := client.SendSteerDirective(ctx, steerTaskID, directive); err != nil {
			return fmt.Errorf("failed to send steering directive: %w", err)
		}

		if steerTaskID != "" {
			fmt.Printf("🎯 Steering directive sent to task %s: %q\n", steerTaskID, directive)
		} else {
			fmt.Printf("🎯 Steering directive injected into active task: %q\n", directive)
		}
		return nil
	},
}

var orderCmd = &cobra.Command{
	Use:   "order [prompt]",
	Short: "Submit a new feature specification order / prompt to noctifab",
	Args:  cobra.MinimumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		prompt := strings.Join(args, " ")
		if strings.TrimSpace(prompt) == "" {
			return fmt.Errorf("order prompt cannot be empty")
		}

		if err := ensureDaemonRunning(); err != nil {
			return fmt.Errorf("noctifab daemon is not running: %w", err)
		}

		client := services.NewDaemonClient()
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		if err := client.SendOrderPrompt(ctx, prompt); err != nil {
			return fmt.Errorf("failed to send order prompt: %w", err)
		}

		fmt.Printf("✅ Order submitted successfully: %q\n", prompt)
		return nil
	},
}

func init() {
	RootCmd.AddCommand(steerCmd)
	RootCmd.AddCommand(orderCmd)
	steerCmd.Flags().StringVarP(&steerTaskID, "task-id", "t", "", "Target task ID to steer (defaults to active running task)")
}
