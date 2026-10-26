package cli

import (
	"context"
	"os"

	"github.com/diegojromerolopez/noctifab/pkg/services/demo"
	"github.com/spf13/cobra"
)

var (
	demoProject string

	// demoStartRunnerOverride allows injecting mock runners for testing.
	demoStartRunnerOverride demo.StartRunner
)

var demoCmd = &cobra.Command{
	Use:   "demo [target_dir]",
	Short: "Run an autonomous dark factory demo sandbox",
	Args:  cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		targetDir := "."
		if len(args) > 0 && args[0] != "" {
			targetDir = args[0]
		}

		runner := demoStartRunnerOverride
		if runner == nil {
			runner = func(_ context.Context, dir string) error {
				origWd, err := os.Getwd()
				if err == nil {
					defer func() { _ = os.Chdir(origWd) }()
				}
				return runStartCommand(startCmd, []string{dir})
			}
		}

		svc := demo.NewDemoService(runner)
		cfg := demo.DemoConfig{
			Archetype: demo.DemoArchetype(demoProject),
			TargetDir: targetDir,
		}
		return svc.Run(cmd.Context(), cfg)
	},
}

func init() {
	RootCmd.AddCommand(demoCmd)
	demoCmd.Flags().StringVar(&demoProject, "project", "t4", "Demo project archetype (t4, cli, rest)")
}
