package services

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/diegojromerolopez/noctifab/pkg/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTestDepthEvaluator(t *testing.T) {
	evaluator := NewTestDepthEvaluator()

	t.Run("when test is shallow with only status checks it fails depth evaluation", func(t *testing.T) {
		tmpDir := t.TempDir()
		testFile := filepath.Join(tmpDir, "tests", "test_smoke.py")
		require.NoError(t, os.MkdirAll(filepath.Dir(testFile), 0755))

		// Shallow test: checks only status code, no body, no headers, no state check
		shallowCode := `
import requests
def test_smoke():
    res = requests.get("http://localhost:8080/obj")
    assert res.status_code == 200
`
		require.NoError(t, os.WriteFile(testFile, []byte(shallowCode), 0644))

		res, err := evaluator.EvaluateDepth(context.Background(), tmpDir, []string{testFile})
		require.NoError(t, err)
		assert.False(t, res.Passed)
		assert.True(t, res.IsShallow)
		assert.NotEmpty(t, res.MissingDimensions)
		assert.Contains(t, res.RemediationMessage, "Test suite is shallow")
	})

	t.Run("when test is exhaustive with body headers state negative boundary it passes", func(t *testing.T) {
		tmpDir := t.TempDir()
		testFile := filepath.Join(tmpDir, "tests", "test_exhaustive.py")
		require.NoError(t, os.MkdirAll(filepath.Dir(testFile), 0755))

		exhaustiveCode := `
import os
import requests

def test_exhaustive_put_get():
    payload = b"hello world"
    res = requests.put("http://localhost:8080/obj1", data=payload)
    assert res.status_code == 201
    assert "etag" in res.headers
    assert res.headers["server"] == "t4"
    assert res.headers["content-length"] == "0"
    
    # Dual-layer verification: verify file persisted on disk directly
    assert os.path.exists("./data/obj1")
    with open("./data/obj1", "rb") as f:
        assert f.read() == payload

    # GET response check with exact body
    get_res = requests.get("http://localhost:8080/obj1")
    assert get_res.status_code == 200
    assert get_res.content == payload
    assert get_res.headers["content-type"] == "application/octet-stream"

def test_negative_traversal():
    # Negative test
    res = requests.get("http://localhost:8080/../etc/passwd")
    assert res.status_code == 400
    assert b"invalid key" in res.content

def test_boundary_empty_body():
    # Boundary test: 0 bytes empty body
    res = requests.put("http://localhost:8080/empty", data=b"")
    assert res.status_code == 201
    assert res.headers["content-length"] == "0"
`
		require.NoError(t, os.WriteFile(testFile, []byte(exhaustiveCode), 0644))

		res, err := evaluator.EvaluateDepth(context.Background(), tmpDir, []string{testFile})
		require.NoError(t, err)
		assert.True(t, res.Passed, "expected exhaustive test to pass depth evaluation: %v", res.MissingDimensions)
		assert.False(t, res.IsShallow)
		assert.Empty(t, res.MissingDimensions)
	})

	t.Run("when test files are empty it returns shallow", func(t *testing.T) {
		tmpDir := t.TempDir()
		testFile := filepath.Join(tmpDir, "tests", "empty_test.py")
		require.NoError(t, os.MkdirAll(filepath.Dir(testFile), 0755))
		require.NoError(t, os.WriteFile(testFile, []byte("   \n"), 0644))

		res, err := evaluator.EvaluateDepth(context.Background(), tmpDir, []string{testFile})
		require.NoError(t, err)
		assert.False(t, res.Passed)
		assert.True(t, res.IsShallow)
	})

	t.Run("when ValidateTask encounters shallow test it rejects pass", func(t *testing.T) {
		tmpDir := t.TempDir()
		testFile := filepath.Join(tmpDir, "tests", "test_shallow.py")
		require.NoError(t, os.MkdirAll(filepath.Dir(testFile), 0755))
		// Shallow test: status check only
		require.NoError(t, os.WriteFile(testFile, []byte("assert res.status == 200\n"), 0644))

		sb := &scriptedSandbox{results: []error{nil}, outputs: []string{"OK: 1 test passed"}}
		tv := NewTestValidator(sb, false, nil, nil)
		state := &domain.State{ProjectPath: tmpDir}
		task := domain.Task{
			ID:          "task-1",
			TargetFiles: []string{testFile},
		}

		passed, msg, err := tv.ValidateTask(context.Background(), state, task)
		require.NoError(t, err)
		assert.False(t, passed, "expected task with shallow test to fail validation")
		assert.Contains(t, msg, "Test suite is shallow")
	})
}
