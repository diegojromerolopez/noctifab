package services

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSyntaxValidator(t *testing.T) {
	v := NewSyntaxValidator()
	tmpDir := t.TempDir()

	t.Run("valid Go file passes", func(t *testing.T) {
		p := filepath.Join(tmpDir, "valid.go")
		err := os.WriteFile(p, []byte("package main\n\nfunc main() {\n\tprintln(\"hello\")\n}\n"), 0600)
		require.NoError(t, err)

		violation, err := v.ValidateFile(context.Background(), p)
		require.NoError(t, err)
		assert.Nil(t, violation)
	})

	t.Run("invalid Go file fails with violation", func(t *testing.T) {
		p := filepath.Join(tmpDir, "invalid.go")
		err := os.WriteFile(p, []byte("package main\n\nfunc main( {\n"), 0600)
		require.NoError(t, err)

		violation, err := v.ValidateFile(context.Background(), p)
		require.NoError(t, err)
		require.NotNil(t, violation)
		assert.Contains(t, violation.Message, "Go syntax error")
	})

	t.Run("Go file with stub implementation fails anti-stub gate", func(t *testing.T) {
		p := filepath.Join(tmpDir, "stub.go")
		err := os.WriteFile(p, []byte("package main\n\nfunc DoWork() error {\n\treturn nil\n}\n"), 0600)
		require.NoError(t, err)

		violation, err := v.ValidateFile(context.Background(), p)
		require.NoError(t, err)
		require.NotNil(t, violation)
		assert.Contains(t, violation.Message, "anti-stub AST violation")
	})

	t.Run("Go file with undeclared import fails manifest gate", func(t *testing.T) {
		projDir := filepath.Join(tmpDir, "go_project")
		require.NoError(t, os.MkdirAll(projDir, 0755))
		require.NoError(t, os.WriteFile(filepath.Join(projDir, "go.mod"), []byte("module myapp\n\ngo 1.25\n"), 0600))

		p := filepath.Join(projDir, "app.go")
		code := `package main
import "github.com/nonexistent/hallucinated-package"
func Run() {
	println("running")
}`
		require.NoError(t, os.WriteFile(p, []byte(code), 0600))

		violation, err := v.ValidateFile(context.Background(), p)
		require.NoError(t, err)
		require.NotNil(t, violation)
		assert.Contains(t, violation.Message, "hallucinated import")
	})

	t.Run("Python file with pass stub fails anti-stub gate", func(t *testing.T) {
		p := filepath.Join(tmpDir, "stub.py")
		err := os.WriteFile(p, []byte("def calculate():\n    pass\n"), 0600)
		require.NoError(t, err)

		violation, err := v.ValidateFile(context.Background(), p)
		require.NoError(t, err)
		require.NotNil(t, violation)
		assert.Contains(t, violation.Message, "anti-stub AST violation")
	})

	t.Run("valid JSON file passes", func(t *testing.T) {
		p := filepath.Join(tmpDir, "valid.json")
		err := os.WriteFile(p, []byte(`{"key": "value", "num": 42}`), 0600)
		require.NoError(t, err)

		violation, err := v.ValidateFile(context.Background(), p)
		require.NoError(t, err)
		assert.Nil(t, violation)
	})

	t.Run("invalid JSON file fails with violation", func(t *testing.T) {
		p := filepath.Join(tmpDir, "invalid.json")
		err := os.WriteFile(p, []byte(`{"key": "value", trailing`), 0600)
		require.NoError(t, err)

		violation, err := v.ValidateFile(context.Background(), p)
		require.NoError(t, err)
		require.NotNil(t, violation)
		assert.Contains(t, violation.Message, "JSON syntax error")
	})

	t.Run("valid Python file passes if python3 available", func(t *testing.T) {
		p := filepath.Join(tmpDir, "valid.py")
		err := os.WriteFile(p, []byte("def hello():\n    x = 42\n    return x\n"), 0600)
		require.NoError(t, err)

		violation, err := v.ValidateFile(context.Background(), p)
		require.NoError(t, err)
		assert.Nil(t, violation)
	})

	t.Run("Check on valid and invalid files", func(t *testing.T) {
		validP := filepath.Join(tmpDir, "check_valid.go")
		err := os.WriteFile(validP, []byte("package main\n\nfunc Run() {\n\tprintln(1)\n}\n"), 0600)
		require.NoError(t, err)
		assert.NoError(t, v.Check(context.Background(), validP))

		invalidP := filepath.Join(tmpDir, "check_invalid.go")
		err = os.WriteFile(invalidP, []byte("package main\nfunc {"), 0600)
		require.NoError(t, err)
		assert.Error(t, v.Check(context.Background(), invalidP))
	})

	t.Run("test file calling missing facade method fails validation", func(t *testing.T) {
		tmpDir := t.TempDir()
		srcDir := filepath.Join(tmpDir, "src")
		testDir := filepath.Join(tmpDir, "tests")
		require.NoError(t, os.MkdirAll(srcDir, 0755))
		require.NoError(t, os.MkdirAll(testDir, 0755))
		require.NoError(t, os.WriteFile(filepath.Join(tmpDir, "pyproject.toml"), []byte("[project]\n"), 0644))

		srcFile := filepath.Join(srcDir, "store.py")
		require.NoError(t, os.WriteFile(srcFile, []byte("class Store:\n    def get(self, k):\n        pass\n"), 0644))

		testFile := filepath.Join(testDir, "test_store.py")
		testContent := "class TestStore:\n    def test_run(self):\n        s = Store()\n        s.get_hash('x')\n"
		require.NoError(t, os.WriteFile(testFile, []byte(testContent), 0644))

		v := NewSyntaxValidator()
		violation, err := v.ValidateFile(context.Background(), testFile)
		require.NoError(t, err)
		require.NotNil(t, violation)
		assert.Contains(t, violation.Message, "missing method 'get_hash'")
	})

	t.Run("test file with bare dispatch assertion fails diagnostic richness check", func(t *testing.T) {
		tmpDir := t.TempDir()
		testFile := filepath.Join(tmpDir, "test_bare.py")
		testContent := "class TestBare:\n    def test_run(self):\n        self.assertEqual(b'+OK', self.dispatch('PING'))\n"
		require.NoError(t, os.WriteFile(testFile, []byte(testContent), 0644))

		v := NewSyntaxValidator()
		violation, err := v.ValidateFile(context.Background(), testFile)
		require.NoError(t, err)
		require.NotNil(t, violation)
		assert.Contains(t, violation.Message, "lacks descriptive msg=")
	})
}
