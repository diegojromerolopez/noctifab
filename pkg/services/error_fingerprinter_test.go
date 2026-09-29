package services

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestErrorFingerprinter_NormalizeAndFingerprint(t *testing.T) {
	ef := NewErrorFingerprinter()

	err1 := `2026-09-25T23:14:01Z main.go:42:15: undefined: MyStruct at pointer 0x7ffee4b2a1c0 (duration 150ms)`
	err2 := `2026-09-25T23:15:22Z main.go:89:12: undefined: MyStruct at pointer 0x7ffee4b2beef (duration 210ms)`

	norm1 := ef.NormalizeError(err1)
	norm2 := ef.NormalizeError(err2)

	assert.Equal(t, norm1, norm2)
	assert.Contains(t, norm1, "main.go:LINE:COL: undefined: MyStruct at pointer 0xADDR (duration DURATION)")

	hash1 := ef.Fingerprint(err1)
	hash2 := ef.Fingerprint(err2)
	assert.Equal(t, hash1, hash2)
}

func TestErrorFingerprinter_LoopDetection(t *testing.T) {
	ef := NewErrorFingerprinter(1)

	errSample1 := `pkg/service.go:10:5: cannot use x (variable of type int) as string`
	errSample2 := `pkg/service.go:20:5: syntax error: unexpected semicolon`

	t.Run("first occurrence is not a loop", func(t *testing.T) {
		isLoop, hash, diag := ef.RecordAndCheckLoop(errSample1)
		assert.False(t, isLoop)
		assert.NotEmpty(t, hash)
		assert.Empty(t, diag)
	})

	t.Run("immediate duplicate is detected as loop", func(t *testing.T) {
		isLoop, hash, diag := ef.RecordAndCheckLoop(errSample1)
		assert.True(t, isLoop)
		assert.NotEmpty(t, hash)
		assert.Contains(t, diag, "sycophantic error loop detected")
	})

	t.Run("different error is not a loop", func(t *testing.T) {
		isLoop, _, _ := ef.RecordAndCheckLoop(errSample2)
		assert.False(t, isLoop)
	})

	t.Run("reset clears history", func(t *testing.T) {
		ef.Reset()
		isLoop, _, _ := ef.RecordAndCheckLoop(errSample1)
		assert.False(t, isLoop)
	})
}
