package services

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestRepairAlignmentGuard_ExtractTracebackFiles(t *testing.T) {
	guard := NewRepairAlignmentGuard()

	t.Run("extracts Python traceback files", func(t *testing.T) {
		log := `Traceback (most recent call last):
  File "/app/src/commands/strings.py", line 45, in execute_append
    return self.store.append(key, val)
  File "/app/tests/unit/test_strings.py", line 12, in test_append
    self.assertEqual(b":8\r\n", res)
AssertionError: b':8\r\n' != b':6\r\n'`

		files := guard.ExtractTracebackFiles("/app", log)
		assert.Contains(t, files, "src/commands/strings.py")
		assert.Contains(t, files, "tests/unit/test_strings.py")
	})

	t.Run("extracts Go error files", func(t *testing.T) {
		log := `--- FAIL: TestStore (0.01s)
    store_test.go:42: assertion failed
FAIL
pkg/services/store.go:128:2: undefined: MissingMethod`

		files := guard.ExtractTracebackFiles("", log)
		assert.Contains(t, files, "store_test.go")
		assert.Contains(t, files, "pkg/services/store.go")
	})

	t.Run("filters external node_modules and site-packages", func(t *testing.T) {
		log := `Traceback (most recent call last):
  File "/root/.local/lib/python3.11/site-packages/pytest/__init__.py", line 10, in <module>
  File "/app/src/main.py", line 5, in run`

		files := guard.ExtractTracebackFiles("/app", log)
		assert.NotContains(t, files, "/root/.local/lib/python3.11/site-packages/pytest/__init__.py")
		assert.Contains(t, files, "src/main.py")
	})
}

func TestRepairAlignmentGuard_ValidateRepairAlignment(t *testing.T) {
	guard := NewRepairAlignmentGuard()

	t.Run("rejects zero modified files", func(t *testing.T) {
		res := guard.ValidateRepairAlignment(nil, []string{"src/store.py"}, []string{"src/store.py"}, "", nil)
		assert.False(t, res.Allowed)
		assert.Contains(t, res.Reason, "zero file modifications")
	})

	t.Run("rejects identical repeated diff hash", func(t *testing.T) {
		diff := "diff --git a/foo.py b/foo.py\n+pass\n"
		hash := guard.ComputeDiffHash(diff)

		res := guard.ValidateRepairAlignment(
			[]string{"foo.py"},
			[]string{"foo.py"},
			[]string{"foo.py"},
			hash,
			[]string{"some-other-hash", hash},
		)
		assert.False(t, res.Allowed)
		assert.Contains(t, res.Reason, "identical diff")
	})

	t.Run("rejects sycophantic repair modifying unrelated files", func(t *testing.T) {
		res := guard.ValidateRepairAlignment(
			[]string{"README.md", "src/unrelated.py"},
			[]string{"src/commands/strings.py"},
			[]string{"src/commands/strings.py"},
			"diff-1",
			nil,
		)
		assert.False(t, res.Allowed)
		assert.Contains(t, res.Reason, "sycophantic repair rejected")
	})

	t.Run("allows repair modifying traceback file", func(t *testing.T) {
		res := guard.ValidateRepairAlignment(
			[]string{"src/commands/strings.py"},
			[]string{"src/commands/strings.py"},
			[]string{"src/commands/strings.py"},
			"diff-1",
			nil,
		)
		assert.True(t, res.Allowed)
	})

	t.Run("allows repair matching base filename", func(t *testing.T) {
		res := guard.ValidateRepairAlignment(
			[]string{"strings.py"},
			[]string{"src/commands/strings.py"},
			[]string{"src/commands/strings.py"},
			"diff-1",
			nil,
		)
		assert.True(t, res.Allowed)
	})
}
