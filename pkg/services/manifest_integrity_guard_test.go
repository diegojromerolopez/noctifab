package services

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestManifestIntegrityGuard_PathIsolation(t *testing.T) {
	guard := NewManifestIntegrityGuard()
	root := "/workspace/my-app"

	t.Run("valid relative path inside workspace passes", func(t *testing.T) {
		err := guard.ValidatePathIsolation(root, "src/main.go")
		require.NoError(t, err)
	})

	t.Run("valid absolute path inside workspace passes", func(t *testing.T) {
		err := guard.ValidatePathIsolation(root, "/workspace/my-app/pkg/foo.go")
		require.NoError(t, err)
	})

	t.Run("path escaping root fails", func(t *testing.T) {
		err := guard.ValidatePathIsolation(root, "../secret.txt")
		assert.ErrorContains(t, err, "attempts to escape project root")
	})

	t.Run("absolute path outside root fails", func(t *testing.T) {
		err := guard.ValidatePathIsolation(root, "/etc/passwd")
		assert.ErrorContains(t, err, "attempts to escape project root")
	})

	t.Run("accessing .git fails", func(t *testing.T) {
		err := guard.ValidatePathIsolation(root, ".git/config")
		assert.ErrorContains(t, err, "modifying .git directory is forbidden")
	})

	t.Run("accessing .noctifab fails", func(t *testing.T) {
		err := guard.ValidatePathIsolation(root, ".noctifab/state.json")
		assert.ErrorContains(t, err, "modifying internal .noctifab directory is forbidden")
	})
}

func TestManifestIntegrityGuard_GoImports(t *testing.T) {
	guard := NewManifestIntegrityGuard()

	t.Run("standard library imports pass", func(t *testing.T) {
		code := `package main
import (
	"fmt"
	"net/http"
)
func main() {}`
		err := guard.ValidateImports("main.go", code, nil, "my-module")
		require.NoError(t, err)
	})

	t.Run("project internal package passes", func(t *testing.T) {
		code := `package main
import (
	"my-module/pkg/service"
)
func main() {}`
		err := guard.ValidateImports("main.go", code, nil, "my-module")
		require.NoError(t, err)
	})

	t.Run("declared third-party package passes", func(t *testing.T) {
		code := `package main
import (
	"github.com/gin-gonic/gin"
)
func main() {}`
		err := guard.ValidateImports("main.go", code, []string{"github.com/gin-gonic/gin"}, "my-module")
		require.NoError(t, err)
	})

	t.Run("undeclared third-party package fails", func(t *testing.T) {
		code := `package main
import (
	"github.com/hallucinated/pkg"
)
func main() {}`
		err := guard.ValidateImports("main.go", code, []string{"github.com/gin-gonic/gin"}, "my-module")
		assert.ErrorContains(t, err, "hallucinated import \"github.com/hallucinated/pkg\"")
	})
}

func TestManifestIntegrityGuard_PythonImports(t *testing.T) {
	guard := NewManifestIntegrityGuard()

	t.Run("standard library imports pass", func(t *testing.T) {
		code := `
import os
import sys
from collections import defaultdict
`
		err := guard.ValidateImports("main.py", code, nil, "")
		require.NoError(t, err)
	})

	t.Run("declared package passes", func(t *testing.T) {
		code := `
import fastapi
from pydantic import BaseModel
`
		err := guard.ValidateImports("main.py", code, []string{"fastapi", "pydantic"}, "")
		require.NoError(t, err)
	})

	t.Run("undeclared package fails", func(t *testing.T) {
		code := `
import requests
from nonexistent_ai import Model
`
		err := guard.ValidateImports("main.py", code, []string{"requests"}, "")
		assert.ErrorContains(t, err, "hallucinated import \"nonexistent_ai\"")
	})
}
