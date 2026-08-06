package domain

import (
	"fmt"
	"strings"
	"testing"
)

func TestTruncateActionResult(t *testing.T) {
	t.Run("when result is shorter than the cap, it is returned unchanged", func(t *testing.T) {
		in := "short result"
		if got := TruncateActionResult(in); got != in {
			t.Errorf("expected %q, got %q", in, got)
		}
	})

	t.Run("when result is exactly at the cap, it is returned unchanged", func(t *testing.T) {
		in := strings.Repeat("a", MaxActionResultChars)
		if got := TruncateActionResult(in); got != in {
			t.Errorf("expected unchanged result at exactly the cap")
		}
	})

	t.Run("when result exceeds the cap, it keeps the tail with a truncation marker prefix", func(t *testing.T) {
		in := strings.Repeat("x", 10000) + "FINAL_FAILURE_LINE"
		got := TruncateActionResult(in)
		if len(got) != MaxActionResultChars {
			t.Errorf("expected length %d, got %d", MaxActionResultChars, len(got))
		}
		if !strings.HasPrefix(got, "…[truncated]") {
			t.Errorf("expected truncation marker prefix, got %q", got[:30])
		}
		if !strings.HasSuffix(got, "FINAL_FAILURE_LINE") {
			t.Errorf("expected tail of the log to be preserved")
		}
	})
}

func TestAppendAction(t *testing.T) {
	t.Run("when state is nil, it does not panic", func(t *testing.T) {
		AppendAction(nil, Action{Tool: "x"})
	})

	t.Run("when appending an action, it truncates the result field", func(t *testing.T) {
		state := &State{}
		AppendAction(state, Action{Tool: "evaluate", Result: strings.Repeat("y", 20000)})
		if len(state.LastActions) != 1 {
			t.Fatalf("expected 1 action, got %d", len(state.LastActions))
		}
		if len(state.LastActions[0].Result) != MaxActionResultChars {
			t.Errorf("expected truncated result of %d chars, got %d", MaxActionResultChars, len(state.LastActions[0].Result))
		}
	})

	t.Run("when appending beyond the cap, it keeps only the most recent MaxLastActions entries", func(t *testing.T) {
		state := &State{}
		total := MaxLastActions + 50
		for i := 0; i < total; i++ {
			AppendAction(state, Action{Tool: fmt.Sprintf("tool-%d", i)})
		}
		if len(state.LastActions) != MaxLastActions {
			t.Fatalf("expected %d actions, got %d", MaxLastActions, len(state.LastActions))
		}
		if state.LastActions[0].Tool != fmt.Sprintf("tool-%d", total-MaxLastActions) {
			t.Errorf("expected oldest retained action to be tool-%d, got %s", total-MaxLastActions, state.LastActions[0].Tool)
		}
		if state.LastActions[len(state.LastActions)-1].Tool != fmt.Sprintf("tool-%d", total-1) {
			t.Errorf("expected newest action to be tool-%d, got %s", total-1, state.LastActions[len(state.LastActions)-1].Tool)
		}
	})
}
