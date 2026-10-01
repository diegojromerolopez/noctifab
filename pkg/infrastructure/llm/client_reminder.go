package llm

import (
	"fmt"
	"strings"
)

// Default caps for format-reminder prompts.
const jsonReminderTaskCap = 1500
const jsonReminderBodyCap = 12000

// buildJSONReminderPrompt returns a single user-message prompt that re-states
// the JSON envelope demand and includes a truncated tail of the model's
// previous non-JSON answer. The original prompt is capped to a task summary
// to prevent re-transmitting 50-100 KB of spec context on format pullbacks.
func (c *Client) buildJSONReminderPrompt(originalPrompt string, prevBody []byte) string {
	taskCap := jsonReminderTaskCap
	if c != nil && c.JSONReminderTaskCap > 0 {
		taskCap = c.JSONReminderTaskCap
	}
	bodyCap := jsonReminderBodyCap
	if c != nil && c.JSONReminderBodyCap > 0 {
		bodyCap = c.JSONReminderBodyCap
	}

	taskSummary := strings.TrimSpace(originalPrompt)
	if len(taskSummary) > taskCap {
		taskSummary = taskSummary[:taskCap] + "\n...[spec/context truncated for format reminder]..."
	}
	tail := string(prevBody)
	if len(tail) > bodyCap {
		tail = "...[truncated]...\n" + tail[len(tail)-bodyCap:]
	}
	return fmt.Sprintf(`Your previous response did NOT contain the structured JSON envelope that this system requires. The system cannot continue without a single valid JSON object.

Original task:
%s

Your previous (rejected) response:
%s

CRITICAL INSTRUCTION (overrides anything above):
Respond with ONLY a single JSON object matching this schema. Ensure the JSON starts with an opening brace '{' and ends with a closing brace '}'. No markdown, no code fences, no prose before or after the JSON. Keys and string values must use double quotes.

Schema:
{
  "reasoning": "your reasoning",
  "actions": [
    { "tool": "write_file", "args": { "path": "...", "content": "..." } }
  ]
}

Return the valid JSON block now and nothing else.`, taskSummary, tail)
}

// buildJSONReminderPrompt is a package-level helper that delegates to (*Client).buildJSONReminderPrompt
// using default caps when called without an active client instance.
func buildJSONReminderPrompt(originalPrompt string, prevBody []byte) string {
	var c *Client
	return c.buildJSONReminderPrompt(originalPrompt, prevBody)
}
