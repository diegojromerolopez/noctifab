package services

import (
	"fmt"
	"strings"
)

const (
	// toolOutputCapChars caps each tool output embedded into the next agent
	// turn's prompt.
	toolOutputCapChars = 8000
	// fileContextCapChars caps each file content embedded into a task prompt.
	fileContextCapChars = 16000
	// defaultAgentIterations is the fallback number of agent loop turns when
	// no per-role iterations value is configured.
	defaultAgentIterations = 10
)

// capText caps s to approximately max characters, keeping the head and tail
// halves with a "...[truncated N chars]..." marker in the middle. Failure
// details usually live at the end of logs while context lives at the start, so
// both ends are preserved.
func capText(s string, max int) string {
	if max <= 0 || len(s) <= max {
		return s
	}
	marker := fmt.Sprintf("\n...[truncated %d chars]...\n", len(s)-max)
	budget := max - len(marker)
	if budget <= 0 {
		// Degenerate cap: return only the marker-sized tail.
		return s[len(s)-max:]
	}
	head := budget / 2
	tail := budget - head
	return s[:head] + marker + s[len(s)-tail:]
}

// capStrings returns a copy of items with each element capped to max chars.
func capStrings(items []string, max int) []string {
	out := make([]string, len(items))
	for i, s := range items {
		out[i] = capText(s, max)
	}
	return out
}

// joinCappedToolOutputs joins per-tool outputs for prompt embedding, capping
// each output individually.
func joinCappedToolOutputs(outputs []string) string {
	return strings.Join(capStrings(outputs, toolOutputCapChars), "\n---\n")
}

// iterationsOrDefault returns the configured per-role iterations value, or the
// default (10) when the configured value is zero or negative.
func iterationsOrDefault(v int) int {
	if v <= 0 {
		return defaultAgentIterations
	}
	return v
}
