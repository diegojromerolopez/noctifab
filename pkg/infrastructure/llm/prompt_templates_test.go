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
	})

	t.Run("Tester Agent Prompt Preprocessing", func(t *testing.T) {
		raw := "Write tests for task:\n\nTask title"
		processed := preprocessPrompt(raw)

		if !strings.Contains(processed, "BLACK-BOX TESTING & DEPENDENCY INJECTION MANDATE:") {
			t.Errorf("expected black-box testing mandate in prompt, got: %s", processed)
		}
	})

	t.Run("Generator Agent Prompt Preprocessing", func(t *testing.T) {
		raw := "Execute task:\n\nTask title"
		processed := preprocessPrompt(raw)

		if !strings.Contains(processed, "FUNCTIONAL CORRECTNESS FIRST:") {
			t.Errorf("expected functional correctness first in prompt, got: %s", processed)
		}
		if !strings.Contains(processed, "GENERATOR SELF-VERIFICATION:") {
			t.Errorf("expected generator self-verification in prompt, got: %s", processed)
		}
	})

	t.Run("CompactMarkdownSpec", func(t *testing.T) {
		raw := "Please note that this is a feature.\n---\n```go\nfmt.Println(\"hello\")\n```\nIn order to ensure that it works."
		compacted := CompactMarkdownSpec(raw)

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
}
