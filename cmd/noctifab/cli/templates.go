package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/diegojromerolopez/noctifab/pkg/infrastructure/templates"
	"github.com/spf13/cobra"
)

var (
	templatesGlobalFlag bool
)

var templatesCmd = &cobra.Command{
	Use:   "templates",
	Short: "Inspect, manage, and initialize language sandbox templates",
	RunE: func(cmd *cobra.Command, args []string) error {
		return cmd.Help()
	},
}

var templatesListCmd = &cobra.Command{
	Use:           "list",
	Short:         "List all available language templates and their effective source",
	SilenceUsage:  true,
	SilenceErrors: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		ws := promptsWorkspace(cmd)
		mgr := templates.NewManager(ws)
		list, err := mgr.List()
		if err != nil {
			return err
		}

		out := cmd.OutOrStdout()
		_, _ = fmt.Fprintln(out, "Available language templates:")
		_, _ = fmt.Fprintf(out, "  %-14s %-12s %s\n", "NAME", "SOURCE", "DESCRIPTION")
		for _, info := range list {
			desc := info.Description
			if info.Path != "" {
				desc = fmt.Sprintf("%s (%s)", desc, info.Path)
			}
			_, _ = fmt.Fprintf(out, "  %-14s %-12s %s\n", info.Name, info.Source, desc)
		}
		return nil
	},
}

var templatesShowCmd = &cobra.Command{
	Use:           "show <name>",
	Short:         "Print the YAML configuration of a language template",
	Args:          cobra.ExactArgs(1),
	SilenceUsage:  true,
	SilenceErrors: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		ws := promptsWorkspace(cmd)
		mgr := templates.NewManager(ws)
		content, source, path, err := mgr.Get(args[0])
		if err != nil {
			return err
		}

		out := cmd.OutOrStdout()
		_, _ = fmt.Fprintf(out, "# Template: %s\n# Source:   %s\n", args[0], source)
		if path != "" {
			_, _ = fmt.Fprintf(out, "# Path:     %s\n", path)
		}
		_, _ = fmt.Fprintf(out, "\n%s", content)
		return nil
	},
}

var templatesInitCmd = &cobra.Command{
	Use:           "init [name]",
	Short:         "Export default template(s) to .noctifab/templates/ or ~/.noctifab/templates/ as editable YAML",
	Args:          cobra.MaximumNArgs(1),
	SilenceUsage:  true,
	SilenceErrors: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		ws := promptsWorkspace(cmd)
		mgr := templates.NewManager(ws)

		destDir := mgr.GetWorkspaceDir()
		if templatesGlobalFlag {
			destDir = mgr.GetGlobalDir()
			if destDir == "" {
				home, err := os.UserHomeDir()
				if err != nil {
					return fmt.Errorf("unable to determine user home directory: %w", err)
				}
				destDir = filepath.Join(home, ".noctifab", "templates")
			}
		}

		out := cmd.OutOrStdout()
		targetTemplates := []string{}
		if len(args) == 1 {
			targetTemplates = append(targetTemplates, args[0])
		} else {
			for name := range templates.BuiltinTemplates {
				targetTemplates = append(targetTemplates, name)
			}
		}

		for _, name := range targetTemplates {
			destPath, err := mgr.Export(name, destDir, false)
			if err != nil {
				if os.IsExist(err) || strings.Contains(err.Error(), "already exists") {
					_, _ = fmt.Fprintf(out, "skipped %s (already exists)\n", destPath)
					continue
				}
				return err
			}
			_, _ = fmt.Fprintf(out, "created %s\n", destPath)
		}
		return nil
	},
}

func init() {
	templatesInitCmd.Flags().BoolVarP(&templatesGlobalFlag, "global", "g", false, "Export to user global directory (~/.noctifab/templates/) instead of workspace")
	templatesCmd.AddCommand(templatesListCmd)
	templatesCmd.AddCommand(templatesShowCmd)
	templatesCmd.AddCommand(templatesInitCmd)
	RootCmd.AddCommand(templatesCmd)
}
