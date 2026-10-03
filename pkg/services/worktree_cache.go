package services

import (
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// sharedDependencyDirs defines directory names to project via symlinks into task worktrees
// when they exist in the root project workspace.
var sharedDependencyDirs = []string{
	"node_modules",
	".venv",
	"venv",
	"vendor",
	".bundle",
	"gradle",
	".mvn",
	"deps",
	"_opam",
}

// sharedWrapperFiles defines executable wrapper scripts to project into worktrees.
var sharedWrapperFiles = []string{
	"gradlew",
	"gradlew.bat",
	"mvnw",
	"mvnw.cmd",
}

// SeedTaskWorktreeWorkspace synchronizes manifests, projects dependency symlinks,
// and configures toolchain cache paths for an isolated task worktree.
func SeedTaskWorktreeWorkspace(srcDir, dstDir string) {
	// 1. Synchronize root project manifests
	syncRootManifests(srcDir, dstDir)

	// 2. Project existing dependency directories and wrapper scripts via symlinks
	SymlinkSharedDependencies(srcDir, dstDir)

	// 3. Configure toolchain-specific cache paths and build cache flags
	ConfigureToolchainWorktreeCaches(srcDir, dstDir)
}

// SymlinkSharedDependencies projects root dependency directories and wrapper scripts
// into the destination worktree using relative symlinks whenever possible.
func SymlinkSharedDependencies(srcDir, dstDir string) {
	cleanSrc := filepath.Clean(srcDir)
	cleanDst := filepath.Clean(dstDir)
	if cleanSrc == cleanDst {
		return
	}

	// Project directories
	for _, dir := range sharedDependencyDirs {
		srcPath := filepath.Join(cleanSrc, dir)
		dstPath := filepath.Join(cleanDst, dir)
		if info, err := os.Stat(srcPath); err == nil && info.IsDir() {
			if _, errDst := os.Lstat(dstPath); os.IsNotExist(errDst) {
				_ = createRelativeOrAbsoluteSymlink(srcPath, dstPath)
			}
		} else if dir == "node_modules" && hasNodeManifest(cleanSrc, cleanDst) {
			// Global node_modules fallback if available in environment (e.g. containers) for Node.js projects
			globalNode := "/usr/local/lib/node_modules"
			if gInfo, gErr := os.Stat(globalNode); gErr == nil && gInfo.IsDir() {
				if _, errDst := os.Lstat(dstPath); os.IsNotExist(errDst) {
					_ = os.Symlink(globalNode, dstPath)
				}
			}
		}
	}

	// Project wrapper scripts
	for _, file := range sharedWrapperFiles {
		srcPath := filepath.Join(cleanSrc, file)
		dstPath := filepath.Join(cleanDst, file)
		if info, err := os.Stat(srcPath); err == nil && !info.IsDir() {
			if _, errDst := os.Lstat(dstPath); os.IsNotExist(errDst) {
				_ = createRelativeOrAbsoluteSymlink(srcPath, dstPath)
			}
		}
	}
}

// sharedToolchainCacheDirs defines all cache directories to pre-warm under .noctifab/cache.
var sharedToolchainCacheDirs = []string{
	"cargo-target",
	"go-cache",
	"gradle",
	"m2-repo",
	"pip",
	"npm",
	"ccache",
	"bundle",
	"nuget",
	"composer",
	"dune",
	"hex",
	"mix",
}

// PrecreateSharedCacheDirs ensures all supported toolchain cache directories exist.
func PrecreateSharedCacheDirs(cacheBase string) {
	_ = os.MkdirAll(cacheBase, 0755)
	for _, sub := range sharedToolchainCacheDirs {
		_ = os.MkdirAll(filepath.Join(cacheBase, sub), 0755)
	}
}

// ConfigureToolchainWorktreeCaches configures build caches for Rust, Gradle, and other toolchains.
func ConfigureToolchainWorktreeCaches(srcDir, dstDir string) {
	cleanSrc := filepath.Clean(srcDir)
	cleanDst := filepath.Clean(dstDir)

	sharedCacheBase := filepath.Join(cleanSrc, ".noctifab", "cache")
	PrecreateSharedCacheDirs(sharedCacheBase)

	// 1. Rust Cargo: redirect target-dir to shared cache via .cargo/config.toml
	if _, err := os.Stat(filepath.Join(cleanSrc, "Cargo.toml")); err == nil {
		cargoCacheDir := filepath.Join(sharedCacheBase, "cargo-target")
		_ = os.MkdirAll(cargoCacheDir, 0755)

		cargoConfigDir := filepath.Join(cleanDst, ".cargo")
		_ = os.MkdirAll(cargoConfigDir, 0755)
		configFile := filepath.Join(cargoConfigDir, "config.toml")

		if _, err := os.Stat(configFile); os.IsNotExist(err) {
			relTarget, errRel := filepath.Rel(cleanDst, cargoCacheDir)
			targetVal := cargoCacheDir
			if errRel == nil {
				targetVal = filepath.ToSlash(relTarget)
			}
			configContent := fmt.Sprintf("[build]\ntarget-dir = %q\n", targetVal)
			_ = os.WriteFile(configFile, []byte(configContent), 0644)
		}
	}

	// 2. Gradle: enable task build cache in gradle.properties
	if hasGradleManifest(cleanSrc) {
		gradlePropsFile := filepath.Join(cleanDst, "gradle.properties")
		if data, err := os.ReadFile(gradlePropsFile); err == nil {
			content := string(data)
			if !strings.Contains(content, "org.gradle.caching") {
				newContent := content + "\norg.gradle.caching=true\n"
				_ = os.WriteFile(gradlePropsFile, []byte(newContent), 0644)
			}
		} else {
			_ = os.WriteFile(gradlePropsFile, []byte("org.gradle.caching=true\n"), 0644)
		}
	}
}

func hasGradleManifest(dir string) bool {
	for _, f := range []string{"build.gradle", "build.gradle.kts", "settings.gradle", "settings.gradle.kts"} {
		if _, err := os.Stat(filepath.Join(dir, f)); err == nil {
			return true
		}
	}
	return false
}

func hasNodeManifest(dirs ...string) bool {
	for _, dir := range dirs {
		if _, err := os.Stat(filepath.Join(dir, "package.json")); err == nil {
			return true
		}
	}
	return false
}

func createRelativeOrAbsoluteSymlink(srcPath, dstPath string) error {
	dstDir := filepath.Dir(dstPath)
	_ = os.MkdirAll(dstDir, 0755)

	// If a dead symlink or stale target exists at dstPath, remove it before linking
	if _, err := os.Lstat(dstPath); err == nil {
		_ = os.Remove(dstPath)
	}

	rel, err := filepath.Rel(dstDir, srcPath)
	if err == nil {
		return os.Symlink(rel, dstPath)
	}
	return os.Symlink(srcPath, dstPath)
}

// ResolveRootProjectDir inspects projectPath and extracts the root project directory
// if the given path is an isolated worktree under .noctifab/worktrees/.
func ResolveRootProjectDir(projectPath string) string {
	cleanPath := filepath.Clean(projectPath)
	slashPath := filepath.ToSlash(cleanPath)
	parts := strings.Split(slashPath, "/")

	for i := 0; i < len(parts); i++ {
		if parts[i] == ".noctifab" && i+1 < len(parts) && parts[i+1] == "worktrees" {
			root := strings.Join(parts[:i], "/")
			if root == "" && strings.HasPrefix(slashPath, "/") {
				return "/"
			}
			if root != "" {
				return filepath.FromSlash(root)
			}
		}
	}
	return cleanPath
}

// BuildSharedCacheEnv constructs the environment variable slice configuring shared caches
// across all supported language ecosystems based on the root project directory.
func BuildSharedCacheEnv(rootProjectDir string) []string {
	cleanRoot := filepath.Clean(rootProjectDir)
	cacheBase := filepath.Join(cleanRoot, ".noctifab", "cache")
	PrecreateSharedCacheDirs(cacheBase)

	env := []string{
		fmt.Sprintf("CARGO_TARGET_DIR=%s", filepath.Join(cacheBase, "cargo-target")),
		fmt.Sprintf("GOCACHE=%s", filepath.Join(cacheBase, "go-cache")),
		fmt.Sprintf("GRADLE_USER_HOME=%s", filepath.Join(cacheBase, "gradle")),
		fmt.Sprintf("MAVEN_OPTS=-Dmaven.repo.local=%s", filepath.Join(cacheBase, "m2-repo")),
		fmt.Sprintf("PIP_CACHE_DIR=%s", filepath.Join(cacheBase, "pip")),
		fmt.Sprintf("npm_config_cache=%s", filepath.Join(cacheBase, "npm")),
		fmt.Sprintf("CCACHE_DIR=%s", filepath.Join(cacheBase, "ccache")),
		fmt.Sprintf("BUNDLE_PATH=%s", filepath.Join(cacheBase, "bundle")),
		fmt.Sprintf("NUGET_PACKAGES=%s", filepath.Join(cacheBase, "nuget")),
		fmt.Sprintf("COMPOSER_CACHE_DIR=%s", filepath.Join(cacheBase, "composer")),
		"DUNE_CACHE=enabled",
		fmt.Sprintf("DUNE_CACHE_ROOT=%s", filepath.Join(cacheBase, "dune")),
		fmt.Sprintf("HEX_HOME=%s", filepath.Join(cacheBase, "hex")),
		fmt.Sprintf("MIX_HOME=%s", filepath.Join(cacheBase, "mix")),
		"DOCKER_BUILDKIT=1",
		"COMPOSE_DOCKER_CLI_BUILD=1",
	}

	testTimeout := os.Getenv("NOCTIFAB_TEST_TIMEOUT")
	if testTimeout == "" {
		testTimeout = os.Getenv("TEST_TIMEOUT")
	}
	if testTimeout == "" {
		testTimeout = "0.5"
	}
	env = append(env,
		fmt.Sprintf("NOCTIFAB_TEST_TIMEOUT=%s", testTimeout),
		fmt.Sprintf("TEST_TIMEOUT=%s", testTimeout),
	)

	// Python virtual environment projection: ensure pip is seeded and prepend bin to PATH
	for _, venvName := range []string{".venv", "venv"} {
		venvPath := filepath.Join(cleanRoot, venvName)
		if info, err := os.Stat(venvPath); err == nil && info.IsDir() {
			EnsureVenvPip(venvPath)
			venvBin := filepath.Join(venvPath, "bin")
			env = append(env,
				fmt.Sprintf("VIRTUAL_ENV=%s", venvPath),
				fmt.Sprintf("PATH=%s:%s", venvBin, os.Getenv("PATH")),
			)
			break
		}
	}

	// Dynamic ephemeral port allocation to prevent port collisions in parallel integration tests
	port := allocateWorktreePort()
	env = append(env,
		fmt.Sprintf("PORT=%d", port),
		fmt.Sprintf("TEST_PORT=%d", port),
		fmt.Sprintf("REDIS_PORT=%d", port),
		fmt.Sprintf("SERVER_PORT=%d", port),
		fmt.Sprintf("NOCTIFAB_PORT=%d", port),
	)

	return env
}

// allocateWorktreePort finds an available ephemeral TCP port for isolated parallel testing.
func allocateWorktreePort() int {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 16379
	}
	defer func() { _ = l.Close() }()
	return l.Addr().(*net.TCPAddr).Port
}

// EnsureVenvPip checks if a Python virtual environment has pip installed in its bin directory.
// If pip is missing (e.g. from an unseeded uv venv or minimal Python 3.13 venv), it automatically seeds it.
func EnsureVenvPip(venvPath string) {
	venvBin := filepath.Join(venvPath, "bin")
	pipPath := filepath.Join(venvBin, "pip")
	if info, err := os.Stat(pipPath); err == nil && !info.IsDir() {
		return // pip already present
	}

	pythonPath := filepath.Join(venvBin, "python3")
	if _, err := os.Stat(pythonPath); err != nil {
		pythonPath = filepath.Join(venvBin, "python")
		if _, err := os.Stat(pythonPath); err != nil {
			return // not a valid python venv
		}
	}

	// 1. Try uv pip install pip --python <pythonPath>
	if _, err := exec.LookPath("uv"); err == nil {
		cmd := exec.Command("uv", "pip", "install", "pip", "--python", pythonPath)
		if err := cmd.Run(); err == nil {
			return
		}
	}

	// 2. Fallback to python3 -m ensurepip --upgrade
	cmd := exec.Command(pythonPath, "-m", "ensurepip", "--upgrade")
	_ = cmd.Run()
}
