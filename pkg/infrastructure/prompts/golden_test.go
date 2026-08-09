package prompts

import (
	"strings"
	"testing"
)

// legacyRaw builds the exact raw prompt strings the services used to send
// through the legacy prefix-dispatch pipeline.
func legacyTaskRaw(prefix, instruction, title, description, context string) string {
	return prefix + " " + title + " - " + description + "\n\n" + instruction + context
}

func TestGoldenDefaults_ByteIdenticalToLegacyAssembly(t *testing.T) {
	r := NewDefaultRenderer()

	title := "Implement parser"
	description := "Parse the input file into an AST."
	context := "\n\nExisting files context:\nFile main.go:\n```\npackage main\n```"
	spec := "# Spec\nBuild a word count utility.\n"
	legacyBlock := "\n\nExisting Legacy Code Files Detected in Workspace:\n- main.go\n\nLEGACY STABILIZATION MANDATE: Code already exists in the project workspace. Assume it is legacy code with existing functionality. The primary initial goal is to stabilize it by creating unit and integration characterization tests for existing parts in US-001, and leveraging those tests as safety rails when refactoring the code to match future user story requirements."
	stories := "=== File: roadmap/US-001.md ===\n# US-001\n"
	feedback := "The assertion on line 42 expects the wrong exit code."

	cases := []struct {
		name   string
		agent  string
		action string
		data   any
		legacy string // raw prompt fed to the legacy prefix pipeline
	}{
		{
			name: "product_manager/generate", agent: AgentProductManager, action: "generate",
			data:   ProductManagerPromptData{Spec: spec, LegacyFiles: legacyBlock},
			legacy: "Generate detailed user stories from specification:\n\n" + spec + legacyBlock,
		},
		{
			name: "product_manager/audit", agent: AgentProductManager, action: "audit",
			data:   ProductManagerPromptData{Spec: spec, ExistingStories: stories, LegacyFiles: ""},
			legacy: "Audit and refine existing user stories to ensure complete Definition of Done (DoD), edge cases, and interface contracts:\n\nSpecification:\n" + spec + "\n\nExisting User Stories:\n" + stories,
		},
		{
			name: "planner/decompose", agent: AgentPlanner, action: "decompose",
			data:   PlannerPromptData{Spec: spec},
			legacy: "Decompose specification into tasks:\n\n" + spec,
		},
		{
			name: "tester/write", agent: AgentTester, action: "write",
			data:   TaskPromptData{Title: title, Description: description, Context: context},
			legacy: legacyTaskRaw("Write tests for task:", "The minimal implementation has already been created. Write tests to verify this implementation, including unit and integration tests as specified in the guidelines.", title, description, context),
		},
		{
			name: "tester/fix", agent: AgentTester, action: "fix",
			data:   TaskPromptData{Title: title, Description: description, Context: context, Feedback: feedback},
			legacy: legacyTaskRaw("Fix the tests for task:", "Feedback from generator agent:\n"+feedback+"\n\nCorrect the test files to resolve this issue.", title, description, context),
		},
		{
			name: "generator/implement", agent: AgentGenerator, action: "implement",
			data:   TaskPromptData{Title: title, Description: description, Context: context},
			legacy: legacyTaskRaw("Execute task:", "Focus on creating the minimal implementation/functionality to fulfill the task requirements. The tests will be written in a later phase.", title, description, context),
		},
		{
			name: "generator/refactor", agent: AgentGenerator, action: "refactor",
			data:   TaskPromptData{Title: title, Description: description, Context: context},
			legacy: legacyTaskRaw("Execute task:", "Refactor the implementation to make the code better and ensure it passes all tests. You may update both the implementation files and the test files if needed.", title, description, context),
		},
		{
			name: "generator/fix", agent: AgentGenerator, action: "fix",
			data:   TaskPromptData{Title: title, Description: description, Context: context},
			legacy: legacyTaskRaw("Execute task:", "Refactor and fix the implementation to resolve the previous failures and ensure all tests pass.", title, description, context),
		},
		{
			name: "generator/single_pass", agent: AgentGenerator, action: "single_pass",
			data:   TaskPromptData{Title: title, Description: description, Context: context},
			legacy: legacyTaskRaw("Execute task:", "Implement the feature AND write corresponding unit/integration tests immediately in a single pass. Ensure both code and tests are created together.", title, description, context),
		},
		{
			name: "generator/single_pass_fix", agent: AgentGenerator, action: "single_pass_fix",
			data:   TaskPromptData{Title: title, Description: description, Context: context},
			legacy: legacyTaskRaw("Execute task:", "Fix both the implementation and the tests to resolve previous failures and ensure all tests pass. MANDATE: If an error or dependency issue persists, force a working solution (even if simplified) so the code compiles cleanly and tests pass. Leaving non-compiling code is unacceptable.", title, description, context),
		},
	}

	for _, tc := range cases {
		t.Run("when rendering the default "+tc.name+" template it matches the legacy assembly byte for byte", func(t *testing.T) {
			got, err := r.Render(tc.agent, tc.action, tc.data)
			if err != nil {
				t.Fatalf("Render failed: %v", err)
			}
			want := legacyPreprocessPrompt(tc.legacy)
			if got != want {
				t.Fatalf("rendered prompt differs from legacy assembly.\n--- got:\n%s\n--- want:\n%s\n--- first divergence at byte %d", got, want, firstDiff(got, want))
			}
		})
	}
}

// TestGoldenFixedVariants asserts the NEW, corrected assembly for the 4
// variants that previously bypassed their role bodies via the prefix-dispatch
// bug (CUSTOM_PROMPTS.md §1.1). Their instruction text is preserved and the
// full role body + output contract is now present.
func TestGoldenFixedVariants_ReceiveRoleBodyAndContract(t *testing.T) {
	r := NewDefaultRenderer()
	data := TaskPromptData{Title: "T", Description: "D", Context: ""}

	cases := []struct {
		agent, action, persona, instruction string
	}{
		{AgentTester, "write_breadth_first", "You are acting as the Tester Agent.", "Write baseline acceptance tests verifying the primary happy-path scenarios for the core functionality."},
		{AgentGenerator, "breadth_first", "You are acting as the Generator Agent.", "Focus on implementing the ~80% core happy-path functionality to make the feature functional end-to-end."},
		{AgentGenerator, "breadth_first_fix", "You are acting as the Generator Agent.", "Address previous execution failures or missing edge cases. Ensure all existing happy-path tests continue to pass with zero regressions."},
		{AgentTester, "refactor", "You are acting as the Tester Agent.", "Refactor and fix the tests to resolve the previous failures and align them with the updated code."},
	}
	for _, tc := range cases {
		t.Run("when rendering "+tc.agent+"/"+tc.action+" it contains persona, instruction and contract", func(t *testing.T) {
			got, err := r.Render(tc.agent, tc.action, data)
			if err != nil {
				t.Fatalf("Render failed: %v", err)
			}
			for _, needle := range []string{tc.persona, tc.instruction, "ANTI-STALLING MANDATE:", "Return format", `"reasoning"`} {
				if !strings.Contains(got, needle) {
					t.Errorf("rendered prompt missing %q", needle)
				}
			}
			if !strings.HasSuffix(got, Contract(tc.agent)) {
				t.Errorf("rendered prompt does not end with the %s output contract", tc.agent)
			}
		})
	}
}

func firstDiff(a, b string) int {
	n := len(a)
	if len(b) < n {
		n = len(b)
	}
	for i := 0; i < n; i++ {
		if a[i] != b[i] {
			return i
		}
	}
	return n
}
