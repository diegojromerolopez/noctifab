package services

import (
	"strings"
	"testing"
)

func TestCapText(t *testing.T) {
	t.Run("when text is shorter than the cap, it is returned unchanged", func(t *testing.T) {
		if got := capText("hello", 100); got != "hello" {
			t.Errorf("expected unchanged text, got %q", got)
		}
	})

	t.Run("when max is zero or negative, it is returned unchanged", func(t *testing.T) {
		if got := capText("hello", 0); got != "hello" {
			t.Errorf("expected unchanged text with max=0, got %q", got)
		}
		if got := capText("hello", -1); got != "hello" {
			t.Errorf("expected unchanged text with max=-1, got %q", got)
		}
	})

	t.Run("when text exceeds the cap, it keeps head and tail halves with a truncation marker", func(t *testing.T) {
		head := strings.Repeat("H", 5000)
		tail := strings.Repeat("T", 5000)
		in := head + tail
		got := capText(in, 1000)

		if !strings.Contains(got, "...[truncated") || !strings.Contains(got, "chars]...") {
			t.Errorf("expected truncation marker in output, got %q", got)
		}
		if !strings.HasPrefix(got, "HHHH") {
			t.Error("expected head of the text to be preserved")
		}
		if !strings.HasSuffix(got, "TTTT") {
			t.Error("expected tail of the text to be preserved")
		}
		if len(got) > 1000+10 {
			t.Errorf("expected capped output to be about 1000 chars, got %d", len(got))
		}
	})

	t.Run("when text exceeds the cap, the marker reports the number of dropped chars", func(t *testing.T) {
		in := strings.Repeat("x", 9000)
		got := capText(in, 1000)
		if !strings.Contains(got, "...[truncated 8000 chars]...") {
			t.Errorf("expected marker with 8000 dropped chars, got %q", got)
		}
	})
}

func TestCapStrings(t *testing.T) {
	t.Run("when capping a slice, each element is capped independently", func(t *testing.T) {
		items := []string{"short", strings.Repeat("y", 5000)}
		got := capStrings(items, 1000)
		if got[0] != "short" {
			t.Errorf("expected short element unchanged, got %q", got[0])
		}
		if len(got[1]) > 1010 {
			t.Errorf("expected long element capped, got length %d", len(got[1]))
		}
	})

	t.Run("when capping, the original slice is not mutated", func(t *testing.T) {
		long := strings.Repeat("z", 5000)
		items := []string{long}
		_ = capStrings(items, 100)
		if items[0] != long {
			t.Error("expected original slice to remain unmutated")
		}
	})
}

func TestJoinCappedToolOutputs(t *testing.T) {
	t.Run("when joining outputs, each is capped to the tool output limit", func(t *testing.T) {
		outputs := []string{strings.Repeat("a", toolOutputCapChars*2), "small"}
		got := joinCappedToolOutputs(outputs)
		if !strings.Contains(got, "...[truncated") {
			t.Error("expected oversized tool output to be truncated")
		}
		if !strings.Contains(got, "\n---\n") {
			t.Error("expected outputs to be joined with separators")
		}
		if !strings.Contains(got, "small") {
			t.Error("expected small output to be preserved verbatim")
		}
	})
}

func TestIterationsOrDefault(t *testing.T) {
	t.Run("when the configured value is positive, it is used", func(t *testing.T) {
		if got := iterationsOrDefault(7); got != 7 {
			t.Errorf("expected 7, got %d", got)
		}
	})

	t.Run("when the configured value is zero or negative, it defaults to 20", func(t *testing.T) {
		if got := iterationsOrDefault(0); got != 20 {
			t.Errorf("expected default 20 for 0, got %d", got)
		}
		if got := iterationsOrDefault(-3); got != 20 {
			t.Errorf("expected default 20 for -3, got %d", got)
		}
	})
}

func TestPruneMutatedFileContexts(t *testing.T) {
	prompt := `Existing files context:
File src/main.go:
` + "```" + `
package main

func main() {
    println("hello")
}
` + "```" + `

File src/util.go (Diff Window +/- 5 lines):
` + "```diff" + `
+ func Helper() bool { return true }
` + "```" + `

File src/unmodified.go:
` + "```" + `
package main
var Version = "1.0"
` + "```" + `

Some other instructions here.`

	t.Run("when mutatedFiles is empty, prompt is returned unchanged", func(t *testing.T) {
		got := pruneMutatedFileContexts(prompt, nil)
		if got != prompt {
			t.Errorf("expected prompt unchanged with nil map")
		}
		got = pruneMutatedFileContexts(prompt, map[string]bool{})
		if got != prompt {
			t.Errorf("expected prompt unchanged with empty map")
		}
	})

	t.Run("when a file was mutated, its content block is replaced with omission marker", func(t *testing.T) {
		mutated := map[string]bool{"src/main.go": true}
		got := pruneMutatedFileContexts(prompt, mutated)
		if strings.Contains(got, "println(\"hello\")") {
			t.Errorf("expected src/main.go content to be omitted, but found it in output:\n%s", got)
		}
		if !strings.Contains(got, "File src/main.go: [Contents omitted: modified in earlier turn of this task. Use read_file to inspect latest content if needed.]") {
			t.Errorf("expected omission marker for src/main.go, got:\n%s", got)
		}
		if !strings.Contains(got, "Version = \"1.0\"") {
			t.Errorf("expected unmodified file content to remain intact")
		}
	})

	t.Run("when mutated file matches with relative ./ prefix, it is still pruned", func(t *testing.T) {
		mutated := map[string]bool{"./src/main.go": true}
		got := pruneMutatedFileContexts(prompt, mutated)
		if strings.Contains(got, "println(\"hello\")") {
			t.Errorf("expected ./src/main.go to match src/main.go and omit content")
		}
	})

	t.Run("when file has diff window header, it is pruned correctly", func(t *testing.T) {
		mutated := map[string]bool{"src/util.go": true}
		got := pruneMutatedFileContexts(prompt, mutated)
		if strings.Contains(got, "func Helper()") {
			t.Errorf("expected src/util.go diff window content to be omitted")
		}
		if !strings.Contains(got, "File src/util.go: [Contents omitted: modified in earlier turn of this task. Use read_file to inspect latest content if needed.]") {
			t.Errorf("expected omission marker for src/util.go, got:\n%s", got)
		}
	})

	t.Run("when file is not present in prompt, prompt remains unchanged", func(t *testing.T) {
		mutated := map[string]bool{"src/nonexistent.go": true}
		got := pruneMutatedFileContexts(prompt, mutated)
		if got != prompt {
			t.Errorf("expected prompt unchanged for nonexistent mutated file")
		}
	})
}
