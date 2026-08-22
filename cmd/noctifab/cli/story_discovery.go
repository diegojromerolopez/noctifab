package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

func discoverStoryFiles(targetDir string) []string {
	storiesDir := filepath.Join(targetDir, "roadmap", "user-stories")

	var storyFiles []string
	if matches, err := filepath.Glob(filepath.Join(storiesDir, "*.md")); err == nil {
		storyFiles = append(storyFiles, matches...)
	}
	sort.Strings(storyFiles)

	roadmapDir := filepath.Join(targetDir, "roadmap")
	if matches, err := filepath.Glob(filepath.Join(roadmapDir, "US-*.md")); err == nil && len(matches) > 0 {
		fmt.Printf("Warning: Found %d user story file(s) directly in 'roadmap/'; user stories must be located in 'roadmap/user-stories/'. Please move them to %s.\n", len(matches), storiesDir)
	}

	return storyFiles
}

func extractStoryTitle(filePath string) string {
	content, err := os.ReadFile(filePath)
	if err != nil {
		return ""
	}
	lines := strings.Split(string(content), "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "# ") {
			title := strings.TrimPrefix(line, "# ")
			return strings.TrimSpace(title)
		}
	}
	return ""
}
