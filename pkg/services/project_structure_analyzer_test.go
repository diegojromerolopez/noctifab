package services

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestProjectStructureAnalyzer_ModuleShadowing(t *testing.T) {
	t.Run("detects Python module shadowing collision when file and directory share stem", func(t *testing.T) {
		tempDir := t.TempDir()
		srcDir := filepath.Join(tempDir, "src")
		respDir := filepath.Join(srcDir, "resp")
		require.NoError(t, os.MkdirAll(respDir, 0755))

		// Create flat file src/resp.py and subpackage src/resp/parser.py
		require.NoError(t, os.WriteFile(filepath.Join(srcDir, "resp.py"), []byte("# legacy flat module"), 0644))
		require.NoError(t, os.WriteFile(filepath.Join(respDir, "__init__.py"), []byte(""), 0644))
		require.NoError(t, os.WriteFile(filepath.Join(respDir, "parser.py"), []byte("def parse(): pass"), 0644))

		analyzer := NewProjectStructureAnalyzer(tempDir)
		violations := analyzer.CheckModuleShadowing(tempDir)

		require.Len(t, violations, 1)
		assert.Equal(t, ViolationShadowing, violations[0].Type)
		assert.Equal(t, "python", violations[0].Language)
		assert.Contains(t, violations[0].File, "resp.py")
		assert.Contains(t, violations[0].Dir, "resp")
		assert.Contains(t, violations[0].Description, "Python package shadowing collision")
	})

	t.Run("detects TypeScript module shadowing collision", func(t *testing.T) {
		tempDir := t.TempDir()
		srcDir := filepath.Join(tempDir, "src")
		authDir := filepath.Join(srcDir, "auth")
		require.NoError(t, os.MkdirAll(authDir, 0755))

		require.NoError(t, os.WriteFile(filepath.Join(srcDir, "auth.ts"), []byte("export const auth = 1;"), 0644))
		require.NoError(t, os.WriteFile(filepath.Join(authDir, "index.ts"), []byte("export const auth = 2;"), 0644))

		analyzer := NewProjectStructureAnalyzer(tempDir)
		violations := analyzer.CheckModuleShadowing(tempDir)

		require.Len(t, violations, 1)
		assert.Equal(t, ViolationShadowing, violations[0].Type)
		assert.Equal(t, "typescript", violations[0].Language)
		assert.Contains(t, violations[0].File, "auth.ts")
	})

	t.Run("detects Rust module shadowing collision", func(t *testing.T) {
		tempDir := t.TempDir()
		srcDir := filepath.Join(tempDir, "src")
		engineDir := filepath.Join(srcDir, "engine")
		require.NoError(t, os.MkdirAll(engineDir, 0755))

		require.NoError(t, os.WriteFile(filepath.Join(srcDir, "engine.rs"), []byte("pub fn run() {}"), 0644))
		require.NoError(t, os.WriteFile(filepath.Join(engineDir, "core.rs"), []byte("pub fn core() {}"), 0644))

		analyzer := NewProjectStructureAnalyzer(tempDir)
		violations := analyzer.CheckModuleShadowing(tempDir)

		require.Len(t, violations, 1)
		assert.Equal(t, ViolationShadowing, violations[0].Type)
		assert.Equal(t, "rust", violations[0].Language)
		assert.Contains(t, violations[0].File, "engine.rs")
	})

	t.Run("clean layout with non-colliding files and subpackages produces zero shadowing violations", func(t *testing.T) {
		tempDir := t.TempDir()
		srcDir := filepath.Join(tempDir, "src")
		storeDir := filepath.Join(srcDir, "store")
		require.NoError(t, os.MkdirAll(storeDir, 0755))

		require.NoError(t, os.WriteFile(filepath.Join(srcDir, "main.py"), []byte("import src.store"), 0644))
		require.NoError(t, os.WriteFile(filepath.Join(storeDir, "__init__.py"), []byte(""), 0644))
		require.NoError(t, os.WriteFile(filepath.Join(storeDir, "engine.py"), []byte(""), 0644))

		analyzer := NewProjectStructureAnalyzer(tempDir)
		violations := analyzer.CheckModuleShadowing(tempDir)
		assert.Empty(t, violations)
	})
}

