package cli

import (
	"context"
	"os"

	"github.com/diegojromerolopez/noctifab/pkg/services/demo"
	"github.com/spf13/cobra"
)

var (
	demoProject     string
	demoTargetDir   string
	demoOffline     bool
	demoSpeedFactor float64
	demoCleanup     bool

	// demoStartRunnerOverride allows injecting mock runners for testing.
	demoStartRunnerOverride demo.StartRunner
)

var demoCmd = &cobra.Command{
	Use:   "demo [target_dir]",
	Short: "Run an autonomous dark factory demo sandbox",
	Args:  cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		targetDir := demoTargetDir
		if len(args) > 0 && args[0] != "" {
			targetDir = args[0]
		}
		if targetDir == "" {
			targetDir = "demo"
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
			Archetype:    demo.DemoArchetype(demoProject),
			TargetDir:    targetDir,
			ForceOffline: demoOffline,
			SpeedFactor:  demoSpeedFactor,
			Cleanup:      demoCleanup,
		}
		return svc.Run(cmd.Context(), cfg)
	},
}

func init() {
	RootCmd.AddCommand(demoCmd)
	demoCmd.Flags().StringVar(&demoProject, "project", "t4", "Demo project archetype (t4, cli, rest)")
	demoCmd.Flags().StringVar(&demoTargetDir, "dir", "demo", "Target directory for demo project generation")
	demoCmd.Flags().BoolVar(&demoOffline, "offline", false, "Force offline execution using deterministic mock replay")
	demoCmd.Flags().Float64Var(&demoSpeedFactor, "speed", 1.0, "Execution speed multiplier")
	demoCmd.Flags().BoolVar(&demoCleanup, "cleanup", false, "Clean up and remove generated demo directory on exit")
}
