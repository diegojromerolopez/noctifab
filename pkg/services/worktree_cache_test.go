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

func TestResolveRootProjectDir(t *testing.T) {
	t.Run("when path is a regular project root, it returns the path unmodified", func(t *testing.T) {
		path := filepath.Join("home", "user", "myproject")
		assert.Equal(t, path, services.ResolveRootProjectDir(path))
	})

	t.Run("when path is inside .noctifab/worktrees/task-id, it extracts the root project directory", func(t *testing.T) {
		root := filepath.Join("home", "user", "myproject")
		worktree := filepath.Join(root, ".noctifab", "worktrees", "task-abc-123")
		assert.Equal(t, root, services.ResolveRootProjectDir(worktree))
	})

	t.Run("when path is nested inside a worktree package, it resolves root", func(t *testing.T) {
		root := filepath.Join("home", "user", "myproject")
		worktreeSub := filepath.Join(root, ".noctifab", "worktrees", "task-abc-123", "pkg", "sub")
		assert.Equal(t, root, services.ResolveRootProjectDir(worktreeSub))
	})
}

func TestBuildSharedCacheEnv(t *testing.T) {
	tempDir := t.TempDir()

	t.Run("when root project directory is given, it includes toolchain cache environment variables", func(t *testing.T) {
		env := services.BuildSharedCacheEnv(tempDir)
		envMap := make(map[string]string)
		for _, e := range env {
			parts := strings.SplitN(e, "=", 2)
			if len(parts) == 2 {
				envMap[parts[0]] = parts[1]
			}
		}

		cacheBase := filepath.Join(tempDir, ".noctifab", "cache")

		assert.Equal(t, filepath.Join(cacheBase, "cargo-target"), envMap["CARGO_TARGET_DIR"])
		assert.Equal(t, filepath.Join(cacheBase, "go-cache"), envMap["GOCACHE"])
		assert.Equal(t, filepath.Join(cacheBase, "gradle"), envMap["GRADLE_USER_HOME"])
		assert.Contains(t, envMap["MAVEN_OPTS"], filepath.Join(cacheBase, "m2-repo"))
		assert.Equal(t, filepath.Join(cacheBase, "pip"), envMap["PIP_CACHE_DIR"])
		assert.Equal(t, filepath.Join(cacheBase, "npm"), envMap["npm_config_cache"])
		assert.Equal(t, filepath.Join(cacheBase, "ccache"), envMap["CCACHE_DIR"])
		assert.Equal(t, filepath.Join(cacheBase, "bundle"), envMap["BUNDLE_PATH"])
		assert.Equal(t, filepath.Join(cacheBase, "nuget"), envMap["NUGET_PACKAGES"])
		assert.Equal(t, filepath.Join(cacheBase, "composer"), envMap["COMPOSER_CACHE_DIR"])
		assert.Equal(t, "enabled", envMap["DUNE_CACHE"])
		assert.Equal(t, filepath.Join(cacheBase, "dune"), envMap["DUNE_CACHE_ROOT"])
		assert.Equal(t, filepath.Join(cacheBase, "hex"), envMap["HEX_HOME"])
		assert.Equal(t, filepath.Join(cacheBase, "mix"), envMap["MIX_HOME"])
	})

	t.Run("when .venv exists in root, it injects VIRTUAL_ENV and prepends PATH", func(t *testing.T) {
		venvDir := filepath.Join(tempDir, ".venv")
		venvBin := filepath.Join(venvDir, "bin")
		require.NoError(t, os.MkdirAll(venvBin, 0755))

		env := services.BuildSharedCacheEnv(tempDir)
		envMap := make(map[string]string)
		for _, e := range env {
			parts := strings.SplitN(e, "=", 2)
			if len(parts) == 2 {
				envMap[parts[0]] = parts[1]
			}
		}

		assert.Equal(t, venvDir, envMap["VIRTUAL_ENV"])
		assert.True(t, strings.HasPrefix(envMap["PATH"], venvBin))
	})
}

