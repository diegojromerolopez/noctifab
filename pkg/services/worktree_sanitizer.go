package services

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// MaxContextFileSize is the maximum file size (512 KB) allowed into LLM context packing.
const MaxContextFileSize = 512 * 1024

var (
	// Ephemeral cache directories that should be purged from disk
	ephemeralCacheDirs = map[string]bool{
		"__pycache__":   true,
		".pytest_cache": true,
		".ruff_cache":   true,
		".mypy_cache":   true,
		".tox":          true,
		".nox":          true,
		".parcel-cache": true,
		".next":         true,
		".nuxt":         true,
		".turbo":        true,
		"coverage":      true,
		"htmlcov":       true,
		".coverage":     true,
	}

	// Ephemeral cache or temporary files that should be deleted
	ephemeralCacheExts = map[string]bool{
		".pyc":         true,
		".pyo":         true,
		".pyd":         true,
		".tmp":         true,
		".swp":         true,
		".swo":         true,
		".bak":         true,
		".pid":         true,
		".prof":        true,
		".tsbuildinfo": true,
	}

	ephemeralCacheFiles = map[string]bool{
		".ds_store": true,
		"thumbs.db": true,
	}

	// Non-text binary asset extensions to ignore in context
	nonContextAssetExts = map[string]bool{
		".png":   true,
		".jpg":   true,
		".jpeg":  true,
		".gif":   true,
		".ico":   true,
		".svg":   true,
		".webp":  true,
		".pdf":   true,
		".zip":   true,
		".tar":   true,
		".gz":    true,
		".bz2":   true,
		".xz":    true,
		".7z":    true,
		".exe":   true,
		".dll":   true,
		".so":    true,
		".dylib": true,
		".class": true,
		".o":     true,
		".a":     true,
		".woff":  true,
		".woff2": true,
		".ttf":   true,
		".eot":   true,
		".mp3":   true,
		".mp4":   true,
		".mov":   true,
		".avi":   true,
	}
)

// SanitizeWorkspace walks projectPath and purges ephemeral cache directories and files.
// It skips .git, .noctifab, and any specified custom skip folders so they are never disturbed.
func SanitizeWorkspace(projectPath string, customSkipFolders ...string) error {
	if projectPath == "" {
		return nil
	}

	skipMap := make(map[string]bool)
	for _, folder := range customSkipFolders {
		clean := strings.Trim(strings.ToLower(folder), "/\\")
		if clean != "" {
			skipMap[clean] = true
		}
	}

	var purgeErrors []string

	err := filepath.WalkDir(projectPath, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil || d == nil {
			return nil
		}

		name := d.Name()
		nameLower := strings.ToLower(name)

		if d.IsDir() {
			if name == ".git" || name == ".noctifab" || skipMap[nameLower] {
				return filepath.SkipDir
			}
			if ephemeralCacheDirs[nameLower] {
				if rErr := os.RemoveAll(path); rErr != nil {
					purgeErrors = append(purgeErrors, fmt.Sprintf("dir %s: %v", path, rErr))
				}
				return filepath.SkipDir
			}
			return nil
		}

		// Check file extensions and names
		if ephemeralCacheFiles[nameLower] {
			if rErr := os.Remove(path); rErr != nil {
				purgeErrors = append(purgeErrors, fmt.Sprintf("file %s: %v", path, rErr))
			}
			return nil
		}

		ext := strings.ToLower(filepath.Ext(name))
		if ephemeralCacheExts[ext] {
			if rErr := os.Remove(path); rErr != nil {
				purgeErrors = append(purgeErrors, fmt.Sprintf("file %s: %v", path, rErr))
			}
		}

		return nil
	})

	if err != nil {
		return fmt.Errorf("sanitize workspace walk %s: %w", projectPath, err)
	}
	if len(purgeErrors) > 0 {
		return fmt.Errorf("sanitize workspace encountered %d deletion errors: %s", len(purgeErrors), strings.Join(purgeErrors, "; "))
	}
	return nil
}

// IsContextRelevantFile returns true if relPath is suitable for LLM context inclusion based on path patterns.
func IsContextRelevantFile(relPath string, excludePaths []string) bool {
	if IsPathExcluded(relPath, excludePaths) {
		return false
	}
	ext := strings.ToLower(filepath.Ext(relPath))
	if nonContextAssetExts[ext] {
		return false
	}
	base := strings.ToLower(filepath.Base(relPath))
	return !ephemeralCacheFiles[base]
}

// FilterRelevantFiles filters a slice of relative paths, keeping only existing, non-excluded,
// non-binary, appropriately-sized text files.
func FilterRelevantFiles(projectPath string, files []string, excludePaths []string) []string {
	var relevant []string

	for _, f := range files {
		clean := strings.TrimSpace(f)
		if clean == "" {
			continue
		}

		if !IsContextRelevantFile(clean, excludePaths) {
			continue
		}

		fullPath := clean
		if !filepath.IsAbs(clean) && projectPath != "" {
			fullPath = filepath.Join(projectPath, clean)
		}

		info, err := os.Stat(fullPath)
		if err != nil || info.IsDir() {
			continue
		}

		// Reject oversized files
		if info.Size() > MaxContextFileSize || info.Size() == 0 {
			continue
		}

		// Reject binary files
		if !IsTextFile(fullPath) {
			continue
		}

		relevant = append(relevant, clean)
	}

	return relevant
}
