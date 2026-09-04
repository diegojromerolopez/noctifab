package services_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/diegojromerolopez/noctifab/pkg/services"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestIsIgnoredLegacyFile(t *testing.T) {
	tests := []struct {
		path     string
		expected bool
	}{
		// Documentation & Meta
		{"SPEC.md", true},
		{"spec.md", true},
		{"README.md", true},
		{"README.rst", true},
		{"LICENSE", true},
		{"LICENSE-APACHE", true},
		{"CHANGELOG.md", true},
		{"CONTRIBUTING.md", true},
		{"noctifab_evaluation_report.md", true},

		// Docker
		{"Dockerfile", true},
		{"dockerfile.validation", true},
		{"docker-compose.yml", true},
		{"docker-compose.e2e.yaml", true},

		// VCS & Dotfiles
		{".gitignore", true},
		{".editorconfig", true},
		{".rubocop.yml", true},
		{".golangci.yml", true},
		{".github/workflows/ci.yml", true},
		{".vscode/settings.json", true},

		// Manifests & Lockfiles
		{"Cargo.toml", true},
		{"Cargo.lock", true},
		{"go.mod", true},
		{"go.sum", true},
		{"package.json", true},
		{"package-lock.json", true},
		{"yarn.lock", true},
		{"requirements.txt", true},
		{"Pipfile", true},
		{"Pipfile.lock", true},
		{"pyproject.toml", true},
		{"setup.py", true},
		{"Gemfile", true},
		{"Gemfile.lock", true},
		{"pom.xml", true},
		{"build.gradle", true},
		{"Makefile", true},
		{"CMakeLists.txt", true},

		// Test configs & helpers
		{".rspec", true},
		{"spec_helper.rb", true},
		{"conftest.py", true},
		{"pytest.ini", true},
		{"tsconfig.json", true},

		// Build directories
		{"target/debug/app", true},
		{"node_modules/express/index.js", true},
		{"vendor/bundle/ruby.rb", true},
		{"build/output.js", true},
		{"__pycache__/mod.pyc", true},

		// Real candidate source files (should NOT be ignored)
		{"src/main.rs", false},
		{"lib/calculator.rb", false},
		{"app/models/user.py", false},
		{"pkg/service/auth.go", false},
		{"index.ts", false},
	}

	for _, tt := range tests {
		t.Run("when checking "+tt.path+", it returns expected value", func(t *testing.T) {
			assert.Equal(t, tt.expected, services.IsIgnoredLegacyFile(tt.path))
		})
	}
}

func TestCountSignificantLines(t *testing.T) {
	tempDir := t.TempDir()

	t.Run("when file is empty, it returns 0", func(t *testing.T) {
		path := filepath.Join(tempDir, "empty.txt")
		require.NoError(t, os.WriteFile(path, []byte(""), 0644))
		count, err := services.CountSignificantLines(path)
		require.NoError(t, err)
		assert.Equal(t, 0, count)
	})

	t.Run("when file contains only comments and whitespace, it returns 0", func(t *testing.T) {
		content := strings.Join([]string{
			"// Go comment",
			"# Python comment",
			"/* Block comment start",
			"* middle",
			"*/ end",
			"-- SQL comment",
			"; Lisp comment",
			"<!-- HTML comment -->",
			"   ",
			"\t\t",
		}, "\n")
		path := filepath.Join(tempDir, "comments.txt")
		require.NoError(t, os.WriteFile(path, []byte(content), 0644))
		count, err := services.CountSignificantLines(path)
		require.NoError(t, err)
		assert.Equal(t, 0, count)
	})

	t.Run("when file has mixed comments, blanks, and code, it counts only code lines", func(t *testing.T) {
		content := strings.Join([]string{
			"// Package math provides calculators",
			"package math",
			"",
			"// Add adds two numbers",
			"func Add(a, b int) int {",
			"\t// Return sum",
			"\treturn a + b",
			"}",
		}, "\n")
		path := filepath.Join(tempDir, "math.go")
		require.NoError(t, os.WriteFile(path, []byte(content), 0644))
		count, err := services.CountSignificantLines(path)
		require.NoError(t, err)
		// Code lines: "package math", "func Add(a, b int) int {", "return a + b", "}" -> 4 lines
		assert.Equal(t, 4, count)
	})
}

