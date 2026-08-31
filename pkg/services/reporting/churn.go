package reporting

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/diegojromerolopez/noctifab/pkg/domain"
)

var storyContractBlockRE = regexp.MustCompile("(?s)```noctifab-contract[ \\t]*\\r?\\n(.*?)\\r?\\n```")

func collectStoryContracts(projectPath string) []domain.PublicContract {
	storiesDir := filepath.Join(projectPath, "roadmap", "user-stories")

	var storyFiles []string
	if matches, err := filepath.Glob(filepath.Join(storiesDir, "*.md")); err == nil {
		storyFiles = append(storyFiles, matches...)
	}

	var contracts []domain.PublicContract
	seen := make(map[string]bool)

	for _, fullPath := range storyFiles {
		content, rErr := os.ReadFile(fullPath)
		if rErr != nil {
			continue
		}
		matches := storyContractBlockRE.FindAllStringSubmatch(string(content), -1)
		for _, match := range matches {
			var payload struct {
				PublicContracts []domain.PublicContract `json:"public_contracts"`
			}
			if jsonErr := json.Unmarshal([]byte(match[1]), &payload); jsonErr == nil {
				for _, pc := range payload.PublicContracts {
					if pc.ID != "" && !seen[pc.ID] {
						seen[pc.ID] = true
						contracts = append(contracts, pc)
					}
				}
			}
		}
	}
	return contracts
}

func isVendorOrArtifactPath(projectPath, filePath string) bool {
	filePath = filepath.ToSlash(strings.TrimSpace(filePath))
	parts := strings.Split(filePath, "/")
	for _, part := range parts {
		if part == ".git" || part == ".noctifab" {
			return true
		}
	}
	if projectPath != "" {
		cmd := exec.Command("git", "check-ignore", "-q", filePath)
		cmd.Dir = projectPath
		if cmd.Run() == nil {
			return true
		}
	}
	return false
}

func computeWorkspaceChurn(projectPath string) CodeChurnSummary {
	var churn CodeChurnSummary
	if projectPath == "" {
		return churn
	}

	changedMap := make(map[string]bool)

	// Helper to parse git diff --numstat lines filtered by source code boundaries
	parseNumstat := func(output string) {
		lines := strings.Split(strings.TrimSpace(output), "\n")
		for _, line := range lines {
			line = strings.TrimSpace(line)
			if line == "" {
				continue
			}
			parts := strings.Fields(line)
			if len(parts) < 3 {
				continue
			}
			filePath := parts[2]
			if isVendorOrArtifactPath(projectPath, filePath) {
				continue
			}
			var added, deleted int64
			_, _ = fmt.Sscanf(parts[0], "%d", &added)
			_, _ = fmt.Sscanf(parts[1], "%d", &deleted)

			churn.LinesAdded += added
			churn.LinesDeleted += deleted
			changedMap[filePath] = true
		}
	}

	// Find root commit of repository
	rootCmd := exec.Command("git", "rev-list", "--max-parents=0", "HEAD")
	rootCmd.Dir = projectPath
	if rootOut, rErr := rootCmd.Output(); rErr == nil && len(bytes.TrimSpace(rootOut)) > 0 {
		rootHash := strings.TrimSpace(string(rootOut))
		diffCmd := exec.Command("git", "diff", "--numstat", rootHash)
		diffCmd.Dir = projectPath
		if out, err := diffCmd.Output(); err == nil && len(bytes.TrimSpace(out)) > 0 {
			parseNumstat(string(out))
		}
	}

	// Fallback to git diff --numstat HEAD if root commit diff yielded nothing
	if len(changedMap) == 0 {
		cmd := exec.Command("git", "diff", "--numstat", "HEAD")
		cmd.Dir = projectPath
		if out, err := cmd.Output(); err == nil && len(bytes.TrimSpace(out)) > 0 {
			parseNumstat(string(out))
		}
	}

	cmdStatus := exec.Command("git", "status", "--porcelain")
	cmdStatus.Dir = projectPath
	if outStatus, errStatus := cmdStatus.Output(); errStatus == nil {
		lines := strings.Split(strings.TrimSpace(string(outStatus)), "\n")
		for _, line := range lines {
			if len(line) < 3 {
				continue
			}
			filePath := strings.TrimSpace(line[2:])
			if filePath == "" || isVendorOrArtifactPath(projectPath, filePath) {
				continue
			}
			changedMap[filePath] = true
			if strings.HasPrefix(line, "??") || strings.HasPrefix(line, "A ") {
				fullPath := filepath.Join(projectPath, filePath)
				if content, rErr := os.ReadFile(fullPath); rErr == nil {
					lineCount := int64(bytes.Count(content, []byte("\n")))
					if len(content) > 0 && !bytes.HasSuffix(content, []byte("\n")) {
						lineCount++
					}
					churn.LinesAdded += lineCount
				}
			}
		}
	}

	// Walk workspace directory to ensure all created artifacts (files) are included
	_ = filepath.WalkDir(projectPath, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		rel, rErr := filepath.Rel(projectPath, path)
		if rErr != nil || rel == "." {
			return nil
		}
		relSlash := filepath.ToSlash(rel)
		if isVendorOrArtifactPath(projectPath, relSlash) {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if !d.IsDir() {
			changedMap[relSlash] = true
		}
		return nil
	})

	var fileList []string
	for f := range changedMap {
		fileList = append(fileList, f)
	}
	sort.Strings(fileList)

	churn.ChangedFiles = fileList
	churn.FilesChanged = int64(len(fileList))

	return churn
}
