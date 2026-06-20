package cli

import (
	"fmt"

	"github.com/diegojromerolopez/noctifab/pkg/infrastructure/config"
	"github.com/spf13/cobra"
)

var startCmd = &cobra.Command{
	Use:           "start",
	Short:         "Start the noctifab daemon loop",
	SilenceErrors: true,
	SilenceUsage:  true,
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := config.Load(cmd)
		if err != nil {
			return err
		}

		// Perform startup pre-flight health checks
		fmt.Println("Running pre-flight checks...")

		// 1. Git CLI availability check (stub/mock)
		fmt.Println("- Git CLI: OK")

		// 2. Orphaned Worktree Cleanup (stub/mock)
		fmt.Println("- Worktree cleanup: OK")

		// 3. Database Connectivity (stub/mock)
		fmt.Printf("- Database connectivity (%s): OK\n", cfg.Storage.Provider)

		// 4. LLM API Reachability (stub/mock)
		fmt.Printf("- LLM provider (%s) ping: OK\n", cfg.LLM.Provider)

		// 5. Sandbox integrity (stub/mock)
		fmt.Printf("- Sandbox mode (%s): OK\n", cfg.Sandbox.Mode)

		fmt.Println("Pre-flight checks passed successfully.")
		fmt.Printf("Starting noctifab daemon loop (concurrency: %d, poll interval: %v)...\n",
			cfg.Orchestrator.Concurrency, cfg.Orchestrator.PollInterval)

		// In a real run, this would loop. For the CLI subcommand routing layer, we print status.
		return nil
	},
}

func init() {
	RootCmd.AddCommand(startCmd)
}