func TestScanLegacyFiles_And_IsGreenfieldWorkspace(t *testing.T) {
	t.Run("when workspace is completely empty or has only SPEC.md, it classifies as greenfield", func(t *testing.T) {
		tempDir := t.TempDir()
		require.NoError(t, os.WriteFile(filepath.Join(tempDir, "SPEC.md"), []byte("# Spec"), 0644))

		files, err := services.ScanLegacyFiles(tempDir)
		require.NoError(t, err)
		assert.Empty(t, files)

		isGreenfield, legacyFiles, err := services.IsGreenfieldWorkspace(tempDir)
		require.NoError(t, err)
		assert.True(t, isGreenfield)
		assert.Empty(t, legacyFiles)
	})

	t.Run("when workspace has manifests, lockfiles, and small stubs (< 5 lines), it classifies as greenfield", func(t *testing.T) {
		tempDir := t.TempDir()
		require.NoError(t, os.WriteFile(filepath.Join(tempDir, "SPEC.md"), []byte("# Spec"), 0644))
		require.NoError(t, os.WriteFile(filepath.Join(tempDir, "Cargo.toml"), []byte("[package]\nname = \"calc\"\n"), 0644))
		require.NoError(t, os.WriteFile(filepath.Join(tempDir, "Cargo.lock"), []byte("# lockfile\n"), 0644))
		require.NoError(t, os.WriteFile(filepath.Join(tempDir, ".gitignore"), []byte("target/\n"), 0644))

		// 3-line stub file
		srcDir := filepath.Join(tempDir, "src")
		require.NoError(t, os.MkdirAll(srcDir, 0755))
		require.NoError(t, os.WriteFile(filepath.Join(srcDir, "main.rs"), []byte("fn main() {\n    println!(\"hello\");\n}\n"), 0644))

		files, err := services.ScanLegacyFiles(tempDir)
		require.NoError(t, err)
		assert.Empty(t, files)

		isGreenfield, legacyFiles, err := services.IsGreenfieldWorkspace(tempDir)
		require.NoError(t, err)
		assert.True(t, isGreenfield)
		assert.Empty(t, legacyFiles)
	})

	t.Run("when workspace has small files totaling less than 50 lines, it classifies as greenfield", func(t *testing.T) {
		tempDir := t.TempDir()
		require.NoError(t, os.WriteFile(filepath.Join(tempDir, "SPEC.md"), []byte("# Spec"), 0644))

		// 15 lines of starter code
		var lines []string
		for i := 1; i <= 15; i++ {
			lines = append(lines, "x = "+string(rune('0'+i)))
		}
		require.NoError(t, os.WriteFile(filepath.Join(tempDir, "starter.py"), []byte(strings.Join(lines, "\n")), 0644))

		files, err := services.ScanLegacyFiles(tempDir)
		require.NoError(t, err)
		assert.Empty(t, files)

		isGreenfield, _, err := services.IsGreenfieldWorkspace(tempDir)
		require.NoError(t, err)
		assert.True(t, isGreenfield)
	})

	t.Run("when workspace has substantial legacy code (>= 50 lines), it detects legacy files", func(t *testing.T) {
		tempDir := t.TempDir()
		require.NoError(t, os.WriteFile(filepath.Join(tempDir, "SPEC.md"), []byte("# Spec"), 0644))

		// Create 55 lines of legacy code
		var codeLines []string
		codeLines = append(codeLines, "class LegacyService:")
		for i := 1; i <= 27; i++ {
			codeLines = append(codeLines, "    def method_"+string(rune('a'+i))+strings.Repeat("x", 2)+"(self):")
			codeLines = append(codeLines, "        return "+string(rune('0'+(i%10))))
		}
		require.NoError(t, os.WriteFile(filepath.Join(tempDir, "legacy_service.py"), []byte(strings.Join(codeLines, "\n")), 0644))

		files, err := services.ScanLegacyFiles(tempDir)
		require.NoError(t, err)
		assert.Contains(t, files, "legacy_service.py")

		isGreenfield, legacyFiles, err := services.IsGreenfieldWorkspace(tempDir)
		require.NoError(t, err)
		assert.False(t, isGreenfield)
		assert.Equal(t, []string{"legacy_service.py"}, legacyFiles)
	})
}
