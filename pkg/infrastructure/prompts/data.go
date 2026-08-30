package prompts

// This file defines the typed template data structs. They are the public
// customization contract: the named placeholders available to user-provided
// template overrides. Fields are only ever added, never renamed or removed.

// TaskPromptData backs the tester/* and generator/* action templates.
type TaskPromptData struct {
	// Title is the task title.
	Title string
	// Description is the detailed task description.
	Description string
	// Context is the combined context block (existing file contents,
	// inspection results, recent tests, recovery directives, and previous
	// failure summaries). Empty when there is no context; otherwise it
	// begins with a blank line so it can be appended verbatim.
	Context string
	// Feedback carries the generator's test-fix feedback (tester/fix only).
	Feedback string
	// RecentTestsContext contains the recently written test files context
	// (generator refactor/fix paths). Also included inside Context.
	RecentTestsContext string
	// RecoveryDirective is the stall-recovery directive injected by the
	// unblocker, if any. Also included inside Context.
	RecoveryDirective string
	// TargetFiles lists the relative file paths this task targets.
	TargetFiles []string
}

// ProductManagerPromptData backs the product_manager/* action templates.
type ProductManagerPromptData struct {
	// Spec is the raw SPEC.md content.
	Spec string
	// ExistingStories is the concatenated existing user stories (audit only).
	ExistingStories string
	// LegacyFiles is the pre-formatted legacy codebase context block, or
	// empty when no legacy files were detected.
	LegacyFiles string
	// MaxUserStories is the ceiling on the number of user stories to generate (0 = unlimited).
	MaxUserStories int
}

// PlannerPromptData backs the planner/* action templates.
type PlannerPromptData struct {
	// Spec is the user story / specification content to decompose.
	Spec string
}

// QAPromptData backs the qa/acceptance action template.
type QAPromptData struct {
	State              string
	StoryContract      string
	ValidationCommands []string
	MaxScenarios       int
}

// AcceptanceAuditPromptData backs the auditor/acceptance_audit action template.
type AcceptanceAuditPromptData struct {
	Spec            string
	WorkspaceFiles  string
	StoryContracts  string
	PublicContracts string
	TaskSummaries   string
}

// SpecPromptData backs the spec/* action templates.
type SpecPromptData struct {
	UserPrompt   string
	ExistingSpec string
	DraftSpec    string
	HumanHistory string
	Feedback     string
}

// FixtureData returns representative data for the given agent, used to
// test-render every effective template at startup so a broken override fails
// fast with a clear error instead of failing mid-run.
func FixtureData(agent string) any {
	switch agent {
	case AgentProductManager:
		return ProductManagerPromptData{
			Spec:            "fixture specification",
			ExistingStories: "fixture stories",
			LegacyFiles:     "",
		}
	case AgentPlanner:
		return PlannerPromptData{Spec: "fixture specification"}
	case AgentQA:
		return QAPromptData{
			State:              "fixture state snapshot",
			StoryContract:      "fixture story contract",
			ValidationCommands: []string{"./dist/example"},
			MaxScenarios:       8,
		}
	case AgentAuditor:
		return AcceptanceAuditPromptData{
			Spec:            "fixture specification",
			WorkspaceFiles:  "main.go\ncommands.go",
			StoryContracts:  "fixture story contract",
			PublicContracts: "cli.run",
			TaskSummaries:   "task-1: completed",
		}
	case AgentSpec:
		return SpecPromptData{
			UserPrompt:   "fixture prompt",
			ExistingSpec: "fixture existing spec",
			DraftSpec:    "fixture draft spec",
			HumanHistory: "fixture history",
			Feedback:     "fixture feedback",
		}
	default:
		return TaskPromptData{
			Title:       "fixture title",
			Description: "fixture description",
			Context:     "",
			Feedback:    "fixture feedback",
			TargetFiles: []string{"main.go"},
		}
	}
}
