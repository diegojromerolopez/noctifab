package usecase

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

type SelfUpdateManager struct {
	RepoPath   string
	BinaryPath string
	GoCmd      string
}

const selfUpdateTempDir = "/tmp/noctifab-self-update"

var allowedSelfPatchPrefixes = []string{"cmd/noctifab/", "pkg/"}

type Patch struct {
	Path    string
	Content string
}

func (sum *SelfUpdateManager) BuildAndTest(ctx context.Context, patches []Patch) error {
	tmpDir := filepath.Join(selfUpdateTempDir, "src")

	os.RemoveAll(selfUpdateTempDir)
	defer os.RemoveAll(selfUpdateTempDir)

	if err := os.MkdirAll(selfUpdateTempDir, 0755); err != nil {
		return fmt.Errorf("self-update: failed to create temp dir: %w", err)
	}

	if err := sum.copyRepo(tmpDir); err != nil {
		return fmt.Errorf("self-update: failed to copy repo: %w", err)
	}

	for _, p := range patches {
		if err := sum.validatePatch(p); err != nil {
			return fmt.Errorf("self-update: patch validation failed: %w", err)
		}
		fullPath := filepath.Join(tmpDir, p.Path)
		if err := os.MkdirAll(filepath.Dir(fullPath), 0755); err != nil {
			return fmt.Errorf("self-update: failed to create dir for %s: %w", p.Path, err)
		}
		if err := os.WriteFile(fullPath, []byte(p.Content), 0644); err != nil {
			return fmt.Errorf("self-update: failed to write %s: %w", p.Path, err)
		}
	}

	buildCmd := exec.CommandContext(ctx, sum.GoCmd, "build", "-o", "/tmp/noctifab-new", "./cmd/noctifab")
	buildCmd.Dir = tmpDir
	if output, err := buildCmd.CombinedOutput(); err != nil {
		return fmt.Errorf("self-update: build failed: %w\nOutput: %s", err, string(output))
	}

	testCmd := exec.CommandContext(ctx, sum.GoCmd, "test", "./pkg/...")
	testCmd.Dir = tmpDir
	if output, err := testCmd.CombinedOutput(); err != nil {
		return fmt.Errorf("self-update: tests failed: %w\nOutput: %s", err, string(output))
	}

	return nil
}

func (sum *SelfUpdateManager) validatePatch(p Patch) error {
	if p.Path == "go.mod" || p.Path == "go.sum" {
		return fmt.Errorf("rejected: changes to %s require human review", p.Path)
	}
	allowed := false
	for _, prefix := range allowedSelfPatchPrefixes {
		if strings.HasPrefix(p.Path, prefix) {
			allowed = true
			break
		}
	}
	if !allowed {
		return fmt.Errorf("rejected: path %s is outside allowed patch directories", p.Path)
	}
	return nil
}

func (sum *SelfUpdateManager) copyRepo(dst string) error {
	cmd := exec.Command("cp", "-R", sum.RepoPath, dst)
	return cmd.Run()
}
