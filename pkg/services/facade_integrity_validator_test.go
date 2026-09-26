package services

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFacadeIntegrityValidator_Python(t *testing.T) {
	v := NewFacadeIntegrityValidator()

	t.Run("detects missing method on Store facade", func(t *testing.T) {
		sourceFiles := map[string]string{
			"src/store.py": `
class Store:
    def __init__(self):
        self.data = {}

    def get(self, key):
        return self.data.get(key)

    def set(self, key, val):
        self.data[key] = val
`,
		}

		testFiles := map[string]string{
			"tests/unit/test_hashes.py": `
import unittest
from src.store import Store

class TestHashes(unittest.TestCase):
    def test_hashes(self):
        store = Store()
        store.set("foo", "bar")
        val = store.get("foo")
        h = store.get_hash("foo")  # Missing method!
`,
		}

		violations := v.ValidateFacades(sourceFiles, testFiles)
		require.Len(t, violations, 1)
		assert.Equal(t, "Store", violations[0].ClassName)
		assert.Equal(t, "get_hash", violations[0].MissingMethod)
		assert.Contains(t, violations[0].Error(), "missing method 'get_hash'")
		assert.Contains(t, violations[0].AvailableNames, "get")
		assert.Contains(t, violations[0].AvailableNames, "set")
	})

	t.Run("passes when all invoked methods exist", func(t *testing.T) {
		sourceFiles := map[string]string{
			"src/store.py": `
class Store:
    def get(self, key):
        pass

    def get_hash(self, key):
        pass
`,
		}

		testFiles := map[string]string{
			"tests/unit/test_hashes.py": `
class TestHashes:
    def test_ok(self):
        store = Store()
        store.get("foo")
        store.get_hash("bar")
`,
		}

		violations := v.ValidateFacades(sourceFiles, testFiles)
		assert.Empty(t, violations)
	})
}

func TestFacadeIntegrityValidator_Go(t *testing.T) {
	v := NewFacadeIntegrityValidator()

	t.Run("detects missing method on Go struct", func(t *testing.T) {
		sourceFiles := map[string]string{
			"pkg/store/store.go": `package store

type Store struct{}

func (s *Store) Get(key string) string { return "" }
func (s *Store) Set(key, val string) {}
`,
		}

		testFiles := map[string]string{
			"pkg/store/store_test.go": `package store

import "testing"

func TestStore(t *testing.T) {
	s := &Store{}
	s.Get("k")
	s.GetHash("h")
}
`,
		}

		violations := v.ValidateFacades(sourceFiles, testFiles)
		require.Len(t, violations, 1)
		assert.Equal(t, "Store", violations[0].ClassName)
		assert.Equal(t, "GetHash", violations[0].MissingMethod)
		assert.Contains(t, violations[0].Error(), "missing method 'GetHash'")
	})
}
