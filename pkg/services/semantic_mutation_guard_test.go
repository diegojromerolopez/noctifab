package services

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSemanticMutationGuard(t *testing.T) {
	guard := NewSemanticMutationGuard()

	t.Run("when content is identical, it reports zero byte mutation", func(t *testing.T) {
		err := guard.ValidateSemanticChange("main.py", "x = 1\ny = 2\n", "x = 1\ny = 2\n")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "zero byte mutation")
	})

	t.Run("when only whitespace or comments change, it reports no logic alteration", func(t *testing.T) {
		orig := `
def calculate(a, b):
    # original comment
    return a + b
`
		updated := `
def calculate(a, b):
    # updated explanation comment
    return a + b
`
		err := guard.ValidateSemanticChange("app.py", orig, updated)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "only modifies comments, docstrings, or whitespace")
	})

	t.Run("when only docstrings change in Python, it reports no logic alteration", func(t *testing.T) {
		orig := `
def run():
    """Old docstring."""
    return 42
`
		updated := `
def run():
    """New comprehensive docstring explaining everything."""
    return 42
`
		err := guard.ValidateSemanticChange("app.py", orig, updated)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "only modifies comments, docstrings, or whitespace")
	})

	t.Run("when only debug print statements are added, it reports debug logging only", func(t *testing.T) {
		orig := `
def process(items):
    total = sum(items)
    return total
`
		updated := `
def process(items):
    print("DEBUG: process called with", items)
    total = sum(items)
    console.log("items total computed")
    return total
`
		err := guard.ValidateSemanticChange("app.py", orig, updated)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "only adds or alters debug print/logging statements")
	})

	t.Run("when real structural logic is changed, validation passes", func(t *testing.T) {
		orig := `
def process(items):
    total = sum(items)
    return total
`
		updated := `
def process(items):
    total = sum(items)
    if total < 0:
        return 0
    return total
`
		err := guard.ValidateSemanticChange("app.py", orig, updated)
		assert.NoError(t, err)
	})

	t.Run("when real logic changed and debug prints added, validation passes", func(t *testing.T) {
		orig := `
func Add(a, b int) int {
    return a + b
}
`
		updated := `
func Add(a, b int) int {
    fmt.Println("adding", a, b)
    if a < 0 {
        return 0
    }
    return a + b
}
`
		err := guard.ValidateSemanticChange("main.go", orig, updated)
		assert.NoError(t, err)
	})
}
