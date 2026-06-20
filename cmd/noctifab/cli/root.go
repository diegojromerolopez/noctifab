package cli

import (
	"fmt"
	"os"

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
	// Global persistent flags
	RootCmd.PersistentFlags().StringP("config", "c", ".noctifab/config.yaml", "Path to the YAML configuration file")
	RootCmd.PersistentFlags().String("db-path", "", "Path to the local SQLite database file")
	RootCmd.PersistentFlags().String("storage-provider", "sqlite", "Storage backend provider: sqlite, postgres, mysql, json")
	RootCmd.PersistentFlags().String("storage-conn", "", "Connection string or filepath for the storage database")
	RootCmd.PersistentFlags().StringP("input", "i", "", "Path, issue URL to fetch the feature specification")
	RootCmd.PersistentFlags().Bool("auto-commit", false, "Enable automatic branch creation, conventional commit, version bump, and PR creation")
	RootCmd.PersistentFlags().IntP("agents", "a", 3, "Maximum number of parallel workers/agents to spawn")
	RootCmd.PersistentFlags().StringP("interval", "t", "5m", "Cycle loop polling duration interval")
	RootCmd.PersistentFlags().StringP("vcs-provider", "p", "github", "Version Control System target: github, gitlab")
	RootCmd.PersistentFlags().String("vcs-token", "", "API Access Token for the VCS provider")
	RootCmd.PersistentFlags().StringP("vcs-repo", "r", "", "Repository identifier format: owner/repo")
	RootCmd.PersistentFlags().StringP("llm-provider", "l", "openai", "LLM client API provider: openai, anthropic, gemini, ollama")
	RootCmd.PersistentFlags().StringP("llm-model", "m", "gpt-4o", "LLM Model Identifier")
	RootCmd.PersistentFlags().StringP("llm-api-key", "k", "", "API authentication key")
	RootCmd.PersistentFlags().StringP("llm-url", "u", "", "Custom endpoint URL")
	RootCmd.PersistentFlags().String("llm-planner-model", "", "Model override for the Planner agent")
	RootCmd.PersistentFlags().String("llm-generator-model", "", "Model override for the Generator agent")
	RootCmd.PersistentFlags().String("llm-evaluator-model", "", "Model override for the Evaluator agent")
	RootCmd.PersistentFlags().String("jira-user", "", "User email for Jira REST API authentication")
	RootCmd.PersistentFlags().String("jira-token", "", "API Token for Jira REST API authentication")
	RootCmd.PersistentFlags().String("jira-url", "", "Base URL of the Jira cloud instance")
	RootCmd.PersistentFlags().Int("http-max-retries", 10, "Maximum HTTP request retries for API clients")
	RootCmd.PersistentFlags().String("http-retry-backoff", "100ms", "Base delay time duration for exponential backoff")
	RootCmd.PersistentFlags().Int("max-tools-per-response", 5, "Maximum number of parallel tool calls allowed per response")
	RootCmd.PersistentFlags().Int("max-actions", 100, "Global action count ceiling per run session")
	RootCmd.PersistentFlags().String("max-duration", "0", "Elapsed duration run ceiling")
	RootCmd.PersistentFlags().String("conversation-mode", "sliding-window", "Conversation history tracking mode")
	RootCmd.PersistentFlags().Int("max-history-messages", 10, "Maximum number of messages kept in history")
	RootCmd.PersistentFlags().Int("compaction-threshold", 15, "Message count threshold before triggering conversation compaction")
	RootCmd.PersistentFlags().Int("max-history-tokens", 4096, "Token limit for conversation history context")
	RootCmd.PersistentFlags().String("sandbox-mode", "host", "Sandbox isolation mode: host or docker")
	RootCmd.PersistentFlags().String("shutdown-grace-period", "30s", "Delay period to wait for in-flight tasks during graceful shutdown")
	RootCmd.PersistentFlags().Int("occ-max-retries", 5, "Maximum number of reload-modify-retry iterations on version conflicts")
	RootCmd.PersistentFlags().String("occ-backoff-base", "50ms", "Base delay time duration for OCC lock retry backoff")
	RootCmd.PersistentFlags().Float64("occ-backoff-factor", 2.0, "Exponential backoff factor on OCC conflicts")
	RootCmd.PersistentFlags().Float64("max-budget-usd", 10.00, "Daily LLM credit budget boundary in USD")
	RootCmd.PersistentFlags().Int64("token-usage-limit", 0, "Daily token limit boundary")
	RootCmd.PersistentFlags().String("log-level", "info", "Logging verbosity: debug, info, warn, error")
	RootCmd.PersistentFlags().String("log-file", "", "Path to target log file")
}
