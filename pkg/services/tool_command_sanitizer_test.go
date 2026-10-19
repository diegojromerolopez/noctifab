package services

import (
	"testing"
)

func TestSanitizeToolCommand(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "empty input",
			input:    "",
			expected: "",
		},
		{
			name:     "clean format command",
			input:    "go fmt ./...",
			expected: "go fmt ./...",
		},
		{
			name:     "clean dune format",
			input:    "dune fmt",
			expected: "dune fmt",
		},
		{
			name:     "opam install stripped from dune fmt",
			input:    "opam install ocamlformat && dune fmt",
			expected: "dune fmt",
		},
		{
			name:     "pip install stripped from ruff check",
			input:    "pip install ruff && ruff check .",
			expected: "ruff check .",
		},
		{
			name:     "npm install stripped from npm run format",
			input:    "npm install && npm run format",
			expected: "npm run format",
		},
		{
			name:     "pure installation command returns empty string",
			input:    "opam install ocamlformat",
			expected: "",
		},
		{
			name:     "pip3 install with flags returns empty string",
			input:    "pip3 install --user -r requirements.txt",
			expected: "",
		},
		{
			name:     "cargo install clippy stripped from cargo clippy",
			input:    "cargo install clippy; cargo clippy",
			expected: "cargo clippy",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := SanitizeToolCommand(tc.input)
			if got != tc.expected {
				t.Errorf("SanitizeToolCommand(%q) = %q; expected %q", tc.input, got, tc.expected)
			}
		})
	}
}
