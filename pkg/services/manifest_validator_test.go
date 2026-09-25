package services

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/diegojromerolopez/noctifab/pkg/domain"
)

func TestParseCargoToml(t *testing.T) {
	content := `
[package]
name = "myproject"
version = "0.1.0"

[dependencies]
tokio = { version = "1.0", features = ["full"] }
serde-json = "1.0"
bytes = "1.4"

[dev-dependencies]
tempfile = "3.3"
`
	deps, err := ParseCargoToml(content)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, ok := deps["tokio"]; !ok {
		t.Errorf("expected tokio in dependencies")
	}
	// serde-json normalized to serde_json
	if _, ok := deps["serde_json"]; !ok {
		t.Errorf("expected serde_json in dependencies")
	}
	if _, ok := deps["tempfile"]; !ok {
		t.Errorf("expected tempfile in dev-dependencies")
	}
}

func TestParsePyprojectToml(t *testing.T) {
	content := `
[project]
name = "pyedis"
version = "0.1.0"
dependencies = [
    "redis>=4.0.0",
    "fastapi",
    "pydantic-settings>=2.0",
]
`
	deps, err := ParsePyprojectToml(content)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, ok := deps["redis"]; !ok {
		t.Errorf("expected redis in dependencies")
	}
	if _, ok := deps["fastapi"]; !ok {
		t.Errorf("expected fastapi in dependencies")
	}
	if _, ok := deps["pydantic_settings"]; !ok {
		t.Errorf("expected pydantic_settings in dependencies")
	}
}

func TestValidateWorkspaceManifests_UndeclaredPython(t *testing.T) {
	tempDir := t.TempDir()

	pyproject := `
[project]
name = "testapp"
dependencies = [
    "requests",
]
`
	_ = os.WriteFile(filepath.Join(tempDir, "pyproject.toml"), []byte(pyproject), 0644)

	// Source file imports requests (declared) and redis (undeclared!)
	srcFile := `
import os
import requests
import redis

def fetch():
    pass
`
	_ = os.WriteFile(filepath.Join(tempDir, "main.py"), []byte(srcFile), 0644)

	res, err := ValidateWorkspaceManifests(tempDir, []string{"main.py"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Valid {
		t.Fatalf("expected validation to fail due to undeclared redis import")
	}
	if len(res.Violations) != 1 {
		t.Fatalf("expected 1 violation, got %d", len(res.Violations))
	}
	if res.Violations[0].Package != "redis" {
		t.Errorf("expected violation on 'redis', got %s", res.Violations[0].Package)
	}
}

func TestValidateWorkspaceManifests_UndeclaredRust(t *testing.T) {
	tempDir := t.TempDir()

	cargoToml := `
[package]
name = "testapp"
version = "0.1.0"

[dependencies]
serde = "1.0"
`
	srcDir := filepath.Join(tempDir, "src")
	_ = os.MkdirAll(srcDir, 0755)
	_ = os.WriteFile(filepath.Join(tempDir, "Cargo.toml"), []byte(cargoToml), 0644)

	mainRs := `
use std::io;
use serde::Serialize;
use tokio::net::TcpListener;

fn main() {}
`
	_ = os.WriteFile(filepath.Join(srcDir, "main.rs"), []byte(mainRs), 0644)

	res, err := ValidateWorkspaceManifests(tempDir, []string{"src/main.rs"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Valid {
		t.Fatalf("expected validation to fail due to undeclared tokio crate")
	}
	if len(res.Violations) != 1 {
		t.Fatalf("expected 1 violation, got %d", len(res.Violations))
	}
	if res.Violations[0].Package != "tokio" {
		t.Errorf("expected violation on 'tokio', got %s", res.Violations[0].Package)
	}
}

func TestValidateManifestTool_Execute(t *testing.T) {
	tempDir := t.TempDir()
	cargoToml := `
[package]
name = "validapp"
version = "0.1.0"

[dependencies]
bytes = "1.0"
`
	_ = os.WriteFile(filepath.Join(tempDir, "Cargo.toml"), []byte(cargoToml), 0644)
	_ = os.WriteFile(filepath.Join(tempDir, "lib.rs"), []byte("use bytes::Bytes;\n"), 0644)

	tool := &ValidateManifestTool{}
	if tool.Name() != "validate_manifest" {
		t.Errorf("expected name 'validate_manifest', got %s", tool.Name())
	}

	state := &domain.State{ProjectPath: tempDir}
	out, err := tool.Execute(context.Background(), state, map[string]any{"target_files": []any{"lib.rs"}})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(out, "Status: VALID") {
		t.Errorf("expected Status: VALID, got: %s", out)
	}
}
