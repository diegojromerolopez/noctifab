package jira

import (
	"strings"
	"testing"
)

func TestParseADFJSON(t *testing.T) {
	tests := []struct {
		name     string
		adfJSON  string
		expected string
		wantErr  bool
	}{
		{
			name:     "empty adf",
			adfJSON:  "",
			expected: "",
			wantErr:  false,
		},
		{
			name: "simple paragraph and heading",
			adfJSON: `{
				"type": "doc",
				"content": [
					{
						"type": "heading",
						"attrs": {"level": 2},
						"content": [
							{"type": "text", "text": "Feature Request"}
						]
					},
					{
						"type": "paragraph",
						"content": [
							{"type": "text", "text": "Please implement "},
							{
								"type": "text",
								"text": "contact CRUD",
								"marks": [{"type": "strong"}]
							},
							{"type": "text", "text": "."}
						]
					}
				]
			}`,
			expected: "\n## Feature Request\n\nPlease implement **contact CRUD**.\n",
			wantErr:  false,
		},
		{
			name: "bullet list and code block",
			adfJSON: `{
				"type": "doc",
				"content": [
					{
						"type": "bulletList",
						"content": [
							{
								"type": "listItem",
								"content": [
									{"type": "text", "text": "Step 1"}
								]
							}
						]
					},
					{
						"type": "codeBlock",
						"attrs": {"language": "go"},
						"content": [
							{"type": "text", "text": "package main"}
						]
					}
				]
			}`,
			expected: "\n* Step 1\n\n```go\npackage main\n```\n",
			wantErr:  false,
		},
		{
			name: "panel and unsupported media",
			adfJSON: `{
				"type": "doc",
				"content": [
					{
						"type": "panel",
						"content": [
							{"type": "text", "text": "Info panel text"}
						]
					},
					{
						"type": "mediaSingle"
					}
				]
			}`,
			expected: "\n> Info panel text\n\n[Unsupported block node: mediaSingle]\n",
			wantErr:  false,
		},
		{
			name:     "malformed json error",
			adfJSON:  `{malformed}`,
			expected: "",
			wantErr:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseADFJSON(tt.adfJSON)
			if (err != nil) != tt.wantErr {
				t.Errorf("ParseADFJSON() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if err == nil {
				// Normalize whitespaces slightly for easier comparison
				gotNorm := strings.ReplaceAll(got, "\r", "")
				expectedNorm := strings.ReplaceAll(tt.expected, "\r", "")
				if gotNorm != expectedNorm {
					t.Errorf("ParseADFJSON() = %q, want %q", gotNorm, expectedNorm)
				}
			}
		})
	}
}
