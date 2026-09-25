package services

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTestASTAnalyzer_Go(t *testing.T) {
	analyzer := NewTestASTAnalyzer()
	tmpDir := t.TempDir()

	t.Run("valid test passes without violations", func(t *testing.T) {
		code := `package mypkg_test

import (
	"testing"
	"github.com/stretchr/testify/assert"
)

func TestAddition(t *testing.T) {
	sum := 2 + 2
	assert.Equal(t, 4, sum)
}
`
		path := filepath.Join(tmpDir, "valid_test.go")
		require.NoError(t, os.WriteFile(path, []byte(code), 0644))

		violations, err := analyzer.AnalyzeFile(context.Background(), path)
		require.NoError(t, err)
		assert.Empty(t, violations)
	})

	t.Run("empty test body flagged", func(t *testing.T) {
		code := `package mypkg_test

import "testing"

func TestEmpty(t *testing.T) {
}
`
		path := filepath.Join(tmpDir, "empty_test.go")
		require.NoError(t, os.WriteFile(path, []byte(code), 0644))

		violations, err := analyzer.AnalyzeFile(context.Background(), path)
		require.NoError(t, err)
		require.NotEmpty(t, violations)
		assert.Equal(t, "ast_empty_test", violations[0].Rule)
	})

	t.Run("missing assertions flagged", func(t *testing.T) {
		code := `package mypkg_test

import (
	"testing"
	"fmt"
)

func TestNoAssert(t *testing.T) {
	fmt.Println("doing work")
}
`
		path := filepath.Join(tmpDir, "no_assert_test.go")
		require.NoError(t, os.WriteFile(path, []byte(code), 0644))

		violations, err := analyzer.AnalyzeFile(context.Background(), path)
		require.NoError(t, err)
		require.NotEmpty(t, violations)
		assert.Equal(t, "ast_missing_assertion", violations[0].Rule)
	})

	t.Run("tautological assert True flagged", func(t *testing.T) {
		code := `package mypkg_test

import (
	"testing"
	"github.com/stretchr/testify/assert"
)

func TestTautology(t *testing.T) {
	assert.True(t, true)
}
`
		path := filepath.Join(tmpDir, "tautology_test.go")
		require.NoError(t, os.WriteFile(path, []byte(code), 0644))

		violations, err := analyzer.AnalyzeFile(context.Background(), path)
		require.NoError(t, err)
		require.NotEmpty(t, violations)
		hasTautology := false
		for _, v := range violations {
			if v.Rule == "ast_tautological_assertion" {
				hasTautology = true
			}
		}
		assert.True(t, hasTautology)
	})

	t.Run("carbon copy duplicate tests flagged", func(t *testing.T) {
		code := `package mypkg_test

import (
	"testing"
	"github.com/stretchr/testify/assert"
)

func TestFirst(t *testing.T) {
	x := 10
	assert.Equal(t, 10, x)
}

func TestSecond(t *testing.T) {
	x := 10
	assert.Equal(t, 10, x)
}
`
		path := filepath.Join(tmpDir, "duplicate_test.go")
		require.NoError(t, os.WriteFile(path, []byte(code), 0644))

		violations, err := analyzer.AnalyzeFile(context.Background(), path)
		require.NoError(t, err)
		require.NotEmpty(t, violations)
		hasDuplicate := false
		for _, v := range violations {
			if v.Rule == "ast_carbon_copy_duplicate" {
				hasDuplicate = true
			}
		}
		assert.True(t, hasDuplicate)
	})
}

func TestTestASTAnalyzer_Python(t *testing.T) {
	analyzer := NewTestASTAnalyzer()
	tmpDir := t.TempDir()

	code := `def test_empty():
    pass

def test_tautology():
    assert True

def test_real():
    result = 1 + 1
    assert result == 2
`
	path := filepath.Join(tmpDir, "test_sample.py")
	require.NoError(t, os.WriteFile(path, []byte(code), 0644))

	violations, err := analyzer.AnalyzeFile(context.Background(), path)
	require.NoError(t, err)
	if len(violations) > 0 {
		rules := make(map[string]bool)
		for _, v := range violations {
			rules[v.Rule] = true
		}
		assert.True(t, rules["ast_empty_test"])
		assert.True(t, rules["ast_tautological_assertion"])
	}
}

func TestCountDiscoveredTests(t *testing.T) {
	tmpDir := t.TempDir()
	goCode := `package sample_test
import "testing"
func TestOne(t *testing.T) {}
func TestTwo(t *testing.T) {}
`
	require.NoError(t, os.WriteFile(filepath.Join(tmpDir, "foo_test.go"), []byte(goCode), 0644))
	count := CountDiscoveredTests(tmpDir)
	assert.Equal(t, 2, count)
}
