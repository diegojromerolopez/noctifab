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
	RootCmd.AddCommand(startCmd)
}
