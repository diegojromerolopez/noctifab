package llm

import (
	"strings"
	"testing"
)

func TestBuildJSONReminderPrompt_DefaultCaps(t *testing.T) {
	longTask := strings.Repeat("A", 3000)
	longBody := strings.Repeat("B", 20000)

	prompt := buildJSONReminderPrompt(longTask, []byte(longBody))

	// By default, task cap is 1500 and body cap is 12000.
	if strings.Contains(prompt, strings.Repeat("A", 1501)) {
		t.Errorf("expected task prompt to be truncated at 1500 chars")
	}
	if !strings.Contains(prompt, "...[spec/context truncated for format reminder]...") {
		t.Errorf("expected task truncation marker")
	}
	if strings.Contains(prompt, strings.Repeat("B", 12001)) {
		t.Errorf("expected response body to be truncated at 12000 chars")
	}
	if !strings.Contains(prompt, "...[truncated]...\n") {
		t.Errorf("expected body truncation marker")
	}
}

func TestBuildJSONReminderPrompt_CustomCaps(t *testing.T) {
	c := &Client{
		JSONReminderTaskCap: 50,
		JSONReminderBodyCap: 100,
	}

	task := strings.Repeat("X", 200)
	body := strings.Repeat("Y", 300)

	prompt := c.buildJSONReminderPrompt(task, []byte(body))

	if strings.Contains(prompt, strings.Repeat("X", 51)) {
		t.Errorf("expected task prompt to be truncated at custom cap of 50 chars")
	}
	if strings.Contains(prompt, strings.Repeat("Y", 101)) {
		t.Errorf("expected body to be truncated at custom cap of 100 chars")
	}
}
