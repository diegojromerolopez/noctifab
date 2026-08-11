package reportfs

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"path/filepath"
)

type AtomicWriter struct {
	fsSys FileSystem
}

func NewAtomicWriter(fsSys FileSystem) *AtomicWriter {
	if fsSys == nil {
		fsSys = OSFileSystem{}
	}
	return &AtomicWriter{fsSys: fsSys}
}

func (w *AtomicWriter) WriteAtomic(ctx context.Context, path string, content []byte) error {
	if ctx.Err() != nil {
		return ctx.Err()
	}
	if path == "" {
		return errors.New("cannot write atomic to empty path")
	}

	cleanPath := filepath.Clean(path)
	info, err := w.fsSys.Lstat(cleanPath)
	if err == nil {
		if info.IsDir() {
			return errors.New("cannot overwrite directory with report file")
		}
		if info.Mode()&fs.ModeSymlink != 0 {
			return errors.New("cannot overwrite symbolic link with report file")
		}
	}

	dir := filepath.Dir(cleanPath)
	tempFile, err := w.fsSys.CreateTemp(dir, ".report_tmp", 0600)
	if err != nil {
		return fmt.Errorf("failed to create temporary report file: %w", err)
	}
	tempName := tempFile.Name()

	cleanup := true
	defer func() {
		if cleanup {
			_ = w.fsSys.Remove(tempName)
		}
	}()

	if _, err := tempFile.Write(content); err != nil {
		_ = tempFile.Close()
		return fmt.Errorf("failed to write content to temporary report file: %w", err)
	}

	if err := tempFile.Sync(); err != nil {
		_ = tempFile.Close()
		return fmt.Errorf("failed to sync temporary report file: %w", err)
	}

	if err := tempFile.Chmod(0600); err != nil {
		_ = tempFile.Close()
		return fmt.Errorf("failed to set 0600 permissions on temporary report file: %w", err)
	}

	if err := tempFile.Close(); err != nil {
		return fmt.Errorf("failed to close temporary report file: %w", err)
	}

	if err := w.fsSys.Rename(tempName, cleanPath); err != nil {
		return fmt.Errorf("failed to rename temporary report file to destination: %w", err)
	}
	cleanup = false

	if parentDir, err := w.fsSys.Open(dir); err == nil {
		_ = parentDir.Sync()
		_ = parentDir.Close()
	}

	return nil
}
