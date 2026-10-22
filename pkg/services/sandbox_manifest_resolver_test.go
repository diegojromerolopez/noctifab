package services

import (
	"os"
	"path/filepath"
	"testing"
)

func TestResolveSubdirectoryManifest(t *testing.T) {
	t.Run("returns targetDir when pkg is specified", func(t *testing.T) {
		tempDir := t.TempDir()
		res := ResolveSubdirectoryManifest(tempDir, "cargo", "my_pkg")
		if res != tempDir {
			t.Errorf("expected %s, got %s", tempDir, res)
		}
	})

	t.Run("returns targetDir when binary has no manifest pattern", func(t *testing.T) {
		tempDir := t.TempDir()
		res := ResolveSubdirectoryManifest(tempDir, "unknown_tool", "")
		if res != tempDir {
			t.Errorf("expected %s, got %s", tempDir, res)
		}
	})

	t.Run("returns root when manifest is already in root", func(t *testing.T) {
		tempDir := t.TempDir()
		if err := os.WriteFile(filepath.Join(tempDir, "go.mod"), []byte("module test\n"), 0644); err != nil {
			t.Fatal(err)
		}
		res := ResolveSubdirectoryManifest(tempDir, "go", "")
		if res != tempDir {
			t.Errorf("expected %s, got %s", tempDir, res)
		}
	})

	t.Run("auto-detects Cargo.toml in immediate subdirectory", func(t *testing.T) {
		tempDir := t.TempDir()
		subDir := filepath.Join(tempDir, "crates", "core")
		_ = os.MkdirAll(subDir, 0755)
		nestedDir := filepath.Join(tempDir, "server")
		_ = os.MkdirAll(nestedDir, 0755)
		if err := os.WriteFile(filepath.Join(nestedDir, "Cargo.toml"), []byte("[package]\nname=\"server\"\n"), 0644); err != nil {
			t.Fatal(err)
		}

		res := ResolveSubdirectoryManifest(tempDir, "cargo", "")
		if res != nestedDir {
			t.Errorf("expected %s, got %s", nestedDir, res)
		}
	})

	t.Run("auto-detects package.json in frontend subdirectory", func(t *testing.T) {
		tempDir := t.TempDir()
		frontendDir := filepath.Join(tempDir, "frontend")
		_ = os.MkdirAll(frontendDir, 0755)
		if err := os.WriteFile(filepath.Join(frontendDir, "package.json"), []byte(`{"name":"web"}`), 0644); err != nil {
			t.Fatal(err)
		}

		res := ResolveSubdirectoryManifest(tempDir, "npm", "")
		if res != frontendDir {
			t.Errorf("expected %s, got %s", frontendDir, res)
		}
	})

	t.Run("auto-detects pyproject.toml for pytest in backend subdirectory", func(t *testing.T) {
		tempDir := t.TempDir()
		backendDir := filepath.Join(tempDir, "backend")
		_ = os.MkdirAll(backendDir, 0755)
		if err := os.WriteFile(filepath.Join(backendDir, "pyproject.toml"), []byte("[project]\nname=\"api\"\n"), 0644); err != nil {
			t.Fatal(err)
		}

		res := ResolveSubdirectoryManifest(tempDir, "pytest", "")
		if res != backendDir {
			t.Errorf("expected %s, got %s", backendDir, res)
		}
	})

	t.Run("auto-detects csproj for dotnet in subfolder", func(t *testing.T) {
		tempDir := t.TempDir()
		csDir := filepath.Join(tempDir, "MyService")
		_ = os.MkdirAll(csDir, 0755)
		if err := os.WriteFile(filepath.Join(csDir, "MyService.csproj"), []byte("<Project />"), 0644); err != nil {
			t.Fatal(err)
		}

		res := ResolveSubdirectoryManifest(tempDir, "dotnet", "")
		if res != csDir {
			t.Errorf("expected %s, got %s", csDir, res)
		}
	})

	t.Run("ignores hidden and build directories like .git, node_modules, target", func(t *testing.T) {
		tempDir := t.TempDir()
		gitDir := filepath.Join(tempDir, ".git")
		_ = os.MkdirAll(gitDir, 0755)
		_ = os.WriteFile(filepath.Join(gitDir, "package.json"), []byte("{}"), 0644)

		targetDir := filepath.Join(tempDir, "target")
		_ = os.MkdirAll(targetDir, 0755)
		_ = os.WriteFile(filepath.Join(targetDir, "package.json"), []byte("{}"), 0644)

		res := ResolveSubdirectoryManifest(tempDir, "npm", "")
		if res != tempDir {
			t.Errorf("expected %s to be ignored and fallback to %s, got %s", targetDir, tempDir, res)
		}
	})
}

