package services

import (
	"strings"
	"testing"

	"github.com/diegojromerolopez/noctifab/pkg/domain"
)

// These tests pin the uncompactable-tail invariants of the hardcoded prompts:
// the prompt must END with its static schema tail, so that
// domain.WithUncompactableTail(ctx, len(tail)) protects exactly the JSON
// output schema (and tool list) from prompt compaction.

func TestHardcodedPromptTails(t *testing.T) {
	t.Run("when wrapping a repair prompt it ends with the repair tail", func(t *testing.T) {
		got := wrapRepairPrompt("some diagnostic details")
		if !strings.HasSuffix(got, repairPromptTail) {
			t.Error("repair prompt must end with repairPromptTail")
		}
		if !strings.Contains(got, "Task Details & Failure Context:\nsome diagnostic details\n\nCRITICAL:") {
			t.Error("details must sit between head and tail exactly as in the legacy assembly")
		}
		for _, needle := range []string{"You are acting as the Repair Agent.", "Return format", `"reasoning"`} {
			if !strings.Contains(got, needle) {
				t.Errorf("repair prompt missing %q", needle)
			}
		}
	})

	t.Run("when building an unblocker prompt it ends with the unblocker tail", func(t *testing.T) {
		state := &domain.State{StoryStatus: domain.StoryRunning, BuildStatus: domain.BuildUnknown}
		got := buildUnblockerPrompt(state, nil)
		if !strings.HasSuffix(got, unblockerPromptTail) {
			t.Error("unblocker prompt must end with unblockerPromptTail")
		}
		if !strings.Contains(got, "Stalled Tasks/Agents Detected (0 stall(s)):") {
			t.Error("unblocker prompt missing stall header")
		}
		if !strings.Contains(unblockerPromptTail, `"reset_task"`) {
			t.Error("unblocker tail must carry the corrective-action schema")
		}
	})

	t.Run("when inspecting the reader tail it carries the tool list and schema", func(t *testing.T) {
		for _, needle := range []string{"read_file", "list_directory", "find_files", "grep_search", "noop", "Return format", `"reasoning"`} {
			if !strings.Contains(readerPromptTail, needle) {
				t.Errorf("reader tail missing %q", needle)
			}
		}
	})
}
