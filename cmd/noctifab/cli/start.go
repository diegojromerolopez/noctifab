package cli

import (
	"strings"

	"github.com/spf13/cobra"

	"github.com/diegojromerolopez/noctifab/pkg/domain"
)

const (
	daemonPIDFile = ".noctifab/noctifab.pid"
	daemonLogFile = ".noctifab/logs/daemon.log"
)

var startCmd = &cobra.Command{
	Use:           "start [target_path]",
	Short:         "Plan and execute a software specification end-to-end",
	SilenceErrors: true,
	SilenceUsage:  true,
	RunE:          runStartCommand,
}

func allTasksFinished(state *domain.State) bool {
	if state == nil || len(state.Tasks) == 0 {
		return false
	}
	for _, t := range state.Tasks {
		if t.Status != domain.TaskSuccess && t.Status != domain.TaskFailed {
			return false
		}
	}
	return true
}

func isTemplateSpec(content string) bool {
	return strings.Contains(content, "Specification: New Project")
}

func isTemplateStory(content string) bool {
	return strings.Contains(content, "User Story: US-001 - Initial Feature Specification")
}

func init() {
	startCmd.Flags().StringP("spec", "s", "SPEC.md", "Path to feature specification file")
	startCmd.Flags().Bool("resume", false, "Resume execution from the first incomplete user story, skipping completed stories")
	startCmd.Flags().BoolP("web", "w", false, "Launch the real-time visual web dashboard concurrently during execution")
	startCmd.Flags().Int("web-port", 8080, "Port for the concurrent visual web dashboard")
	startCmd.Flags().String("web-host", "127.0.0.1", "Host address to bind the concurrent visual web dashboard")
	startCmd.Flags().Bool("web-open", false, "Automatically open the visual web dashboard in the default browser")
	startCmd.Flags().Bool("standby", false, "Keep daemon alive in standby mode after finishing initial stories to accept prompt orders")
	RootCmd.AddCommand(startCmd)
}
