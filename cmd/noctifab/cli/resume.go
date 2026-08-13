package cli

import (
	"github.com/spf13/cobra"
)

var resumeCmd = &cobra.Command{
	Use:           "resume [target_path]",
	Short:         "Resume software specification execution from the first incomplete story",
	SilenceErrors: true,
	SilenceUsage:  true,
	RunE: func(cmd *cobra.Command, args []string) error {
		_ = cmd.Flags().Set("resume", "true")
		return runStartCommand(cmd, args)
	},
}

func init() {
	resumeCmd.Flags().StringP("spec", "s", "SPEC.md", "Path to feature specification file")
	resumeCmd.Flags().Bool("resume", true, "Resume execution from the first incomplete user story")
	RootCmd.AddCommand(resumeCmd)
}
