package llm

import (
	"strings"
	"testing"
)

func TestPreprocessPrompt(t *testing.T) {
	t.Run("Product Manager Agent Prompt Preprocessing", func(t *testing.T) {
		raw := "Generate detailed user stories from specification:\n\n# Spec for WC\nBuild word count utility."
		processed := preprocessPrompt(raw)

		if !strings.Contains(processed, "You are acting as the Product Manager Agent.") {
			t.Errorf("expected Product Manager agent header in prompt, got: %s", processed)
		}
		if !strings.Contains(processed, "ROADMAP CONSOLIDATION & STORY LIMIT RULE:") {
			t.Errorf("expected roadmap consolidation rule in prompt, got: %s", processed)
		}
		if !strings.Contains(processed, "DEFINITION OF DONE (DoD) & CONTRACT MANDATE:") {
			t.Errorf("expected Definition of Done mandate in prompt, got: %s", processed)
		}
		if !strings.Contains(processed, "LEGACY CODEBASE STABILIZATION & REFACTORING MANDATE:") {
			t.Errorf("expected legacy codebase stabilization mandate in prompt, got: %s", processed)
		}
		if !strings.Contains(processed, "create_story") {
			t.Errorf("expected create_story tool instruction, got: %s", processed)
		}
	})

	t.Run("Planner Agent Prompt Preprocessing", func(t *testing.T) {
		raw := "Decompose specification into tasks:\n\n# Spec\nBuild feature."
		processed := preprocessPrompt(raw)

		if !strings.Contains(processed, "You are acting as the Planner Agent.") {
			t.Errorf("expected Planner agent header in prompt, got: %s", processed)
		}
		if !strings.Contains(processed, "LEGACY CODEBASE STABILIZATION MANDATE:") {
			t.Errorf("expected legacy task planning mandate in prompt, got: %s", processed)
		}
	})

	t.Run("Tester Agent Prompt Preprocessing", func(t *testing.T) {
		raw := "Write tests for task:\n\nTask title"
		processed := preprocessPrompt(raw)

		if !strings.Contains(processed, "BLACK-BOX TESTING & DEPENDENCY INJECTION MANDATE:") {
			t.Errorf("expected black-box testing mandate in prompt, got: %s", processed)
		}
		if !strings.Contains(processed, "LEGACY STABILIZATION TESTING:") {
			t.Errorf("expected legacy stabilization testing mandate in prompt, got: %s", processed)
		}
	})

	t.Run("Generator Agent Prompt Preprocessing", func(t *testing.T) {
		raw := "Execute task:\n\nTask title"
		processed := preprocessPrompt(raw)

		if !strings.Contains(processed, "FUNCTIONAL CORRECTNESS FIRST:") {
			t.Errorf("expected functional correctness first in prompt, got: %s", processed)
		}
		if !strings.Contains(processed, "LEGACY CODE REFACTORING MANDATE:") {
			t.Errorf("expected legacy code refactoring mandate in prompt, got: %s", processed)
		}
		if !strings.Contains(processed, "GENERATOR SELF-VERIFICATION:") {
			t.Errorf("expected generator self-verification in prompt, got: %s", processed)
		}
	})

	t.Run("CompactSimpleEnglish", func(t *testing.T) {
		raw := "Please note that we utilize this component to facilitate building features."
		compacted := CompactSimpleEnglish(raw)

		if strings.Contains(compacted, "Please note that") {
			t.Errorf("expected conversational fluff to be stripped, got: %s", compacted)
		}
		if !strings.Contains(compacted, "use") || !strings.Contains(compacted, "help") {
			t.Errorf("expected vocabulary simplification, got: %s", compacted)
		}
	})

	t.Run("CompactCaveman", func(t *testing.T) {
		raw := "Please note that this is a feature.\n---\n```go\nfmt.Println(\"hello\")\n```\nIn order to ensure that it works."
		compacted := CompactCaveman(raw)

		if strings.Contains(compacted, "Please note that") {
			t.Errorf("expected conversational fluff to be stripped, got: %s", compacted)
		}
		if strings.Contains(compacted, "---") {
			t.Errorf("expected decorative dividers to be stripped, got: %s", compacted)
		}
		if !strings.Contains(compacted, "fmt.Println(\"hello\")") {
			t.Errorf("expected code block to be preserved, got: %s", compacted)
		}
	})

	t.Run("ParallelCompactionLargeInput", func(t *testing.T) {
		var sb strings.Builder
		sb.WriteString("Please note that we utilize this feature.\n")
		// Build input exceeding parallelCompactionThreshold (20,000 bytes)
		chunk := "Please note that this is line for testing. Utilize facilitate demonstrate commence terminate.\n"
		for sb.Len() < 25000 {
			sb.WriteString(chunk)
		}
		sb.WriteString("```go\nfunc utilizeCode() {}\n```\n")
		sb.WriteString("In order to ensure that final line works.\n")

		raw := sb.String()
		if len(raw) < 20000 {
			t.Fatalf("expected prompt > 20000 bytes, got %d", len(raw))
		}

		compactedSimple := CompactSimpleEnglish(raw)
		if strings.Contains(compactedSimple, "Please note that") {
			t.Errorf("expected fluff stripped in parallel CompactSimpleEnglish")
		}
		if strings.Contains(compactedSimple, "use this feature") == false {
			t.Errorf("expected vocabulary replaced in parallel CompactSimpleEnglish")
		}
		if !strings.Contains(compactedSimple, "func utilizeCode() {}") {
			t.Errorf("expected code block preserved in parallel CompactSimpleEnglish")
		}

		compactedCaveman := CompactCaveman(raw)
		if strings.Contains(compactedCaveman, "Please note that") {
			t.Errorf("expected fluff stripped in parallel CompactCaveman")
		}
		if !strings.Contains(compactedCaveman, "func utilizeCode() {}") {
			t.Errorf("expected code block preserved in parallel CompactCaveman")
		}
	})
}
