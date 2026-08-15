package reportfs

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

type SyncFile interface {
	io.Writer
	Name() string
	Sync() error
	Chmod(mode fs.FileMode) error
	Close() error
}

type FileSystem interface {
	Lstat(path string) (fs.FileInfo, error)
	EvalSymlinks(path string) (string, error)
	Mkdir(path string, mode fs.FileMode) error
	CreateTemp(dir, pattern string, mode fs.FileMode) (SyncFile, error)
	Open(path string) (SyncFile, error)
	Rename(oldPath, newPath string) error
	Remove(path string) error
}

type OSFileSystem struct{}

func (o OSFileSystem) Lstat(path string) (fs.FileInfo, error) {
	return os.Lstat(path)
}

func (o OSFileSystem) EvalSymlinks(path string) (string, error) {
	return filepath.EvalSymlinks(path)
}

func (o OSFileSystem) Mkdir(path string, mode fs.FileMode) error {
	return os.Mkdir(path, mode)
}

func (o OSFileSystem) CreateTemp(dir, pattern string, mode fs.FileMode) (SyncFile, error) {
	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil {
		return nil, fmt.Errorf("failed to generate random suffix: %w", err)
	}
	name := filepath.Join(dir, pattern+"_"+hex.EncodeToString(b)+".tmp")
	f, err := os.OpenFile(name, os.O_RDWR|os.O_CREATE|os.O_EXCL, mode)
	if err != nil {
		return nil, err
	}
	return f, nil
}

func (o OSFileSystem) Open(path string) (SyncFile, error) {
	return os.Open(path)
}

func (o OSFileSystem) Rename(oldPath, newPath string) error {
	return os.Rename(oldPath, newPath)
}

func (o OSFileSystem) Remove(path string) error {
	return os.Remove(path)
}

type ReportDestinationPolicy struct {
	ProjectPath  string
	ConfigPath   string
	DatabasePath string
	PIDPath      string
}

func PrepareReportDestination(
	ctx context.Context,
	path string,
	policy ReportDestinationPolicy,
	fsSys FileSystem,
) error {
	if ctx.Err() != nil {
		return ctx.Err()
	}
	if path == "" {
		return errors.New("report destination path is empty")
	}

	cleanPath := filepath.Clean(path)
	dir := filepath.Dir(cleanPath)

	// Protected paths check
	protected := []string{
		policy.ConfigPath,
		policy.DatabasePath,
		policy.PIDPath,
	}
	for _, p := range protected {
		if p != "" && filepath.Clean(p) == cleanPath {
			return fmt.Errorf("report path matches protected location (%s)", p)
		}
	}

	pathParts := strings.Split(filepath.ToSlash(cleanPath), "/")
	for _, part := range pathParts {
		if part == ".git" || part == "secrets.yaml" {
			return fmt.Errorf("report path targets forbidden system path component (%s)", part)
		}
	}

	// Check existing target path
	info, err := fsSys.Lstat(cleanPath)
	if err == nil {
		if info.IsDir() {
			return errors.New("target report path is a directory")
		}
		if info.Mode()&fs.ModeSymlink != 0 {
			return errors.New("target report path is a symbolic link")
		}
	}

	// Track directories created so we can roll back on failure
	var createdDirs []string
	defer func() {
		// Cleanup if err returned
	}()

	// Find ancestor directories to create
	var toCreate []string
	curr := dir
	for {
		_, lstatErr := fsSys.Lstat(curr)
		if lstatErr == nil {
			break
		}
		toCreate = append([]string{curr}, toCreate...)
		parent := filepath.Dir(curr)
		if parent == curr {
			break
		}
		curr = parent
	}

	for _, d := range toCreate {
		if mkErr := fsSys.Mkdir(d, 0700); mkErr != nil && !os.IsExist(mkErr) {
			// Rollback created dirs
			for i := len(createdDirs) - 1; i >= 0; i-- {
				_ = fsSys.Remove(createdDirs[i])
			}
			return fmt.Errorf("failed to create report directory %s: %w", d, mkErr)
		}
		createdDirs = append(createdDirs, d)
	}

	// Canonical symlink check on ancestor
	if policy.ProjectPath != "" {
		canonicalProject, evalErr := fsSys.EvalSymlinks(policy.ProjectPath)
		if evalErr == nil && canonicalProject != "" {
			canonicalDir, evalDirErr := fsSys.EvalSymlinks(dir)
			if evalDirErr == nil {
				inWorkspace := cleanPath == policy.ProjectPath || strings.HasPrefix(cleanPath, policy.ProjectPath+string(filepath.Separator))
				if inWorkspace {
					reportsDir := filepath.Join(canonicalProject, ".noctifab", "reports")
					inCanonicalReports := canonicalDir == reportsDir || strings.HasPrefix(canonicalDir, reportsDir+string(filepath.Separator))
					if !inCanonicalReports {
						// Rollback created dirs
						for i := len(createdDirs) - 1; i >= 0; i-- {
							_ = fsSys.Remove(createdDirs[i])
						}
						return fmt.Errorf("canonical destination dir %s is outside workspace reports dir %s", canonicalDir, reportsDir)
					}
				}
			}
		}
	}

	// Final re-check of target before probe
	info, err = fsSys.Lstat(cleanPath)
	if err == nil {
		if info.IsDir() || info.Mode()&fs.ModeSymlink != 0 {
			for i := len(createdDirs) - 1; i >= 0; i-- {
				_ = fsSys.Remove(createdDirs[i])
			}
			return errors.New("target report path is directory or symlink")
		}
	}

	// Exclusive probe creation in target directory
	probeFile, probeErr := fsSys.CreateTemp(dir, ".report_probe", 0600)
	if probeErr != nil {
		for i := len(createdDirs) - 1; i >= 0; i-- {
			_ = fsSys.Remove(createdDirs[i])
		}
		return fmt.Errorf("writable destination probe failed: %w", probeErr)
	}
	probeName := probeFile.Name()
	_ = probeFile.Close()
	_ = fsSys.Remove(probeName)

	return nil
}
