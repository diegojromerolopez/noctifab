package services

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDiffMutationValidator_NonVacuousDiff(t *testing.T) {
	v := NewDiffMutationValidator()

	t.Run("identical content fails", func(t *testing.T) {
		err := v.ValidateNonVacuousDiff("func A() {}", "func A() {}")
		assert.ErrorContains(t, err, "zero mutation")
	})

	t.Run("only comments and whitespace changed fails", func(t *testing.T) {
		oldCode := `func Calculate(x int) int {
	return x * 2
}`
		newCode := `// Calculation helper
func Calculate(x int) int {
	// multiply by two
	return x * 2
}
`
		err := v.ValidateNonVacuousDiff(oldCode, newCode)
		assert.ErrorContains(t, err, "vacuous edit rejected")
	})

	t.Run("semantic logic change passes", func(t *testing.T) {
		oldCode := `func Calculate(x int) int { return x * 2 }`
		newCode := `func Calculate(x int) int { return x * 3 }`
		err := v.ValidateNonVacuousDiff(oldCode, newCode)
		require.NoError(t, err)
	})
}

func TestDiffMutationValidator_ASTBody(t *testing.T) {
	v := NewDiffMutationValidator()

	t.Run("Go function with real logic passes", func(t *testing.T) {
		code := `package main
func RealWork(a, b int) int {
	total := a + b
	return total * 2
}`
		err := v.ValidateASTBody("main.go", code)
		require.NoError(t, err)
	})

	t.Run("Go function with empty body fails", func(t *testing.T) {
		code := `package main
func EmptyFunc() {}`
		err := v.ValidateASTBody("main.go", code)
		assert.ErrorContains(t, err, "has an empty body")
	})

	t.Run("Go function returning literal nil fails", func(t *testing.T) {
		code := `package main
func ReturnNil() error {
	return nil
}`
		err := v.ValidateASTBody("main.go", code)
		assert.ErrorContains(t, err, "merely returns nil")
	})

	t.Run("Go function placeholder panic fails", func(t *testing.T) {
		code := `package main
func Placeholder() {
	panic("todo")
}`
		err := v.ValidateASTBody("main.go", code)
		assert.ErrorContains(t, err, "merely panics as a placeholder")
	})

	t.Run("Python function with pass fails", func(t *testing.T) {
		code := `
def calculate_metrics(data):
    pass
`
		err := v.ValidateASTBody("metrics.py", code)
		assert.ErrorContains(t, err, "contains only pass or ellipsis placeholder")
	})

	t.Run("Python function with real statements passes", func(t *testing.T) {
		code := `
def calculate_metrics(data):
    res = sum(data)
    return res / len(data)
`
		err := v.ValidateASTBody("metrics.py", code)
		require.NoError(t, err)
	})
}
