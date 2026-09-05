package domain

import "github.com/google/uuid"

const (
	// MaxLastActions caps how many entries State.LastActions retains.
	// Older entries are dropped so the persisted state stays bounded.
	MaxLastActions = 200
	// MaxActionResultChars caps the size of an Action.Result payload.
	// The tail of the log is kept because failures are reported at the end.
	MaxActionResultChars = 4000
	// truncationMarker prefixes truncated results so readers know content was dropped.
	truncationMarker = "…[truncated]"
)

// TruncateActionResult caps result to MaxActionResultChars characters, keeping
// the tail of the log (test failures appear at the end) and prefixing the
// output with a truncation marker when content was dropped.
func TruncateActionResult(result string) string {
	if len(result) <= MaxActionResultChars {
		return result
	}
	keep := MaxActionResultChars - len(truncationMarker)
	if keep < 0 {
		keep = 0
	}
	return truncationMarker + result[len(result)-keep:]
}

// AppendAction appends action to state.LastActions, truncating the action's
// Result field and capping the slice to the most recent MaxLastActions entries.
func AppendAction(state *State, action Action) {
	if state == nil {
		return
	}
	if action.ID == "" {
		action.ID = uuid.New().String()
	}
	action.Result = TruncateActionResult(action.Result)
	state.LastActions = append(state.LastActions, action)
	if len(state.LastActions) > MaxLastActions {
		overflow := len(state.LastActions) - MaxLastActions
		state.LastActions = append([]Action(nil), state.LastActions[overflow:]...)
	}
}
