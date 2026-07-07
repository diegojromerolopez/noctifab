package llm

import (
	"strings"
	"testing"

	"github.com/diegojromerolopez/noctifab/pkg/domain"
)

func TestExtractJSONBlock(t *testing.T) {
	t.Run("simple envelope", func(t *testing.T) {
		in := `{"reasoning":"ok","actions":[]}`
		got, err := ExtractJSONBlock(in)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got != in {
			t.Errorf("got %q, want %q", got, in)
		}
	})

	t.Run("braces inside content string are not counted", func(t *testing.T) {
		// Rust code embedded in the content field contains unbalanced-looking
		// braces; the naive brace counter used to return a truncated substring
		// ending at a '}' inside the content string.
		content := "mod tests {\n    use super::*;\n    fn test_foo() {\n        assert!(true);\n    }\n}\n"
		in := `{"reasoning":"writing tests","actions":[{"tool":"write_file","args":{"path":"tests/unit.rs","content":"` + content + `"}}]}`
		got, err := ExtractJSONBlock(in)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got != in {
			t.Errorf("got %q, want %q", got, in)
		}
	})

	t.Run("code fence before JSON envelope prefers the envelope", func(t *testing.T) {
		rustBlock := "```rust\nmod tests {\n    fn test_x() {\n        assert!(1 == 1);\n    }\n}\n```\n"
		envelope := `{"reasoning":"I wrote the tests","actions":[{"tool":"noop","args":{}}]}`
		in := rustBlock + envelope
		got, err := ExtractJSONBlock(in)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got != envelope {
			t.Errorf("expected the JSON envelope, got %q", got)
		}
	})

	t.Run("multiple blocks prefers last with reasoning key", func(t *testing.T) {
		in := `{"foo":"bar"}` + "\n" + `{"reasoning":"second","actions":[]}`
		got, err := ExtractJSONBlock(in)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !strings.Contains(got, `"reasoning"`) {
			t.Errorf("expected the envelope block, got %q", got)
		}
	})

	t.Run("no braces returns error", func(t *testing.T) {
		_, err := ExtractJSONBlock("no json here")
		if err == nil {
			t.Fatal("expected error, got nil")
		}
	})

	t.Run("raw newlines inside content string preserved for escape step", func(t *testing.T) {
		content := "line one\nline two {\n  inner\n}\n"
		in := "{\"reasoning\":\"ok\",\"actions\":[{\"tool\":\"write_file\",\"args\":{\"path\":\"a.rs\",\"content\":\"" + content + "\"}}]}"
		got, err := ExtractJSONBlock(in)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !strings.Contains(got, "inner") {
			t.Errorf("expected inner content preserved, got %q", got)
		}
		// Sanity: the extracted block should still unmarshal after escaping.
		resp, unmarshalErr := LenientUnmarshal(got)
		if unmarshalErr != nil {
			t.Fatalf("LenientUnmarshal failed: %v", unmarshalErr)
		}
		if resp.Reasoning != "ok" {
			t.Errorf("reasoning: got %q, want ok", resp.Reasoning)
		}
	})

	t.Run("Rust struct literal is not picked up as envelope", func(t *testing.T) {
		// Reproduces a real failure observed with GLM-5.2: the model emitted
		// prose reasoning, and the parser's last-balanced-block fallback
		// returned a Rust struct literal `{ lines: 0, words: 0, bytes: 0 }`.
		// The improved parser must reject this and return an error.
		in := "Let me analyze the task carefully.\n\n" +
			"I need to implement `src/domain/count_strategy.rs` with:\n" +
			"- CountStats struct (lines, words, bytes) with derives\n\n" +
			"CountStats { lines: 0, words: 0, bytes: 0 }\n"
		_, err := ExtractJSONBlock(in)
		if err == nil {
			t.Fatal("expected error for a Rust struct literal with no JSON envelope, got nil")
		}
	})

	t.Run("shell command only response returns error", func(t *testing.T) {
		// Reproduces a real failure: 55-byte response containing only a
		// shell command with no `{` at all. The parser must error, not
		// produce a zero-length block.
		in := "bash\ncommandcd /workspace && cargo test 2>&1 | tail -40\n"
		_, err := ExtractJSONBlock(in)
		if err == nil {
			t.Fatal("expected error for a shell-command-only response, got nil")
		}
	})

	t.Run("fenced code block with Rust braces is stripped", func(t *testing.T) {
		// Reproduces a real 18 KB failure: prose + Rust fenced code blocks,
		// followed by no JSON envelope. The prose contains `let file =
		// File::open(path)?;` which the old scanner would have wrapped into
		// a fake `{...}` block and then fed to json.Unmarshal.
		in := "I'll refactor the tests across all levels.\n\n" +
			"## `src/infrastructure/readers.rs`\n\n" +
			"```rust\n" +
			"use std::fs::File;\nuse std::io::{BufReader, Read};\n\n" +
			"pub fn open_file(path: &Path) -> std::io::Result<Box<dyn Read>> {\n" +
			"    let file = File::open(path)?;\n" +
			"    Ok(Box::new(BufReader::with_capacity(128 * 1024, file)))\n" +
			"}\n" +
			"```\n\n" +
			"And that is it. No JSON envelope provided.\n"
		_, err := ExtractJSONBlock(in)
		if err == nil {
			t.Fatal("expected error for fenced-code-only response, got nil")
		}
	})

	t.Run("json-fenced envelope is unwrapped", func(t *testing.T) {
		// Some compliant models wrap their JSON in a ```json code fence.
		// The parser must unwrap it and parse the JSON inside.
		envelope := `{"reasoning":"wrapped","actions":[{"tool":"noop","args":{}}]}`
		in := "```json\n" + envelope + "\n```\n"
		got, err := ExtractJSONBlock(in)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !strings.Contains(got, `"reasoning"`) {
			t.Errorf("expected the fence to be unwrapped, got %q", got)
		}
	})

	t.Run("json-fenced envelope stays parseable", func(t *testing.T) {
		envelope := `{"reasoning":"wrapped","actions":[{"tool":"noop","args":{}}]}`
		in := "```json\n" + envelope + "\n```\n"
		got, err := ExtractJSONBlock(in)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		resp, err := LenientUnmarshal(got)
		if err != nil {
			t.Fatalf("LenientUnmarshal failed: %v", err)
		}
		if resp.Reasoning != "wrapped" {
			t.Errorf("reasoning: got %q, want wrapped", resp.Reasoning)
		}
	})

	t.Run("mustache prose with no JSON returns error", func(t *testing.T) {
		// A prose-only response must not silently succeed.
		in := "I will write the tests now.\n\nThe implementation looks correct. I just need to add the test file.\n"
		_, err := ExtractJSONBlock(in)
		if err == nil {
			t.Fatal("expected error for a prose-only response, got nil")
		}
	})
}

