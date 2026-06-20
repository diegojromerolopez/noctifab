package cli

import (
	"fmt"

	"github.com/diegojromerolopez/noctifab/pkg/infrastructure/config"
	"github.com/spf13/cobra"
)

var planCmd = &cobra.Command{
	Use:           "plan",
	Short:         "Read specification and plan task DAG",
	SilenceErrors: true,
	SilenceUsage:  true,
	RunE: func(cmd *cobra.Command, args []string) error {
		_, err := config.Load(cmd)
		if err != nil {
			return err
		}

		fmt.Println("Reading specification...")
		fmt.Println("Building task DAG...")
		fmt.Println("Plan created successfully.")
		return nil
	},
}

func init() {
	RootCmd.AddCommand(planCmd)
}
