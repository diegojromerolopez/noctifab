package cli

import (
	"fmt"

	"github.com/diegojromerolopez/noctifab/pkg/version"
	"github.com/spf13/cobra"
)

var (
	versionJSON    bool
	versionShort   bool
	versionVerbose bool
)

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Print the noctifab version, commit, and build information",
	Long:  "Display detailed version metadata including semantic release version, Git commit hash, and commit date.",
	RunE: func(cmd *cobra.Command, args []string) error {
		info := version.GetInfo()

		if versionJSON {
			jsonStr, err := info.JSON()
			if err != nil {
				return err
			}
			fmt.Fprintln(cmd.OutOrStdout(), jsonStr)
			return nil
		}

		if versionShort {
			fmt.Fprintln(cmd.OutOrStdout(), info.Short())
			return nil
		}

		if versionVerbose {
			fmt.Fprintln(cmd.OutOrStdout(), info.Verbose())
			return nil
		}

		fmt.Fprintln(cmd.OutOrStdout(), info.String())
		return nil
	},
}

func init() {
	versionCmd.Flags().BoolVar(&versionJSON, "json", false, "Output version information in JSON format")
	versionCmd.Flags().BoolVarP(&versionShort, "short", "s", false, "Output only the version string")
	versionCmd.Flags().BoolVarP(&versionVerbose, "verbose", "v", false, "Output verbose version and environment metadata")
	RootCmd.AddCommand(versionCmd)
}
