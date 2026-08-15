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
	RootCmd.AddCommand(resumeCmd)
}
