package services

// IntentKind describes the category of a parsed operator command.
type IntentKind string

const (
	// IntentKindStartStory indicates the operator wants to start working on a single user story file.
	IntentKindStartStory IntentKind = "START_STORY"
	// IntentKindStartDirectory indicates the operator wants to process all user stories in a directory.
	IntentKindStartDirectory IntentKind = "START_DIRECTORY"
	// IntentKindListStatus requests a summary of the current orchestrator state.
	IntentKindListStatus IntentKind = "LIST_STATUS"
	// IntentKindUnknown is returned when the input cannot be mapped to a known action.
	IntentKindUnknown IntentKind = "UNKNOWN"
)

// Intent represents a parsed operator intention derived from a raw text command.
type Intent struct {
	// Kind classifies what action the operator wants to perform.
	Kind IntentKind `json:"kind"`
	// Path is the absolute or resolved file/directory path associated with START_STORY and START_DIRECTORY intents.
	Path string `json:"path,omitempty"`
	// Message contains either an explanation for UNKNOWN intents or a status summary for LIST_STATUS intents.
	Message string `json:"message,omitempty"`
}

// LLMIntentResponse is the structured JSON the LLM is expected to return for listener parsing.
type LLMIntentResponse struct {
	Kind    string `json:"kind"`
	Path    string `json:"path,omitempty"`
	Message string `json:"message,omitempty"`
}

// ParseIntentFromLLMResponse maps the raw LLM JSON response to an Intent struct.
// Unrecognised kind values fall back to IntentKindUnknown.
func ParseIntentFromLLMResponse(resp LLMIntentResponse) Intent {
	switch IntentKind(resp.Kind) {
	case IntentKindStartStory:
		return Intent{Kind: IntentKindStartStory, Path: resp.Path}
	case IntentKindStartDirectory:
		return Intent{Kind: IntentKindStartDirectory, Path: resp.Path}
	case IntentKindListStatus:
		return Intent{Kind: IntentKindListStatus, Message: resp.Message}
	default:
		msg := resp.Message
		if msg == "" {
			msg = "I did not understand your request. Try: start <path/to/story.md> or start <path/to/dir>"
		}
		return Intent{Kind: IntentKindUnknown, Message: msg}
	}
}
