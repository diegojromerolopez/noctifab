package services

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestOrphanSourceGuard(t *testing.T) {
	t.Run("detects decoupled source files omitted from Makefile", func(t *testing.T) {
		tmpDir := t.TempDir()
		srcDir := filepath.Join(tmpDir, "src")
		require.NoError(t, os.MkdirAll(srcDir, 0755))

		// Create files: t4.c, store.c, and decoupled http.c, route.c
		require.NoError(t, os.WriteFile(filepath.Join(srcDir, "t4.c"), []byte("int main() {}\n"), 0644))
		require.NoError(t, os.WriteFile(filepath.Join(srcDir, "store.c"), []byte("void store() {}\n"), 0644))
		require.NoError(t, os.WriteFile(filepath.Join(srcDir, "http.c"), []byte("void http() {}\n"), 0644))
		require.NoError(t, os.WriteFile(filepath.Join(srcDir, "route.c"), []byte("void route() {}\n"), 0644))

		// Makefile that only references t4.c and store.c
		makeContent := `
CC = gcc
CFLAGS = -Wall

bin/t4: src/t4.c src/store.c
	$(CC) $(CFLAGS) -o bin/t4 src/t4.c src/store.c
`
		require.NoError(t, os.WriteFile(filepath.Join(tmpDir, "Makefile"), []byte(makeContent), 0644))

		violations, err := DetectOrphanSourceFiles(tmpDir)
		require.NoError(t, err)
		require.Len(t, violations, 2)
		paths := []string{violations[0].Path, violations[1].Path}
		assert.Contains(t, paths, "src/http.c")
		assert.Contains(t, paths, "src/route.c")
	})

	t.Run("permits wildcard Makefile references", func(t *testing.T) {
		tmpDir := t.TempDir()
		srcDir := filepath.Join(tmpDir, "src")
		require.NoError(t, os.MkdirAll(srcDir, 0755))

		require.NoError(t, os.WriteFile(filepath.Join(srcDir, "t4.c"), []byte("int main() {}\n"), 0644))
		require.NoError(t, os.WriteFile(filepath.Join(srcDir, "http.c"), []byte("void http() {}\n"), 0644))

		makeContent := `
SRCS = $(wildcard src/*.c)
bin/t4: $(SRCS)
	gcc -o bin/t4 $(SRCS)
`
		require.NoError(t, os.WriteFile(filepath.Join(tmpDir, "Makefile"), []byte(makeContent), 0644))

		violations, err := DetectOrphanSourceFiles(tmpDir)
		require.NoError(t, err)
		assert.Empty(t, violations)
	})

	t.Run("ignores test files in src/", func(t *testing.T) {
		tmpDir := t.TempDir()
		srcDir := filepath.Join(tmpDir, "src")
		require.NoError(t, os.MkdirAll(srcDir, 0755))

		require.NoError(t, os.WriteFile(filepath.Join(srcDir, "t4.c"), []byte("int main() {}\n"), 0644))
		require.NoError(t, os.WriteFile(filepath.Join(srcDir, "test_t4.c"), []byte("void test() {}\n"), 0644))

		makeContent := `
bin/t4: src/t4.o
	gcc -o bin/t4 src/t4.o
`
		require.NoError(t, os.WriteFile(filepath.Join(tmpDir, "Makefile"), []byte(makeContent), 0644))

		violations, err := DetectOrphanSourceFiles(tmpDir)
		require.NoError(t, err)
		assert.Empty(t, violations)
	})
}
