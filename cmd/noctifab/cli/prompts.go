package cli

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/diegojromerolopez/noctifab/pkg/infrastructure/config"
	"github.com/diegojromerolopez/noctifab/pkg/infrastructure/prompts"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

func promptConfigPath(cmd *cobra.Command) string {
	configPath := ".noctifab/config.yaml"
	if flag := cmd.Flags().Lookup("config"); flag != nil && flag.Changed {
		configPath = flag.Value.String()
	} else if envVal, exists := os.LookupEnv("NOCTIFAB_CONFIG"); exists {
		configPath = envVal
	}
	return configPath
}

// promptsWorkspace returns the workspace directory used by the prompts
// commands (the directory containing .noctifab/, derived from --config).
func promptsWorkspace(cmd *cobra.Command) string {
	return filepath.Dir(filepath.Dir(promptConfigPath(cmd)))
}

// loadPromptOverrides reads only the prompts: section of the config file
// (leniently: other sections and validation errors are ignored so the
// prompts commands work in partially configured workspaces).
func loadPromptOverrides(cmd *cobra.Command) map[string]map[string]prompts.Override {
	data, err := os.ReadFile(promptConfigPath(cmd))
	if err != nil {
		return nil
	}
	var section struct {
		Prompts map[string]map[string]config.PromptOverride `yaml:"prompts"`
	}
	if err := yaml.Unmarshal(data, &section); err != nil || len(section.Prompts) == 0 {
		return nil
	}
	out := make(map[string]map[string]prompts.Override, len(section.Prompts))
	for agent, actions := range section.Prompts {
		byAction := make(map[string]prompts.Override, len(actions))
		for action, ov := range actions {
			byAction[action] = prompts.Override{Path: ov.Path, Append: ov.Append}
		}
		out[agent] = byAction
	}
	return out
}

var promptsCmd = &cobra.Command{
	Use:   "prompts",
	Short: "Inspect and customize the per-agent prompt templates",
	RunE: func(cmd *cobra.Command, args []string) error {
		return cmd.Help()
	},
}

var promptsListCmd = &cobra.Command{
	Use:           "list",
	Short:         "List every (agent, action) prompt and its effective source",
	SilenceUsage:  true,
	SilenceErrors: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		r, err := prompts.NewRenderer(promptsWorkspace(cmd), loadPromptOverrides(cmd))
		if err != nil {
			return err
		}
		out := cmd.OutOrStdout()
		for _, agent := range prompts.Agents() {
			_, _ = fmt.Fprintf(out, "%s\n", agent)
			for _, action := range prompts.Actions(agent) {
				d, dErr := r.Describe(agent, action)
				if dErr != nil {
					return dErr
				}
				status := string(d.Source)
				if d.AppendSource != "" {
					status += " + append(" + d.AppendSource + ")"
				}
				_, _ = fmt.Fprintf(out, "  %-22s %s\n", action, status)
			}
		}
		return nil
	},
}

var promptsShowCmd = &cobra.Command{
	Use:           "show <agent> <action>",
	Short:         "Print the effective prompt template of one action and its source",
	Args:          cobra.ExactArgs(2),
	SilenceUsage:  true,
	SilenceErrors: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		r, err := prompts.NewRenderer(promptsWorkspace(cmd), loadPromptOverrides(cmd))
		if err != nil {
			return err
		}
		d, err := r.Describe(args[0], args[1])
		if err != nil {
			return err
		}
		out := cmd.OutOrStdout()
		_, _ = fmt.Fprintf(out, "# Agent:  %s\n# Action: %s\n# Source: %s\n", d.Agent, d.Action, d.Source)
		if d.AppendSource != "" {
			_, _ = fmt.Fprintf(out, "# Append: %s\n", d.AppendSource)
		}
		_, _ = fmt.Fprintf(out, "\n%s\n# --- Non-overridable output contract (appended by code) ---\n%s", d.Text, prompts.Contract(d.Agent))
		return nil
	},
}

var promptsInitCmd = &cobra.Command{
	Use:           "init [agent] [action]",
	Short:         "Write embedded default templates into .noctifab/prompts/ as editable starting points",
	Args:          cobra.MaximumNArgs(2),
	SilenceUsage:  true,
	SilenceErrors: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		workspace := promptsWorkspace(cmd)
		agents := prompts.Agents()
		if len(args) >= 1 {
			agents = []string{args[0]}
		}
		out := cmd.OutOrStdout()
		for _, agent := range agents {
			actions := prompts.Actions(agent)
			if len(args) == 2 {
				actions = []string{args[1]}
			}
			if len(actions) == 0 {
				return fmt.Errorf("unknown prompt agent %q (valid agents: %v)", agent, prompts.Agents())
			}
			for _, action := range actions {
				text, err := prompts.DefaultTemplate(agent, action)
				if err != nil {
					return err
				}
				path := filepath.Join(workspace, ".noctifab", "prompts", agent, action+".tmpl")
				if _, statErr := os.Stat(path); statErr == nil {
					_, _ = fmt.Fprintf(out, "skipped %s (already exists)\n", path)
					continue
				}
				if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
					return err
				}
				if err := os.WriteFile(path, []byte(text), 0644); err != nil {
					return err
				}
				_, _ = fmt.Fprintf(out, "created %s\n", path)
			}
		}
		return nil
	},
}

var promptsValidateCmd = &cobra.Command{
	Use:           "validate",
	Short:         "Parse and test-render all effective prompt templates",
	SilenceUsage:  true,
	SilenceErrors: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		if _, err := prompts.NewRenderer(promptsWorkspace(cmd), loadPromptOverrides(cmd)); err != nil {
			return err
		}
		_, _ = fmt.Fprintln(cmd.OutOrStdout(), "All prompt templates are valid.")
		return nil
	},
}

func init() {
	promptsCmd.AddCommand(promptsListCmd)
	promptsCmd.AddCommand(promptsShowCmd)
	promptsCmd.AddCommand(promptsInitCmd)
	promptsCmd.AddCommand(promptsValidateCmd)
	RootCmd.AddCommand(promptsCmd)
}
