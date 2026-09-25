package services

import (
	"os"
)

// fileRollbackSnapshot preserves the pre-mutation state of a file on disk
// so that if a post-write hook (e.g. syntax checker) fails, the file can be
// atomically restored to its exact previous state (or deleted if newly created).
type fileRollbackSnapshot struct {
	path    string
	existed bool
	content []byte
	perm    os.FileMode
}

// snapshotFile captures the existence, content, and file permissions of path.
func snapshotFile(fullPath string) fileRollbackSnapshot {
	info, err := os.Stat(fullPath)
	if err != nil || info.IsDir() {
		return fileRollbackSnapshot{path: fullPath, existed: false}
	}
	content, err := os.ReadFile(fullPath)
	if err != nil {
		return fileRollbackSnapshot{path: fullPath, existed: false}
	}
	return fileRollbackSnapshot{
		path:    fullPath,
		existed: true,
		content: content,
		perm:    info.Mode().Perm(),
	}
}

// rollback restores the file to its snapshot state.
func (s fileRollbackSnapshot) rollback() {
	if s.existed {
		_ = os.WriteFile(s.path, s.content, s.perm)
	} else {
		_ = os.Remove(s.path)
	}
}

// rollbackAll restores all snapshots in reverse order.
func rollbackAll(snapshots []fileRollbackSnapshot) {
	for i := len(snapshots) - 1; i >= 0; i-- {
		snapshots[i].rollback()
	}
}
