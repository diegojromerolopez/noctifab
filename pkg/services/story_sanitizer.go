package services

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/diegojromerolopez/noctifab/pkg/domain"
)

// RawStoryItem represents an unpersisted user story produced by the Product Manager agent.
type RawStoryItem struct {
	Filename string
	Content  string
}

// IsSmallCLIProject evaluates whether a project specification describes a small,
// single-binary CLI tool or utility (CU < 35) that requires strict story and task caps.
func IsSmallCLIProject(specContent string, maxUserStories int) bool {
	if maxUserStories > 0 && maxUserStories <= 2 {
		return true
	}
	lower := strings.ToLower(specContent)
	if strings.Contains(lower, "cu < 35") || strings.Contains(lower, "cu: <35") || strings.Contains(lower, "complexity: tier 0") || strings.Contains(lower, "tier 0") {
		return true
	}

	// Exclude complex distributed, multi-tier, or frontend systems
	if strings.Contains(lower, "react") || strings.Contains(lower, "vue") ||
		strings.Contains(lower, "microservice") || strings.Contains(lower, "kubernetes") ||
		strings.Contains(lower, "spring boot") || strings.Contains(lower, "django") ||
		strings.Contains(lower, "fastapi + redis") || strings.Contains(lower, "oauth2") ||
		strings.Contains(lower, "protobuf-native") || strings.Contains(lower, "vector search") {
		return false
	}

	// Detect CLI indicators
	cliScore := 0
	for _, term := range []string{"cli", "command line", "command-line", "stdin", "stdout", "stderr", "exit code", "flags", "subcommand", "terminal"} {
		if strings.Contains(lower, term) {
			cliScore++
		}
	}

	lines := strings.Split(specContent, "\n")
	return cliScore >= 2 && len(lines) < 600
}

// IsHardeningStory evaluates whether a story represents project hardening,
// packaging, static analysis, or maintenance readiness across any tier.
func IsHardeningStory(identifier, title, desc string) bool {
	combined := strings.ToLower(identifier + " " + title + " " + desc)
	return strings.Contains(combined, "hardening") ||
		strings.Contains(combined, "maintenance readiness") ||
		strings.Contains(combined, "us-final") ||
		strings.Contains(combined, "project hardening")
}

// SanitizeAndCapStories deduplicates story IDs, enforces strictly monotonic unique numbering (US-001, US-002, ...),
// caps excessive stories for small CLI utilities or configured limits, and updates cross-story references.
func SanitizeAndCapStories(projectPath string, rawStories []RawStoryItem, specContent string, configuredMax int) []RawStoryItem {
	if len(rawStories) == 0 {
		return nil
	}

	isSmallCLI := IsSmallCLIProject(specContent, configuredMax)

	var legacyStories []RawStoryItem
	var featureStories []RawStoryItem
	var hardeningStories []RawStoryItem

	for _, story := range rawStories {
		lowerFile := strings.ToLower(story.Filename)
		lowerContent := strings.ToLower(story.Content)

		if IsHardeningStory(lowerFile, "", lowerContent) {
			hardeningStories = append(hardeningStories, story)
		} else if strings.Contains(lowerFile, "legacy") || strings.Contains(lowerContent, "characterization") || strings.Contains(lowerFile, "stabilization") {
			legacyStories = append(legacyStories, story)
		} else {
			featureStories = append(featureStories, story)
		}
	}

	// Capping logic: for small CLI utilities, enforce at most 1 feature story (+ optional legacy, + optional hardening)
	if isSmallCLI {
		if len(featureStories) > 1 {
			featureStories = featureStories[:1]
		}
	} else if configuredMax > 0 && len(featureStories) > configuredMax {
		featureStories = featureStories[:configuredMax]
	}

	var ordered []RawStoryItem
	ordered = append(ordered, legacyStories...)
	ordered = append(ordered, featureStories...)
	if len(hardeningStories) > 1 {
		hardeningStories = hardeningStories[:1]
	}
	ordered = append(ordered, hardeningStories...)

	idMap := make(map[string]string)
	type plannedStory struct {
		item    RawStoryItem
		oldID   string
		newID   string
		newFile string
	}
	planned := make([]plannedStory, len(ordered))

	for i, st := range ordered {
		newID := fmt.Sprintf("US-%03d", i+1)
		oldID := ExtractStoryID(st.Filename)
		if oldID == "" {
			oldID = ExtractStoryID(st.Content)
		}
		if oldID != "" && oldID != newID {
			idMap[oldID] = newID
		}

		existingPath := st.Filename
		if !filepath.IsAbs(existingPath) {
			existingPath = filepath.Join(projectPath, existingPath)
		}
		var newFilename string
		if info, err := os.Stat(existingPath); err == nil && !info.IsDir() {
			newFilename = st.Filename
		} else {
			baseName := filepath.Base(st.Filename)
			ext := filepath.Ext(baseName)
			nameNoExt := strings.TrimSuffix(baseName, ext)

			slug := cleanSlugID(nameNoExt)
			if slug == "" || slug == "story" {
				slug = ExtractTitleSlug(st.Content)
				slug = cleanSlugID(slug)
			}
			if slug == "" {
				slug = "story"
			}
			newFilename = filepath.Join("roadmap", "user-stories", fmt.Sprintf("%s-%s.md", newID, slug))
		}

		planned[i] = plannedStory{
			item:    st,
			oldID:   oldID,
			newID:   newID,
			newFile: newFilename,
		}
	}

	var sanitized []RawStoryItem
	contractRE := regexp.MustCompile(`"story_id":\s*"US-\d+"`)
	depLineRE := regexp.MustCompile(`(?m)^(\s*-\s*\*\*Depends On\*\*:\s*|depends_on:\s*)(.*)$`)

	for _, p := range planned {
		content := p.item.Content

		// 1. Rewrite heading with new ID only if ID changed
		if p.oldID != "" && p.oldID != p.newID {
			oldNum := strings.TrimPrefix(p.oldID, "US-")
			newNum := strings.TrimPrefix(p.newID, "US-")
			content = strings.Replace(content, fmt.Sprintf("# User Story %s:", oldNum), fmt.Sprintf("# User Story %s:", newNum), 1)
			content = strings.Replace(content, fmt.Sprintf("# US-%s:", oldNum), fmt.Sprintf("# US-%s:", newNum), 1)
			content = strings.Replace(content, fmt.Sprintf("# US-%s ", oldNum), fmt.Sprintf("# US-%s ", newNum), 1)
		}

		// 2. Rewrite contract block story_id to target newID
		content = contractRE.ReplaceAllLiteralString(content, fmt.Sprintf(`"story_id": "%s"`, p.newID))

		// 3. Rewrite dependencies strictly within dependency lines
		content = depLineRE.ReplaceAllStringFunc(content, func(line string) string {
			for oldID, newID := range idMap {
				if oldID != newID {
					line = strings.ReplaceAll(line, fmt.Sprintf(`"%s"`, oldID), fmt.Sprintf(`"%s"`, newID))
					line = strings.ReplaceAll(line, fmt.Sprintf(`'%s'`, oldID), fmt.Sprintf(`'%s'`, newID))
				}
			}
			return line
		})

		// Ensure first story has empty dependencies if it's US-001
		if p.newID == "US-001" {
			depRE := regexp.MustCompile(`(?m)^(\s*-\s*\*\*Depends On\*\*:\s*).*$`)
			if depRE.MatchString(content) {
				content = depRE.ReplaceAllString(content, `${1}[]`)
			}
			yamlDepRE := regexp.MustCompile(`(?m)^(depends_on:\s*).*$`)
			if yamlDepRE.MatchString(content) {
				content = yamlDepRE.ReplaceAllString(content, `${1}[]`)
			}
		}

		sanitized = append(sanitized, RawStoryItem{
			Filename: p.newFile,
			Content:  content,
		})
	}

	return sanitized
}

