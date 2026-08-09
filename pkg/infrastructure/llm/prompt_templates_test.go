package llm

import (
	"strings"
	"testing"
)

func TestPromptCompaction(t *testing.T) {
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
