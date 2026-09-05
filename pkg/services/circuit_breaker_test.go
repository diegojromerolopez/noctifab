package services

import (
	"testing"
)

func TestTaskCircuitBreaker(t *testing.T) {
	t.Run("initial state is closed", func(t *testing.T) {
		cb := NewTaskCircuitBreaker()
		tripped, _ := cb.ShouldTrip(50)
		if tripped {
			t.Errorf("expected circuit breaker to be closed initially")
		}
	})

	t.Run("trips when criteria are met", func(t *testing.T) {
		cb := NewTaskCircuitBreaker()

		// 1. Initial test pass
		cb.RecordTestResult(true)

		// 2. Test-only edits
		cb.RecordAction("edit_file", map[string]any{"path": "tests/unit/test_app.py"})
		cb.RecordAction("write_file", map[string]any{"path": "tests/e2e/test_cli.py"})

		// 3. Second test pass
		cb.RecordTestResult(true)

		// At progress < 70, should not trip yet
		tripped, _ := cb.ShouldTrip(50)
		if tripped {
			t.Errorf("expected circuit breaker not to trip when progress < 70")
		}

		// At progress >= 70, should trip
		tripped, msg := cb.ShouldTrip(70)
		if !tripped {
			t.Errorf("expected circuit breaker to trip when passes>=2, test_edits>=2, progress>=70")
		}
		if msg == "" {
			t.Errorf("expected non-empty trip reason")
		}

		// Remains tripped on subsequent checks
		trippedAgain, _ := cb.ShouldTrip(75)
		if !trippedAgain {
			t.Errorf("expected circuit breaker to remain tripped")
		}
	})

	t.Run("resets when production code is modified", func(t *testing.T) {
		cb := NewTaskCircuitBreaker()

		cb.RecordTestResult(true)
		cb.RecordAction("edit_file", map[string]any{"path": "tests/test_x.py"})
		cb.RecordAction("edit_file", map[string]any{"path": "tests/test_y.py"})
		cb.RecordTestResult(true)

		// Modifying production code should reset breaker
		cb.RecordAction("write_file", map[string]any{"path": "src/app.py"})

		tripped, _ := cb.ShouldTrip(75)
		if tripped {
			t.Errorf("expected circuit breaker to reset after production file modification")
		}
		if cb.ConsecutiveTestPasses != 0 || cb.ConsecutiveTestOnlyTurns != 0 {
			t.Errorf("expected counts to reset, got passes=%d, edits=%d", cb.ConsecutiveTestPasses, cb.ConsecutiveTestOnlyTurns)
		}
	})

	t.Run("resets when test fails (regression)", func(t *testing.T) {
		cb := NewTaskCircuitBreaker()

		cb.RecordTestResult(true)
		cb.RecordAction("edit_file", map[string]any{"path": "spec/model_spec.rb"})
		cb.RecordAction("edit_file", map[string]any{"path": "spec/cli_spec.rb"})
		cb.RecordTestResult(true)

		tripped, _ := cb.ShouldTrip(70)
		if !tripped {
			t.Fatalf("expected breaker to trip")
		}

		// A test failure must reset the breaker so repairs can happen
		cb.RecordTestResult(false)

		trippedAfterFail, _ := cb.ShouldTrip(70)
		if trippedAfterFail {
			t.Errorf("expected breaker to untrip when test failure occurs")
		}
	})

	t.Run("handles batch write_files with mixed paths", func(t *testing.T) {
		cb := NewTaskCircuitBreaker()

		// Batch write containing only test files
		cb.RecordAction("write_files", map[string]any{
			"files": []any{
				map[string]any{"path": "tests/test_a.go"},
				map[string]any{"path": "tests/test_b.go"},
			},
		})
		if cb.ConsecutiveTestOnlyTurns != 1 {
			t.Errorf("expected 1 test only turn, got %d", cb.ConsecutiveTestOnlyTurns)
		}

		// Batch write containing production files
		cb.RecordAction("write_files", map[string]any{
			"files": []any{
				map[string]any{"path": "pkg/services/server.go"},
				map[string]any{"path": "tests/server_test.go"},
			},
		})
		if cb.ConsecutiveTestOnlyTurns != 0 {
			t.Errorf("expected reset on production file in batch write, got %d", cb.ConsecutiveTestOnlyTurns)
		}
	})
}