func cleanSlugID(slug string) string {
	lower := strings.ToLower(slug)
	re := regexp.MustCompile(`^us-\d+-?`)
	cleaned := re.ReplaceAllString(lower, "")
	return strings.Trim(cleaned, "-")
}

// CapPlannedTasks bounds the number of tasks planned for a story to maxTasks,
// consolidating intermediate micro-tasks to prevent excessive worktree merge latency.
func CapPlannedTasks(tasks []domain.Task, maxTasks int) []domain.Task {
	if len(tasks) <= maxTasks || maxTasks <= 0 {
		return tasks
	}

	if maxTasks == 1 {
		first := tasks[0]
		var allFiles []string
		fileSet := make(map[string]bool)
		var descBuilder strings.Builder
		descBuilder.WriteString(first.Description)
		for _, f := range first.TargetFiles {
			if !fileSet[f] {
				fileSet[f] = true
				allFiles = append(allFiles, f)
			}
		}
		for i := 1; i < len(tasks); i++ {
			descBuilder.WriteString("\n\n---\n\n")
			descBuilder.WriteString(tasks[i].Description)
			for _, f := range tasks[i].TargetFiles {
				if !fileSet[f] {
					fileSet[f] = true
					allFiles = append(allFiles, f)
				}
			}
		}
		first.TargetFiles = allFiles
		first.Description = descBuilder.String()
		first.DependsOn = nil
		return []domain.Task{first}
	}

	first := tasks[0]
	last := tasks[len(tasks)-1]

	middleTasks := tasks[1 : len(tasks)-1]
	mid := middleTasks[0]
	var midFiles []string
	midFileSet := make(map[string]bool)
	var midDesc strings.Builder
	midDesc.WriteString(mid.Description)
	for _, f := range mid.TargetFiles {
		if !midFileSet[f] {
			midFileSet[f] = true
			midFiles = append(midFiles, f)
		}
	}
	for i := 1; i < len(middleTasks); i++ {
		midDesc.WriteString("\n\n---\n\n")
		midDesc.WriteString(middleTasks[i].Description)
		for _, f := range middleTasks[i].TargetFiles {
			if !midFileSet[f] {
				midFileSet[f] = true
				midFiles = append(midFiles, f)
			}
		}
	}
	mid.TargetFiles = midFiles
	mid.Description = midDesc.String()
	mid.DependsOn = []string{first.ID}

	last.DependsOn = []string{mid.ID}

	if maxTasks == 2 {
		var finalFiles []string
		finalFileSet := make(map[string]bool)
		for _, f := range append(mid.TargetFiles, last.TargetFiles...) {
			if !finalFileSet[f] {
				finalFileSet[f] = true
				finalFiles = append(finalFiles, f)
			}
		}
		mid.Title = mid.Title + " & " + last.Title
		mid.Description = mid.Description + "\n\n---\n\n" + last.Description
		mid.TargetFiles = finalFiles
		return []domain.Task{first, mid}
	}

	return []domain.Task{first, mid, last}
}
