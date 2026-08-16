package cli

import (
	"context"

	"github.com/diegojromerolopez/noctifab/pkg/services/demo"
	"github.com/spf13/cobra"
)

var (
	demoProject     string
	demoOffline     bool
	demoSpeedFactor float64
	demoNoCleanup   bool
)

var demoCmd = &cobra.Command{
	Use:   "demo",
	Short: "Run a 2-minute zero-config autonomous dark factory demo sandbox",
	RunE: func(cmd *cobra.Command, args []string) error {
		svc := demo.NewDemoService()
		cfg := demo.DemoConfig{
			Archetype:    demo.DemoArchetype(demoProject),
			ForceOffline: demoOffline,
			SpeedFactor:  demoSpeedFactor,
			NoCleanup:    demoNoCleanup,
		}
		return svc.Run(context.Background(), cfg)
	},
}

func init() {
	RootCmd.AddCommand(demoCmd)
	demoCmd.Flags().StringVar(&demoProject, "project", "cli", "Demo project archetype (cli, rest)")
	demoCmd.Flags().BoolVar(&demoOffline, "offline", true, "Force offline execution using deterministic mock replay")
	demoCmd.Flags().Float64Var(&demoSpeedFactor, "speed", 1.0, "Execution speed multiplier")
	demoCmd.Flags().BoolVar(&demoNoCleanup, "no-cleanup", false, "Preserve the temporary demo sandbox directory on exit")
}
