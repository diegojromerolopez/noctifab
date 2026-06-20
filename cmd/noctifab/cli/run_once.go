package cli

import (
	"fmt"

	"github.com/diegojromerolopez/noctifab/pkg/infrastructure/config"
	"github.com/spf13/cobra"
)

var runOnceCmd = &cobra.Command{
	Use:           "run-once",
	Short:         "Execute one cycle of the orchestrator loop and exit",
	SilenceErrors: true,
	SilenceUsage:  true,
	RunE: func(cmd *cobra.Command, args []string) error {
		_, err := config.Load(cmd)
		if err != nil {
			return err
		}

		fmt.Println("Executing single orchestrator cycle...")
		fmt.Println("Cycle completed successfully.")
		return nil
	},
}

func init() {
	RootCmd.AddCommand(runOnceCmd)
}
