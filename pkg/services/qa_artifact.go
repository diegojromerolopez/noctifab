package services

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

// ArtifactManifestEntry identifies one executable by repository-relative path and content.
type ArtifactManifestEntry struct {
	Path   string `json:"path"`
	SHA256 string `json:"sha256"`
}

// QAArtifact is the immutable output consumed by the QA sandbox.
type QAArtifact struct {
	ID       string                  `json:"id"`
	Path     string                  `json:"path"`
	Manifest []ArtifactManifestEntry `json:"manifest"`
}

// ArtifactBuildRunner builds and copies only declared validation executables.
type ArtifactBuildRunner struct {
	sandbox QABuildSandbox
	fs      QAFileSystem
}

func NewArtifactBuildRunner(sandbox QABuildSandbox, fsys QAFileSystem) *ArtifactBuildRunner {
	return &ArtifactBuildRunner{sandbox: sandbox, fs: fsys}
}

func (r *ArtifactBuildRunner) Build(
	ctx context.Context,
	workspace ReviewWorkspace,
	sourceCommit string,
	buildCommand []string,
	validationExecutables []string,
	artifactPath string,
	outputLimit int,
) (QAArtifact, QACommandResult, error) {
	if r.sandbox == nil || r.fs == nil {
		return QAArtifact{}, QACommandResult{}, errors.New("artifact build: missing dependency")
	}
	if len(buildCommand) == 0 || strings.TrimSpace(sourceCommit) == "" || outputLimit <= 0 {
		return QAArtifact{}, QACommandResult{}, errors.New("artifact build: invalid request")
	}
	paths, err := normalizedExecutablePaths(validationExecutables)
	if err != nil {
		return QAArtifact{}, QACommandResult{}, err
	}
	workspacePath, err := r.fs.Abs(workspace.Path)
	if err != nil {
		return QAArtifact{}, QACommandResult{}, fmt.Errorf("artifact build: resolve workspace: %w", err)
	}
	artifactPath, err = r.fs.Abs(artifactPath)
	if err != nil || nestedPath(workspacePath, artifactPath) || nestedPath(artifactPath, workspacePath) || workspacePath == artifactPath {
		return QAArtifact{}, QACommandResult{}, errors.New("artifact build: workspace and output must be distinct and non-nested")
	}
	result, err := r.sandbox.Run(ctx, ReviewWorkspace{Path: workspacePath, Branch: workspace.Branch}, buildCommand, outputLimit)
	if err != nil {
		return QAArtifact{}, result, fmt.Errorf("artifact build: execute: %w", err)
	}
	if result.TimedOut || result.ExitCode != 0 || result.Truncated {
		return QAArtifact{}, result, errors.New("artifact build: validation surface unavailable")
	}
	if err := r.fs.RemoveAll(artifactPath); err != nil {
		return QAArtifact{}, result, fmt.Errorf("artifact build: reset output: %w", err)
	}
	if err := r.fs.MkdirAll(artifactPath, 0o700); err != nil {
		return QAArtifact{}, result, fmt.Errorf("artifact build: create output: %w", err)
	}
	manifest := make([]ArtifactManifestEntry, 0, len(paths))
	for _, path := range paths {
		source := filepath.Join(workspace.Path, filepath.FromSlash(path))
		info, statErr := r.fs.Lstat(source)
		if statErr != nil || info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() || info.Mode()&0o111 == 0 {
			return QAArtifact{}, result, fmt.Errorf("artifact build: executable %q unavailable", path)
		}
		destination := filepath.Join(artifactPath, filepath.FromSlash(path))
		if err := r.fs.MkdirAll(filepath.Dir(destination), 0o700); err != nil {
			return QAArtifact{}, result, fmt.Errorf("artifact build: create executable directory: %w", err)
		}
		hash, copyErr := r.copyAndHash(source, destination, info.Mode())
		if copyErr != nil {
			return QAArtifact{}, result, fmt.Errorf("artifact build: copy %q: %w", path, copyErr)
		}
		manifest = append(manifest, ArtifactManifestEntry{Path: path, SHA256: hash})
	}
	manifestHash := hashArtifactManifest(manifest)
	return QAArtifact{
		ID: sourceCommit + ":" + manifestHash, Path: artifactPath, Manifest: manifest,
	}, result, nil
}

// Verify recomputes the manifest and rejects any changed or missing artifact.
func (r *ArtifactBuildRunner) Verify(artifact QAArtifact) error {
	manifest := make([]ArtifactManifestEntry, 0, len(artifact.Manifest))
	for _, expected := range artifact.Manifest {
		data, err := r.fs.ReadFile(filepath.Join(artifact.Path, filepath.FromSlash(expected.Path)))
		if err != nil {
			return fmt.Errorf("artifact verify: %q: %w", expected.Path, err)
		}
		sum := sha256.Sum256(data)
		manifest = append(manifest, ArtifactManifestEntry{Path: expected.Path, SHA256: hex.EncodeToString(sum[:])})
	}
	parts := strings.SplitN(artifact.ID, ":", 2)
	if len(parts) != 2 || hashArtifactManifest(manifest) != parts[1] {
		return errors.New("artifact verify: artifact changed")
	}
	return nil
}

func (r *ArtifactBuildRunner) copyAndHash(source, destination string, mode os.FileMode) (string, error) {
	in, err := r.fs.Open(source)
	if err != nil {
		return "", err
	}
	defer func() { _ = in.Close() }()
	out, err := r.fs.OpenFile(destination, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, mode.Perm())
	if err != nil {
		return "", err
	}
	hash := sha256.New()
	_, copyErr := io.Copy(io.MultiWriter(out, hash), in)
	closeErr := out.Close()
	if copyErr != nil || closeErr != nil {
		return "", errors.Join(copyErr, closeErr)
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
}

func normalizedExecutablePaths(paths []string) ([]string, error) {
	if len(paths) == 0 {
		return nil, errors.New("artifact build: no validation executables")
	}
	normalized := make([]string, 0, len(paths))
	seen := make(map[string]struct{}, len(paths))
	for _, value := range paths {
		value = strings.TrimPrefix(filepath.ToSlash(filepath.Clean(value)), "./")
		if value == "" || value == "." || value == ".." || strings.HasPrefix(value, "../") || filepath.IsAbs(value) {
			return nil, fmt.Errorf("artifact build: invalid executable path %q", value)
		}
		if _, exists := seen[value]; exists {
			return nil, fmt.Errorf("artifact build: duplicate executable path %q", value)
		}
		seen[value] = struct{}{}
		normalized = append(normalized, value)
	}
	slices.Sort(normalized)
	return normalized, nil
}

func hashArtifactManifest(manifest []ArtifactManifestEntry) string {
	hash := sha256.New()
	for _, entry := range manifest {
		_, _ = io.WriteString(hash, entry.Path+"\x00"+entry.SHA256+"\n")
	}
	return hex.EncodeToString(hash.Sum(nil))
}

func commandResult(result QAProcessResult) QACommandResult {
	return QACommandResult(result)
}
