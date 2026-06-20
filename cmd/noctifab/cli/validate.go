package cli

import (
	"fmt"

	"github.com/diegojromerolopez/noctifab/pkg/infrastructure/config"
	"github.com/spf13/cobra"
)

var validateCmd = &cobra.Command{
	Use:           "validate",
	Short:         "Validate configuration, state, and directory constraints",
	SilenceErrors: true,
	SilenceUsage:  true,
	RunE: func(cmd *cobra.Command, args []string) error {
		_, err := config.Load(cmd)
		if err != nil {
			return err
		}

		fmt.Println("Validating configuration...")
		fmt.Println("Configuration is valid.")
		return nil
	},
}

func init() {
	RootCmd.AddCommand(validateCmd)
}