func TestEscapeNewlinesInJSON(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "No newlines",
			input: `{"key": "value"}`,
			want:  `{"key": "value"}`,
		},
		{
			name:  "Raw newline inside string value",
			input: `{"key": "val` + "\n" + `ue"}`,
			want:  `{"key": "val\nue"}`,
		},
		{
			name:  "Raw carriage return inside string value",
			input: `{"key": "val` + "\r" + `ue"}`,
			want:  `{"key": "val\rue"}`,
		},
		{
			name:  "Newline outside string value",
			input: `{"key": "value"}` + "\n",
			want:  `{"key": "value"}` + "\n",
		},
		{
			name:  "Escaped quotes and backslashes inside string",
			input: `{"key": "val\\\"` + "\n" + `ue"}`,
			want:  `{"key": "val\\\"\nue"}`,
		},
		{
			name:  "Raw tab inside string value",
			input: `{"content": "func main() {` + "\t" + `body}"}`,
			want:  `{"content": "func main() {\tbody}"}`,
		},
		{
			name:  "Raw backspace inside string value",
			input: `{"k": "x` + "\b" + `y"}`,
			want:  `{"k": "x\by"}`,
		},
		{
			name:  "Raw form-feed inside string value",
			input: `{"k": "x` + "\f" + `y"}`,
			want:  `{"k": "x\fy"}`,
		},
		{
			name:  "Tab outside string (structural) is left alone",
			input: `{"k": "v"}` + "\t",
			want:  `{"k": "v"}` + "\t",
		},
		{
			name:  "Escaped backslash followed by real newline",
			input: `{"k": "x\\` + "\n" + `y"}`,
			want:  `{"k": "x\\\ny"}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := escapeNewlinesInJSON(tt.input)
			if got != tt.want {
				t.Errorf("escapeNewlinesInJSON(%q) = %q; want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestLooksLikeResponseFormatRejection(t *testing.T) {
	cases := []struct {
		body string
		want bool
	}{
		{`{"error":{"message":"unknown parameter: response_format"}}`, true},
		{`{"error":"response_format is not supported"}`, true},
		{`{"error":{"message":"Unrecognized request argument supplied"}}`, true},
		{`{"error":{"message":"unknown parameter"}}`, true},
		{"response_format", true},
		{`{"error":{"message":"insufficient_quota"}}`, false},
		{"", false},
	}
	for _, c := range cases {
		got := looksLikeResponseFormatRejection(c.body)
		if got != c.want {
			t.Errorf("looksLikeResponseFormatRejection(%q) = %v; want %v", c.body, got, c.want)
		}
	}
}

func TestBuildJSONReminderPrompt(t *testing.T) {
	originalPrompt := "Write tests for task: foo - bar"
	body := []byte("some prose here that is not JSON")
	p := buildJSONReminderPrompt(originalPrompt, body)
	if !strings.Contains(p, "CRITICAL INSTRUCTION") {
		t.Error("reminder prompt missing the critical instruction marker")
	}
	if !strings.Contains(p, originalPrompt) {
		t.Error("reminder prompt should include the original prompt")
	}
	if !strings.Contains(p, "some prose here") {
		t.Error("reminder prompt should include a tail of the rejected body")
	}

	// Truncation: huge body must produce a short tail.
	huge := make([]byte, 500_000)
	for i := range huge {
		huge[i] = 'A'
	}
	p2 := buildJSONReminderPrompt(originalPrompt, huge)
	if !strings.Contains(p2, "...[truncated]...") {
		t.Error("reminder prompt for large body should include a truncation marker")
	}
}

func TestParseAndUnmarshal(t *testing.T) {
	// Happy path with embedded newlines/tabs preserved then escaped.
	// The model typically writes `\"` for inner quotes — we keep the JSON
	// well-formed by using the escaped quote sequence.
	body := []byte("{\"reasoning\":\"ok\",\"actions\":[{\"tool\":\"write_file\",\"args\":{\"path\":\"a.rs\",\"content\":\"fn main() {\\n\\tprintln!(\\\"hi\\\");\\n}\"}}]}")
	resp, err := parseAndUnmarshal(body)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Reasoning != "ok" {
		t.Errorf("reasoning: want ok, got %q", resp.Reasoning)
	}
	if len(resp.Actions) != 1 {
		t.Fatalf("expected 1 action, got %d", len(resp.Actions))
	}
	if resp.Actions[0].Tool != "write_file" {
		t.Errorf("action 0 tool: got %q, want write_file", resp.Actions[0].Tool)
	}
	content, _ := resp.Actions[0].Args["content"].(string)
	if !strings.Contains(content, "println") {
		t.Errorf("content lost source code: %q", content)
	}
	if !strings.Contains(content, "\n") {
		t.Errorf("content lost embedded newlines: %q", content)
	}
}

func TestParseAndUnmarshalRejectsRustStruct(t *testing.T) {
	// Regression guard: prose + a Rust struct literal must NOT be accepted.
	body := []byte("Let me analyze...\n\nCountStats { lines: 0, words: 0, bytes: 0 }\n")
	resp, err := parseAndUnmarshal(body)
	if err == nil {
		t.Fatalf("expected error, got response %+v", resp)
	}
	if !strings.Contains(err.Error(), "JSON envelope not detected") &&
		!strings.Contains(err.Error(), "no valid JSON object") {
		t.Errorf("expected a parse-rejection error, got %v", err)
	}
}

func TestParseAndUnmarshalRejectsShellCommand(t *testing.T) {
	body := []byte("bash\ncommandcd /workspace && cargo test 2>&1 | tail -40\n")
	_, err := parseAndUnmarshal(body)
	if err == nil {
		t.Fatal("expected error for shell-only response, got nil")
	}
}

func TestLLMResponseIsExpectedShape(t *testing.T) {
	// Sanity: domain.LLMResponse is still importable and used.
	var r domain.LLMResponse
	_ = r
}
