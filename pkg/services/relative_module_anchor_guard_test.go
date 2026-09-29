package services

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRelativeModuleAnchorGuard(t *testing.T) {
	tempDir := t.TempDir()

	// Scaffold directory structure:
	// tempDir/
	//   pkg/
	//     __init__.py
	//     utils.py
	//     sub/
	//       __init__.py
	//       worker.py
	//   web/
	//     app.ts
	//     components/
	//       header.tsx
	require.NoError(t, os.MkdirAll(filepath.Join(tempDir, "pkg", "sub"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(tempDir, "pkg", "__init__.py"), []byte(""), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(tempDir, "pkg", "utils.py"), []byte("def helper(): pass"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(tempDir, "pkg", "sub", "__init__.py"), []byte(""), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(tempDir, "pkg", "sub", "worker.py"), []byte(""), 0o644))

	require.NoError(t, os.MkdirAll(filepath.Join(tempDir, "web", "components"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(tempDir, "web", "app.ts"), []byte(""), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(tempDir, "web", "components", "header.tsx"), []byte(""), 0o644))

	guard := NewRelativeModuleAnchorGuard(tempDir)

	t.Run("when relative imports resolve to existing files, no violations", func(t *testing.T) {
		pyContent := `
from .utils import helper
from .sub.worker import run
`
		violations := guard.ValidateRelativeImports(filepath.Join(tempDir, "pkg", "service.py"), pyContent)
		assert.Empty(t, violations)

		pyParentContent := `
from ..utils import helper
`
		violations = guard.ValidateRelativeImports(filepath.Join(tempDir, "pkg", "sub", "worker.py"), pyParentContent)
		assert.Empty(t, violations)
	})

	t.Run("when relative import points to non-existent module, violation is flagged", func(t *testing.T) {
		pyContent := `
from .non_existent import ghost
`
		violations := guard.ValidateRelativeImports(filepath.Join(tempDir, "pkg", "service.py"), pyContent)
		require.Len(t, violations, 1)
		assert.Equal(t, ".non_existent", violations[0].ImportSpec)
		assert.Equal(t, 2, violations[0].Line)
		assert.Contains(t, violations[0].Error(), "relative module anchoring violation")
	})

	t.Run("when Node relative import resolves to existing file, no violations", func(t *testing.T) {
		tsContent := `
import { Header } from "./components/header";
`
		violations := guard.ValidateRelativeImports(filepath.Join(tempDir, "web", "app.ts"), tsContent)
		assert.Empty(t, violations)
	})

	t.Run("when Node relative import targets non-existent file, violation is flagged", func(t *testing.T) {
		tsContent := `
import { Footer } from "./components/footer";
`
		violations := guard.ValidateRelativeImports(filepath.Join(tempDir, "web", "app.ts"), tsContent)
		require.Len(t, violations, 1)
		assert.Equal(t, "./components/footer", violations[0].ImportSpec)
		assert.Contains(t, violations[0].Error(), "relative module anchoring violation")
	})
}
