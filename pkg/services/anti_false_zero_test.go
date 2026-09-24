package services

import (
	"strings"
	"testing"
)

func TestDetectFalseZeroExit_Signals(t *testing.T) {
	tests := []struct {
		name           string
		output         string
		exitCode       int
		expectDetected bool
		expectedSignal string
	}{
		{
			name:           "Segmentation fault masked by exit 0",
			output:         "running tests...\nSegmentation fault (core dumped)\n",
			exitCode:       0,
			expectDetected: true,
			expectedSignal: "SIGSEGV",
		},
		{
			name:           "Go runtime panic masked by exit 0",
			output:         "=== RUN TestServer\npanic: runtime error: invalid memory address or nil pointer dereference\n",
			exitCode:       0,
			expectDetected: true,
			expectedSignal: "PANIC_GO",
		},
		{
			name:           "Rust thread panic",
			output:         "running 1 test\nthread 'main' panicked at 'explicit panic', src/main.rs:12:9\n",
			exitCode:       0,
			expectDetected: true,
			expectedSignal: "PANIC_RUST",
		},
		{
			name:           "Pytest internal error",
			output:         "INTERNALERROR> Traceback (most recent call last):\nINTERNALERROR>   File \"pytest.py\", line 10\n",
			exitCode:       0,
			expectDetected: true,
			expectedSignal: "FATAL_PYTHON",
		},
		{
			name:           "Fatal Python interpreter crash",
			output:         "Fatal Python error: Segmentation fault\n",
			exitCode:       0,
			expectDetected: true,
			expectedSignal: "FATAL_PYTHON",
		},
		{
			name:           "JavaScript heap out of memory",
			output:         "<--- Last few GCs --->\nFATAL ERROR: Ineffective mark-compacts near heap limit Allocation failed - JavaScript heap out of memory\n",
			exitCode:       0,
			expectDetected: true,
			expectedSignal: "OOM",
		},
		{
			name:           "AddressSanitizer deadly signal",
			output:         "==1234==ERROR: AddressSanitizer: DEADLYSIGNAL\n==1234==The signal is received at pc 0x0000\n",
			exitCode:       0,
			expectDetected: true,
			expectedSignal: "SANITIZER",
		},
		{
			name:           "Normal clean exit 0 passes cleanly",
			output:         "=== RUN TestMath\n--- PASS: TestMath (0.00s)\nPASS\nok  example.com/math 0.05s\n",
			exitCode:       0,
			expectDetected: false,
		},
		{
			name:           "Exit code already non-zero is not a false-zero",
			output:         "Segmentation fault\n",
			exitCode:       139,
			expectDetected: false,
		},
		{
			name:           "Assertion line checking for crash does not trigger false positive",
			output:         "def test_crash():\n    assert \"Segmentation fault\" in err\n",
			exitCode:       0,
			expectDetected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			violation := DetectFalseZeroExit(tt.output, tt.exitCode)
			if violation.Detected != tt.expectDetected {
				t.Fatalf("expected detected=%v, got %v (output: %q)", tt.expectDetected, violation.Detected, tt.output)
			}
			if tt.expectDetected {
				if violation.Signal != tt.expectedSignal {
					t.Errorf("expected signal %q, got %q", tt.expectedSignal, violation.Signal)
				}
				diag := FormatFalseZeroDiagnostic(violation)
				if !strings.Contains(diag, "REJECTED FALSE-POSITIVE PASS") {
					t.Errorf("expected diagnostic to contain 'REJECTED FALSE-POSITIVE PASS'")
				}
			}
		})
	}
}
