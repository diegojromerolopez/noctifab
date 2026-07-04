package llm

import (
	"strings"
	"testing"
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