func TestDetectDefaultCommands_Subdirectories(t *testing.T) {
	t.Run("auto-detects Cargo.toml in backend subdirectory", func(t *testing.T) {
		tempDir := t.TempDir()
		backendDir := filepath.Join(tempDir, "backend")
		_ = os.MkdirAll(backendDir, 0755)
		_ = os.WriteFile(filepath.Join(backendDir, "Cargo.toml"), []byte("[package]\nname=\"backend\"\n"), 0644)

		if got := DetectDefaultTestCommand(tempDir); got != "cargo test" {
			t.Errorf("expected 'cargo test', got '%s'", got)
		}
		if got := DetectDefaultBuildCommand(tempDir); got != "cargo check" {
			t.Errorf("expected 'cargo check', got '%s'", got)
		}
		if got := DetectDefaultFormatterCommand(tempDir); got != "cargo fmt" {
			t.Errorf("expected 'cargo fmt', got '%s'", got)
		}
	})

	t.Run("auto-detects package.json in frontend subdirectory", func(t *testing.T) {
		tempDir := t.TempDir()
		frontendDir := filepath.Join(tempDir, "frontend")
		_ = os.MkdirAll(frontendDir, 0755)
		pkgJSON := `{"scripts": {"build": "vite build", "format": "prettier --write ."}}`
		_ = os.WriteFile(filepath.Join(frontendDir, "package.json"), []byte(pkgJSON), 0644)

		if got := DetectDefaultTestCommand(tempDir); got != "npm test" {
			t.Errorf("expected 'npm test', got '%s'", got)
		}
		if got := DetectDefaultBuildCommand(tempDir); got != "npm run build" {
			t.Errorf("expected 'npm run build', got '%s'", got)
		}
		if got := DetectDefaultFormatterCommand(tempDir); got != "npm run format" {
			t.Errorf("expected 'npm run format', got '%s'", got)
		}
	})

	t.Run("auto-detects pyproject.toml in service subdirectory", func(t *testing.T) {
		tempDir := t.TempDir()
		serviceDir := filepath.Join(tempDir, "service")
		_ = os.MkdirAll(serviceDir, 0755)
		_ = os.WriteFile(filepath.Join(serviceDir, "pyproject.toml"), []byte("[project]\nname=\"svc\"\n"), 0644)

		if got := DetectDefaultTestCommand(tempDir); got != "python3 -m unittest discover -s tests" {
			t.Errorf("expected python unittest command, got '%s'", got)
		}
	})

	t.Run("auto-detects go.mod in cmd subdirectory", func(t *testing.T) {
		tempDir := t.TempDir()
		cmdDir := filepath.Join(tempDir, "cmd")
		_ = os.MkdirAll(cmdDir, 0755)
		_ = os.WriteFile(filepath.Join(cmdDir, "go.mod"), []byte("module mycmd\n"), 0644)

		if got := DetectDefaultTestCommand(tempDir); got != "go test -v ./..." {
			t.Errorf("expected 'go test -v ./...', got '%s'", got)
		}
		if got := DetectDefaultBuildCommand(tempDir); got != "go build ./..." {
			t.Errorf("expected 'go build ./...', got '%s'", got)
		}
		if got := DetectDefaultFormatterCommand(tempDir); got != "go fmt ./..." {
			t.Errorf("expected 'go fmt ./...', got '%s'", got)
		}
	})

	t.Run("ignores manifests in node_modules and .git", func(t *testing.T) {
		tempDir := t.TempDir()
		_ = os.MkdirAll(filepath.Join(tempDir, "node_modules"), 0755)
		_ = os.WriteFile(filepath.Join(tempDir, "node_modules", "package.json"), []byte("{}"), 0644)

		if got := DetectDefaultBuildCommand(tempDir); got != "" {
			t.Errorf("expected empty build command for ignored node_modules, got '%s'", got)
		}
	})
}

