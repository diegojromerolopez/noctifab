package domain

import (
	"encoding/json"
	"reflect"
	"testing"
)

func TestExitCodeList_UnmarshalJSON(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected ExitCodeList
		wantErr  bool
	}{
		{
			name:     "standard int slice",
			input:    `[0, 1, 2]`,
			expected: ExitCodeList{0, 1, 2},
		},
		{
			name:     "string slice",
			input:    `["0", "1"]`,
			expected: ExitCodeList{0, 1},
		},
		{
			name:     "mixed slice",
			input:    `[0, "1", 2]`,
			expected: ExitCodeList{0, 1, 2},
		},
		{
			name:     "map with code as key",
			input:    `{"0": "ok", "1": "error"}`,
			expected: ExitCodeList{0, 1},
		},
		{
			name:     "map with code as value",
			input:    `{"ok": 0, "error": 1}`,
			expected: ExitCodeList{0, 1},
		},
		{
			name:     "single int",
			input:    `0`,
			expected: ExitCodeList{0},
		},
		{
			name:     "single string int",
			input:    `"0"`,
			expected: ExitCodeList{0},
		},
		{
			name:     "null",
			input:    `null`,
			expected: nil,
		},
		{
			name:     "empty array",
			input:    `[]`,
			expected: ExitCodeList{},
		},
		{
			name:    "invalid format",
			input:   `"invalid"`,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got ExitCodeList
			err := json.Unmarshal([]byte(tt.input), &got)
			if (err != nil) != tt.wantErr {
				t.Fatalf("UnmarshalJSON() error = %v, wantErr %v", err, tt.wantErr)
			}
			if !tt.wantErr {
				if tt.expected == nil && got != nil {
					t.Fatalf("expected nil, got %v", got)
				}
				if tt.expected != nil && !reflect.DeepEqual(got, tt.expected) {
					t.Fatalf("expected %v, got %v", tt.expected, got)
				}
			}
		})
	}
}

func TestPublicContract_ExitCodesIntegration(t *testing.T) {
	input := `{"id": "cli-test", "interface": "cli", "exit_codes": {"0": "success"}}`
	var contract PublicContract
	err := json.Unmarshal([]byte(input), &contract)
	if err != nil {
		t.Fatalf("failed to unmarshal contract: %v", err)
	}
	if len(contract.ExitCodes) != 1 || contract.ExitCodes[0] != 0 {
		t.Fatalf("expected [0], got %v", contract.ExitCodes)
	}
}
