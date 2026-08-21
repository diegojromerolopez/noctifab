package cli

import (
	"fmt"
	"os"

	"github.com/diegojromerolopez/noctifab/pkg/version"
	"github.com/spf13/cobra"
)

type ExitError struct {
	Code int
	Msg  string
}

func (e *ExitError) Error() string {
	return e.Msg
}

var OsExit = os.Exit

var RootCmd = &cobra.Command{
	Use:           "noctifab",
	Short:         "noctifab is an autonomous software development orchestrator",
	Long:          "A dark factory platform for GitHub, GitLab, and BitBucket that coordinates autonomous agents.",
	SilenceErrors: true,
	SilenceUsage:  true,
	RunE: func(cmd *cobra.Command, args []string) error {
		interactive, _ := cmd.Flags().GetBool("interactive")
		if interactive {
			return dashboardCmd.RunE(cmd, args)
		}
		return cmd.Help()
	},
}

func Execute() {
	if err := RootCmd.Execute(); err != nil {
		if exitErr, ok := err.(*ExitError); ok {
			if exitErr.Msg != "" {
				fmt.Fprintln(os.Stderr, exitErr.Msg)
			}
			OsExit(exitErr.Code)
			return
		}
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		OsExit(1)
		return
	}
}

func init() {
	RootCmd.Version = version.GetInfo().String()
	RootCmd.SetVersionTemplate("{{.Version}}\n")

	// Global persistent flags
	RootCmd.PersistentFlags().StringP("config", "c", ".noctifab/config.yaml", "Path to the YAML configuration file")
	RootCmd.PersistentFlags().String("db-path", "", "Path to the local SQLite database file")
	RootCmd.PersistentFlags().String("storage-provider", "sqlite", "Storage backend provider: sqlite, postgres, mysql, json")
	RootCmd.PersistentFlags().String("storage-conn", "", "Connection string or filepath for the storage database")
	RootCmd.PersistentFlags().String("spec-source", "", "Path or issue URL to fetch the feature specification")
	RootCmd.PersistentFlags().BoolP("interactive", "i", false, "Launch in live interactive TUI dashboard mode")
	RootCmd.PersistentFlags().IntP("agents", "a", 3, "Maximum number of parallel workers/agents to spawn")
	RootCmd.PersistentFlags().StringP("interval", "t", "5m", "Cycle loop polling duration interval")
	RootCmd.PersistentFlags().StringP("vcs-provider", "p", "github", "Version Control System target: github, gitlab")
	RootCmd.PersistentFlags().StringP("vcs-repo", "r", "", "Repository identifier format: owner/repo")
	RootCmd.PersistentFlags().StringP("llm-provider", "l", "openai", "LLM client API provider: openai, anthropic, gemini, ollama")
	RootCmd.PersistentFlags().StringP("llm-model", "m", "gpt-4o", "LLM Model Identifier")
	RootCmd.PersistentFlags().StringP("llm-url", "u", "", "Custom endpoint URL")
	RootCmd.PersistentFlags().String("llm-planner-model", "", "Model override for the Planner agent")
	RootCmd.PersistentFlags().String("llm-generator-model", "", "Model override for the Generator agent")
	RootCmd.PersistentFlags().String("llm-tester-model", "", "Model override for the Tester agent")
	RootCmd.PersistentFlags().Int("http-max-retries", 10, "Maximum HTTP request retries for API clients")
	RootCmd.PersistentFlags().String("http-retry-backoff", "100ms", "Base delay time duration for exponential backoff")
	RootCmd.PersistentFlags().Int("max-actions", 100, "Global action count ceiling per run session")
	RootCmd.PersistentFlags().String("max-duration", "0", "Elapsed duration run ceiling")
	RootCmd.PersistentFlags().String("sandbox-mode", "host", "Sandbox isolation mode: host or docker")
	RootCmd.PersistentFlags().Int("occ-max-retries", 5, "Maximum number of reload-modify-retry iterations on version conflicts")
	RootCmd.PersistentFlags().String("occ-backoff-base", "50ms", "Base delay time duration for OCC lock retry backoff")
	RootCmd.PersistentFlags().Float64("occ-backoff-factor", 2.0, "Exponential backoff factor on OCC conflicts")
	RootCmd.PersistentFlags().Int64("token-usage-limit", 0, "Daily token limit boundary")
	RootCmd.PersistentFlags().Bool("pr-auto-create", false, "Automatically create a PR from the task branch")
	RootCmd.PersistentFlags().Bool("pr-auto-merge", false, "Automatically merge the PR when CI checks pass")
	RootCmd.PersistentFlags().Bool("pr-auto-rebase", false, "Automatically rebase the PR branch on base updates")
	RootCmd.PersistentFlags().Bool("pr-draft", false, "Create the PR as a draft")
	RootCmd.PersistentFlags().String("pr-assignees", "", "Comma-separated GitHub usernames to assign to the PR")
	RootCmd.PersistentFlags().String("pr-labels", "", "Comma-separated labels to apply to the PR")
	RootCmd.PersistentFlags().String("log-level", "info", "Logging verbosity: debug, info, warn, error")
	RootCmd.PersistentFlags().String("log-file", "", "Path to target log file")
}
