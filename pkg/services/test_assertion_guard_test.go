package services

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTestAssertionGuard_CountAndMonotonicity(t *testing.T) {
	guard := NewTestAssertionGuard()

	oldGoTest := `package main
import "testing"
func TestAdd(t *testing.T) {
	if 1+1 != 2 {
		t.Errorf("expected 2")
	}
	if 2+2 != 4 {
		t.Fatalf("expected 4")
	}
}`

	newGoTestWeakened := `package main
import "testing"
func TestAdd(t *testing.T) {
	if 1+1 != 2 {
		t.Errorf("expected 2")
	}
}`

	newGoTestStrengthened := `package main
import "testing"
func TestAdd(t *testing.T) {
	if 1+1 != 2 {
		t.Errorf("expected 2")
	}
	if 2+2 != 4 {
		t.Fatalf("expected 4")
	}
	if 3+3 != 6 {
		t.Errorf("expected 6")
	}
}`

	t.Run("count Go assertions accurately", func(t *testing.T) {
		count, err := guard.CountAssertions("main_test.go", oldGoTest)
		require.NoError(t, err)
		assert.Equal(t, 2, count)
	})

	t.Run("weakened assertions fails monotonicity", func(t *testing.T) {
		err := guard.ValidateAssertionMonotonicity("main_test.go", oldGoTest, newGoTestWeakened)
		assert.ErrorContains(t, err, "assertion weakening detected")
	})

	t.Run("equal or strengthened assertions passes monotonicity", func(t *testing.T) {
		err := guard.ValidateAssertionMonotonicity("main_test.go", oldGoTest, newGoTestStrengthened)
		require.NoError(t, err)
	})

	t.Run("count Python assertions accurately", func(t *testing.T) {
		pyCode := `
def test_math():
    assert 1 + 1 == 2
    assert 2 * 2 == 4
`
		count, err := guard.CountAssertions("test_math.py", pyCode)
		require.NoError(t, err)
		assert.Equal(t, 2, count)
	})
}

func TestTestAssertionGuard_Density(t *testing.T) {
	guard := NewTestAssertionGuard()

	t.Run("vacuous Go test with 0 assertions fails density", func(t *testing.T) {
		code := `package main
import "testing"
func TestNoop(t *testing.T) {
	x := 42
	_ = x
}`
		err := guard.ValidateAssertionDensity("main_test.go", code)
		assert.ErrorContains(t, err, "vacuous test detected")
		assert.ErrorContains(t, err, "contains 0 assertions")
	})

	t.Run("Go test with assert passes density", func(t *testing.T) {
		code := `package main
import "testing"
func TestValid(t *testing.T) {
	if 1 != 1 {
		t.Errorf("bad math")
	}
}`
		err := guard.ValidateAssertionDensity("main_test.go", code)
		require.NoError(t, err)
	})
}
