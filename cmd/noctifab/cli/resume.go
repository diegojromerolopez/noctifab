package cli

import (
	"github.com/spf13/cobra"
)

var resumeCmd = &cobra.Command{
	Use:           "resume [target_path]",
	Short:         "Resume software specification execution from the first incomplete story",
	SilenceErrors: true,
	SilenceUsage:  true,
	RunE:          runStartCommand,
}

func init() {
	resumeCmd.Flags().StringP("spec", "s", "SPEC.md", "Path to feature specification file")
	resumeCmd.Flags().BoolP("web", "w", false, "Launch the real-time visual web dashboard concurrently during execution")
	resumeCmd.Flags().Int("web-port", 8080, "Port for the concurrent visual web dashboard")
	resumeCmd.Flags().String("web-host", "127.0.0.1", "Host address to bind the concurrent visual web dashboard")
	resumeCmd.Flags().BoolP("web-open", "o", false, "Automatically open the visual web dashboard in the default browser")
	RootCmd.AddCommand(resumeCmd)
}
