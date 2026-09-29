package services

import (
	"os"
	"path/filepath"
	"strings"
)

// EnsureLanguagePackageMarkers inspects source and test directories across all supported
// languages (Python, Rust, Shell, etc.) and ensures mandatory package markers,
// module anchors, and execution bits are present so test runners don't silently skip test suites.
func EnsureLanguagePackageMarkers(projectPath string) error {
	if projectPath == "" {
		return nil
	}

	// 1. Python package markers (__init__.py) in test and source trees
	ensurePythonPackageMarkers(projectPath)

	// 2. Rust integration test module markers (mod.rs in tests/* submodules)
	ensureRustTestMarkers(projectPath)

	// 3. Shell script executable permissions (chmod +x *.sh)
	ensureShellScriptPermissions(projectPath)

	return nil
}

func ensurePythonPackageMarkers(projectPath string) {
	// Root directories where Python modules and tests reside
	searchRoots := []string{"tests", "test", "spec", "specs", "src", "lib", "app"}

	for _, root := range searchRoots {
		rootPath := filepath.Join(projectPath, root)
		info, err := os.Stat(rootPath)
		if err != nil || !info.IsDir() {
			continue
		}

		// Ensure root test directory itself has __init__.py if it contains tests or subdirectories
		if root == "tests" || root == "test" || root == "spec" || root == "specs" {
			rootInit := filepath.Join(rootPath, "__init__.py")
			if _, statErr := os.Stat(rootInit); os.IsNotExist(statErr) {
				_ = os.WriteFile(rootInit, []byte(""), 0644)
			}
		}

		_ = filepath.Walk(rootPath, func(path string, fi os.FileInfo, walkErr error) error {
			if walkErr != nil || !fi.IsDir() {
				return nil
			}

			// Skip cache, version control, and virtualenv folders
			base := fi.Name()
			if strings.HasPrefix(base, ".") || base == "node_modules" || base == "__pycache__" ||
				base == "target" || base == "vendor" || base == ".venv" || base == "venv" {
				return filepath.SkipDir
			}

			entries, readErr := os.ReadDir(path)
			if readErr != nil {
				return nil
			}

			hasPythonFiles := false
			hasSubdirs := false
			for _, entry := range entries {
				if entry.IsDir() {
					subBase := entry.Name()
					if !strings.HasPrefix(subBase, ".") && subBase != "__pycache__" {
						hasSubdirs = true
					}
				} else if strings.HasSuffix(entry.Name(), ".py") {
					hasPythonFiles = true
				}
			}

			// In test trees or source trees, directories with .py files or subdirs need __init__.py
			if hasPythonFiles || (hasSubdirs && (root == "tests" || root == "test")) {
				initPath := filepath.Join(path, "__init__.py")
				if _, statErr := os.Stat(initPath); os.IsNotExist(statErr) {
					_ = os.WriteFile(initPath, []byte(""), 0644)
				}
			}

			return nil
		})
	}
}

func ensureRustTestMarkers(projectPath string) {
	testsDir := filepath.Join(projectPath, "tests")
	info, err := os.Stat(testsDir)
	if err != nil || !info.IsDir() {
		return
	}

	// In Rust, files in tests/*.rs are integration test crates.
	// Subdirectories in tests/ (e.g. tests/common/ or tests/helpers/) are modules that require mod.rs.
	entries, err := os.ReadDir(testsDir)
	if err != nil {
		return
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		subName := entry.Name()
		if strings.HasPrefix(subName, ".") || subName == "target" {
			continue
		}
		subPath := filepath.Join(testsDir, subName)
		subEntries, readErr := os.ReadDir(subPath)
		if readErr != nil {
			continue
		}

		hasRustFiles := false
		hasModRs := false
		for _, se := range subEntries {
			if !se.IsDir() && strings.HasSuffix(se.Name(), ".rs") {
				hasRustFiles = true
				if se.Name() == "mod.rs" {
					hasModRs = true
					break
				}
			}
		}

		if hasRustFiles && !hasModRs {
			modPath := filepath.Join(subPath, "mod.rs")
			_ = os.WriteFile(modPath, []byte("// Integration test helper module\n"), 0644)
		}
	}
}

func ensureShellScriptPermissions(projectPath string) {
	searchDirs := []string{"tests", "scripts", "bin", "."}
	for _, dir := range searchDirs {
		targetDir := filepath.Join(projectPath, dir)
		info, err := os.Stat(targetDir)
		if err != nil || !info.IsDir() {
			continue
		}

		if dir == "." {
			// Only check direct root scripts to avoid slow whole-workspace recursive walks
			entries, readErr := os.ReadDir(targetDir)
			if readErr != nil {
				continue
			}
			for _, entry := range entries {
				if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".sh") {
					scriptPath := filepath.Join(targetDir, entry.Name())
					_ = os.Chmod(scriptPath, 0755)
				}
			}
			continue
		}

		_ = filepath.Walk(targetDir, func(path string, fi os.FileInfo, walkErr error) error {
			if walkErr != nil || fi.IsDir() {
				return nil
			}
			if strings.HasSuffix(fi.Name(), ".sh") {
				_ = os.Chmod(path, 0755)
			}
			return nil
		})
	}
}
