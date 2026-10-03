package llm

import (
	"strings"
	"testing"
)

func TestBuildJSONReminderPrompt_DefaultCaps(t *testing.T) {
	longTask := strings.Repeat("A", 3000)
	longBody := strings.Repeat("B", 20000)

	prompt := buildJSONReminderPrompt(longTask, []byte(longBody))

	// By default, task cap is 1500.
	if strings.Contains(prompt, strings.Repeat("A", 1501)) {
		t.Errorf("expected task prompt to be truncated at 1500 chars")
	}
	if !strings.Contains(prompt, "...[spec/context truncated for format reminder]...") {
		t.Errorf("expected task truncation marker")
	}
	// The rejected response body must be omitted to prevent token waste and Anthropic reasoning_extraction refusals.
	if strings.Contains(prompt, "BBBB") {
		t.Errorf("expected rejected response body to be omitted from format reminder prompt")
	}
}

func TestBuildJSONReminderPrompt_CustomCaps(t *testing.T) {
	c := &Client{
		JSONReminderTaskCap: 50,
	}

	task := strings.Repeat("X", 200)
	body := strings.Repeat("Y", 300)

	prompt := c.buildJSONReminderPrompt(task, []byte(body))

	if strings.Contains(prompt, strings.Repeat("X", 51)) {
		t.Errorf("expected task prompt to be truncated at custom cap of 50 chars")
	}
	if strings.Contains(prompt, "YYYY") {
		t.Errorf("expected rejected response body to be omitted from format reminder prompt")
	}
}