func TestProjectStructureAnalyzer_LayoutConventions(t *testing.T) {
	t.Run("detects Python package directory missing __init__.py", func(t *testing.T) {
		tempDir := t.TempDir()
		pkgDir := filepath.Join(tempDir, "src", "mypackage")
		require.NoError(t, os.MkdirAll(pkgDir, 0755))
		require.NoError(t, os.WriteFile(filepath.Join(pkgDir, "logic.py"), []byte("def run(): pass"), 0644))

		analyzer := NewProjectStructureAnalyzer(tempDir)
		violations := analyzer.CheckLayoutConventions(tempDir)

		require.Len(t, violations, 1)
		assert.Equal(t, ViolationLayoutConvention, violations[0].Type)
		assert.Equal(t, "python", violations[0].Language)
		assert.Contains(t, violations[0].Description, "missing mandatory '__init__.py'")
	})

	t.Run("Python package with __init__.py passes layout convention check", func(t *testing.T) {
		tempDir := t.TempDir()
		pkgDir := filepath.Join(tempDir, "src", "mypackage")
		require.NoError(t, os.MkdirAll(pkgDir, 0755))
		require.NoError(t, os.WriteFile(filepath.Join(pkgDir, "__init__.py"), []byte(""), 0644))
		require.NoError(t, os.WriteFile(filepath.Join(pkgDir, "logic.py"), []byte("def run(): pass"), 0644))

		analyzer := NewProjectStructureAnalyzer(tempDir)
		violations := analyzer.CheckLayoutConventions(tempDir)
		assert.Empty(t, violations)
	})

	t.Run("detects TypeScript project missing package.json and tsconfig.json", func(t *testing.T) {
		tempDir := t.TempDir()
		srcDir := filepath.Join(tempDir, "src")
		require.NoError(t, os.MkdirAll(srcDir, 0755))
		require.NoError(t, os.WriteFile(filepath.Join(srcDir, "index.ts"), []byte("console.log('hi');"), 0644))

		analyzer := NewProjectStructureAnalyzer(tempDir)
		violations := analyzer.CheckLayoutConventions(tempDir)

		require.Len(t, violations, 1)
		assert.Equal(t, ViolationLayoutConvention, violations[0].Type)
		assert.Equal(t, "typescript", violations[0].Language)
		assert.Contains(t, violations[0].Description, "missing both 'package.json' and 'tsconfig.json'")
	})

	t.Run("detects Rust project missing Cargo.toml", func(t *testing.T) {
		tempDir := t.TempDir()
		srcDir := filepath.Join(tempDir, "src")
		require.NoError(t, os.MkdirAll(srcDir, 0755))
		require.NoError(t, os.WriteFile(filepath.Join(srcDir, "main.rs"), []byte("fn main() {}"), 0644))

		analyzer := NewProjectStructureAnalyzer(tempDir)
		violations := analyzer.CheckLayoutConventions(tempDir)

		require.Len(t, violations, 1)
		assert.Equal(t, ViolationLayoutConvention, violations[0].Type)
		assert.Equal(t, "rust", violations[0].Language)
		assert.Contains(t, violations[0].Description, "missing canonical 'Cargo.toml' manifest")
	})
}

func TestProjectStructureAnalyzer_OrphanZombieFiles(t *testing.T) {
	t.Run("detects unreferenced zombie file left behind after refactoring", func(t *testing.T) {
		tempDir := t.TempDir()
		srcDir := filepath.Join(tempDir, "src")
		require.NoError(t, os.MkdirAll(srcDir, 0755))

		// Active application files
		require.NoError(t, os.WriteFile(filepath.Join(srcDir, "main.py"), []byte("from src.active import do_work\ndo_work()"), 0644))
		require.NoError(t, os.WriteFile(filepath.Join(srcDir, "active.py"), []byte("def do_work(): pass"), 0644))

		// Orphan file never imported or mentioned
		require.NoError(t, os.WriteFile(filepath.Join(srcDir, "zombie_spike_remnant.py"), []byte("def dead(): pass"), 0644))

		analyzer := NewProjectStructureAnalyzer(tempDir)
		violations := analyzer.CheckOrphanZombieFiles(tempDir)

		require.Len(t, violations, 1)
		assert.Equal(t, ViolationOrphanZombie, violations[0].Type)
		assert.Contains(t, violations[0].File, "zombie_spike_remnant.py")
		assert.Contains(t, violations[0].Description, "Orphan/zombie file detected")
	})

	t.Run("does not flag entrypoint, tests, or __init__ files as orphans", func(t *testing.T) {
		tempDir := t.TempDir()
		srcDir := filepath.Join(tempDir, "src")
		testsDir := filepath.Join(tempDir, "tests")
		require.NoError(t, os.MkdirAll(srcDir, 0755))
		require.NoError(t, os.MkdirAll(testsDir, 0755))

		require.NoError(t, os.WriteFile(filepath.Join(srcDir, "main.py"), []byte("import src.service\nsrc.service.run()"), 0644))
		require.NoError(t, os.WriteFile(filepath.Join(srcDir, "service.py"), []byte("def run(): pass"), 0644))
		require.NoError(t, os.WriteFile(filepath.Join(srcDir, "__init__.py"), []byte(""), 0644))
		require.NoError(t, os.WriteFile(filepath.Join(testsDir, "test_service.py"), []byte("import src.service"), 0644))

		analyzer := NewProjectStructureAnalyzer(tempDir)
		violations := analyzer.CheckOrphanZombieFiles(tempDir)
		assert.Empty(t, violations)
	})
}

func TestProjectStructureAnalyzer_FullReport(t *testing.T) {
	tempDir := t.TempDir()
	srcDir := filepath.Join(tempDir, "src")
	subDir := filepath.Join(srcDir, "sub")
	require.NoError(t, os.MkdirAll(subDir, 0755))

	// Introduce shadowing violation
	require.NoError(t, os.WriteFile(filepath.Join(srcDir, "sub.py"), []byte(""), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(subDir, "worker.py"), []byte(""), 0644))

	analyzer := NewProjectStructureAnalyzer(tempDir)
	report, err := analyzer.Analyze(tempDir)

	require.NoError(t, err)
	assert.False(t, report.Valid)
	assert.NotEmpty(t, report.Violations)
	assert.Contains(t, report.Summary(), "Found")
}
