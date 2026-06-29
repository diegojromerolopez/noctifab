package llm

import (
	"testing"
)

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
