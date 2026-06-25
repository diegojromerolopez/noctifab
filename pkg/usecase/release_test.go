package usecase

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/diegojromerolopez/noctifab/pkg/domain"
)

func TestBumpVersion(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "noctifab-version-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer func() { _ = os.RemoveAll(tmpDir) }()

	// Case 1: VERSION does not exist, defaults to 0.0.1 and bumps to 0.0.2
	tasks := []domain.Task{
		{Status: domain.TaskSuccess, ChangeType: domain.ChangeTypeFix},
	}
	v, err := BumpVersion(tmpDir, tasks)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if v != "0.0.2" {
		t.Errorf("expected 0.0.2, got %s", v)
	}

	// Case 2: VERSION exists, features completed -> minor bump
	tasks = []domain.Task{
		{Status: domain.TaskSuccess, ChangeType: domain.ChangeTypeFeature},
	}
	v, err = BumpVersion(tmpDir, tasks)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if v != "0.1.0" {
		t.Errorf("expected 0.1.0, got %s", v)
	}

	// Case 3: Breaking change completed -> major bump
	tasks = []domain.Task{
		{Status: domain.TaskSuccess, ChangeType: domain.ChangeTypeBreaking},
	}
	v, err = BumpVersion(tmpDir, tasks)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if v != "1.0.0" {
		t.Errorf("expected 1.0.0, got %s", v)
	}

	// Case 4: Patch change -> patch bump
	tasks = []domain.Task{
		{Status: domain.TaskSuccess, ChangeType: domain.ChangeTypeFix},
	}
	v, err = BumpVersion(tmpDir, tasks)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if v != "1.0.1" {
		t.Errorf("expected 1.0.1, got %s", v)
	}
}

func TestUpdateChangelog(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "noctifab-changelog-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer func() { _ = os.RemoveAll(tmpDir) }()

	tasks := []domain.Task{
		{
			Status:           domain.TaskSuccess,
			PartialChangelog: []string{"add new endpoints for worker", "fix panic on initialization", "modified client constructor"},
		},
	}

	err = UpdateChangelog(tmpDir, "1.2.3", tasks)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	cPath := filepath.Join(tmpDir, "CHANGELOG.md")
	contentBytes, err := os.ReadFile(cPath)
	if err != nil {
		t.Fatalf("failed to read changelog: %v", err)
	}
	content := string(contentBytes)

	if !strings.Contains(content, "# Changelog") {
		t.Errorf("expected header '# Changelog' in changelog, got: %s", content)
	}
	if !strings.Contains(content, "## [1.2.3]") {
		t.Errorf("expected section '## [1.2.3]' in changelog, got: %s", content)
	}
	if !strings.Contains(content, "- add new endpoints for worker") {
		t.Errorf("expected added list item, got: %s", content)
	}
	if !strings.Contains(content, "- fix panic on initialization") {
		t.Errorf("expected fixed list item, got: %s", content)
	}
}