func TestSymlinkSharedDependencies(t *testing.T) {
	tempDir := t.TempDir()
	srcDir := filepath.Join(tempDir, "root")
	dstDir := filepath.Join(tempDir, "worktree")
	require.NoError(t, os.MkdirAll(srcDir, 0755))
	require.NoError(t, os.MkdirAll(dstDir, 0755))

	t.Run("when root directory has node_modules, it creates a symlink in worktree", func(t *testing.T) {
		rootNodeModules := filepath.Join(srcDir, "node_modules")
		require.NoError(t, os.MkdirAll(filepath.Join(rootNodeModules, "express"), 0755))
		require.NoError(t, os.WriteFile(filepath.Join(rootNodeModules, "express", "index.js"), []byte("console.log('express');"), 0644))

		services.SymlinkSharedDependencies(srcDir, dstDir)

		dstNodeModules := filepath.Join(dstDir, "node_modules")
		info, err := os.Lstat(dstNodeModules)
		require.NoError(t, err)
		assert.True(t, info.Mode()&os.ModeSymlink != 0)

		// Verify files are readable through the symlink
		expressIndex := filepath.Join(dstNodeModules, "express", "index.js")
		content, err := os.ReadFile(expressIndex)
		require.NoError(t, err)
		assert.Equal(t, "console.log('express');", string(content))
	})

	t.Run("when root directory has gradle and gradlew, it creates symlinks in worktree", func(t *testing.T) {
		rootGradle := filepath.Join(srcDir, "gradle")
		require.NoError(t, os.MkdirAll(filepath.Join(rootGradle, "wrapper"), 0755))
		require.NoError(t, os.WriteFile(filepath.Join(rootGradle, "wrapper", "gradle-wrapper.jar"), []byte("dummy-jar"), 0644))
		require.NoError(t, os.WriteFile(filepath.Join(srcDir, "gradlew"), []byte("#!/bin/sh\n"), 0755))

		services.SymlinkSharedDependencies(srcDir, dstDir)

		dstGradle := filepath.Join(dstDir, "gradle")
		info, err := os.Lstat(dstGradle)
		require.NoError(t, err)
		assert.True(t, info.Mode()&os.ModeSymlink != 0)

		dstGradlew := filepath.Join(dstDir, "gradlew")
		infoFile, err := os.Lstat(dstGradlew)
		require.NoError(t, err)
		assert.True(t, infoFile.Mode()&os.ModeSymlink != 0)
	})

	t.Run("when worktree is removed, unlinking does not delete root directory contents", func(t *testing.T) {
		require.NoError(t, os.RemoveAll(dstDir))
		// Root node_modules must remain intact
		rootExpress := filepath.Join(srcDir, "node_modules", "express", "index.js")
		assert.FileExists(t, rootExpress)
	})
}

func TestConfigureToolchainWorktreeCaches(t *testing.T) {
	tempDir := t.TempDir()
	srcDir := filepath.Join(tempDir, "root")
	dstDir := filepath.Join(tempDir, "worktree")
	require.NoError(t, os.MkdirAll(srcDir, 0755))
	require.NoError(t, os.MkdirAll(dstDir, 0755))

	t.Run("when Cargo.toml exists, it creates .cargo/config.toml pointing to shared target-dir", func(t *testing.T) {
		require.NoError(t, os.WriteFile(filepath.Join(srcDir, "Cargo.toml"), []byte("[package]\nname = \"test\"\n"), 0644))

		services.ConfigureToolchainWorktreeCaches(srcDir, dstDir)

		cargoConfigFile := filepath.Join(dstDir, ".cargo", "config.toml")
		require.FileExists(t, cargoConfigFile)

		content, err := os.ReadFile(cargoConfigFile)
		require.NoError(t, err)
		assert.Contains(t, string(content), "[build]")
		assert.Contains(t, string(content), "target-dir")
		assert.Contains(t, string(content), "cargo-target")
	})

	t.Run("when build.gradle exists, it enables org.gradle.caching in gradle.properties", func(t *testing.T) {
		require.NoError(t, os.WriteFile(filepath.Join(srcDir, "build.gradle"), []byte("plugins { id 'java' }\n"), 0644))

		services.ConfigureToolchainWorktreeCaches(srcDir, dstDir)

		propsFile := filepath.Join(dstDir, "gradle.properties")
		require.FileExists(t, propsFile)

		content, err := os.ReadFile(propsFile)
		require.NoError(t, err)
		assert.Contains(t, string(content), "org.gradle.caching=true")
	})
}

func TestSeedTaskWorktreeWorkspace(t *testing.T) {
	tempDir := t.TempDir()
	srcDir := filepath.Join(tempDir, "root")
	dstDir := filepath.Join(tempDir, "worktree")
	require.NoError(t, os.MkdirAll(srcDir, 0755))
	require.NoError(t, os.MkdirAll(dstDir, 0755))

	// Setup root with manifests and dependencies
	require.NoError(t, os.WriteFile(filepath.Join(srcDir, "package.json"), []byte("{\"name\":\"app\"}\n"), 0644))
	require.NoError(t, os.MkdirAll(filepath.Join(srcDir, "node_modules"), 0755))

	services.SeedTaskWorktreeWorkspace(srcDir, dstDir)

	// Manifest copied
	assert.FileExists(t, filepath.Join(dstDir, "package.json"))
	// Symlink created
	info, err := os.Lstat(filepath.Join(dstDir, "node_modules"))
	require.NoError(t, err)
	assert.True(t, info.Mode()&os.ModeSymlink != 0)
}
