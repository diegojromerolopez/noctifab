package services

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/diegojromerolopez/noctifab/pkg/domain"
)

var (
	taskNumSuffixRE = regexp.MustCompile(`(?i)(?:TASK-?|T-?)(\d+)$`)
	digitsOnlyRE    = regexp.MustCompile(`^(\d+)$`)
)

// NormalizeAndDeduplicateTasks sanitizes task IDs to strictly belong to targetStoryID,
// prevents section-number aliasing, updates internal task dependencies to the rewritten IDs,
// and ensures global task ID uniqueness across existing workspace tasks.
func NormalizeAndDeduplicateTasks(tasks []domain.Task, targetStoryID string, existingTasks []domain.Task) []domain.Task {
	if len(tasks) == 0 {
		return tasks
	}

	canonicalStoryID := ExtractStoryID(targetStoryID)
	if canonicalStoryID == "" {
		canonicalStoryID = strings.TrimSpace(targetStoryID)
	}

	// Index existing task IDs to prevent cross-story collisions in the database.
	existingIDs := make(map[string]bool, len(existingTasks))
	for _, et := range existingTasks {
		if et.ID != "" {
			existingIDs[et.ID] = true
		}
	}

	idMap := make(map[string]string)
	normalized := make([]domain.Task, len(tasks))
	copy(normalized, tasks)

	usedInBatch := make(map[string]bool)

	for i := range normalized {
		task := &normalized[i]
		if canonicalStoryID != "" {
			task.StoryID = canonicalStoryID
		}

		oldID := strings.TrimSpace(task.ID)
		desiredSeq := i + 1

		// Attempt to extract existing sequence number if present in oldID
		if matches := taskNumSuffixRE.FindStringSubmatch(oldID); len(matches) > 1 {
			if s, err := strconv.Atoi(matches[1]); err == nil && s > 0 {
				desiredSeq = s
			}
		} else if matches := digitsOnlyRE.FindStringSubmatch(oldID); len(matches) > 1 {
			if s, err := strconv.Atoi(matches[1]); err == nil && s > 0 {
				desiredSeq = s
			}
		}

		// Compute canonical task ID prefix
		prefix := canonicalStoryID
		if prefix == "" {
			prefix = "TASK"
		}

		var candidateID string
		extractedStory := ExtractStoryID(oldID)
		hasMismatchedStory := extractedStory != "" && canonicalStoryID != "" && extractedStory != canonicalStoryID

		if hasMismatchedStory || oldID == "" {
			candidateID = fmt.Sprintf("%s-TASK-%03d", prefix, desiredSeq)
		} else if usedInBatch[oldID] || existingIDs[oldID] {
			candidateID = fmt.Sprintf("%s-TASK-%03d", prefix, desiredSeq)
		} else {
			candidateID = oldID
		}

		// Disambiguate if candidateID already used in this batch or in existing database tasks
		if usedInBatch[candidateID] || existingIDs[candidateID] {
			seq := desiredSeq
			for usedInBatch[candidateID] || existingIDs[candidateID] {
				seq++
				candidateID = fmt.Sprintf("%s-TASK-%03d", prefix, seq)
			}
		}

		usedInBatch[candidateID] = true
		if oldID != "" {
			idMap[oldID] = candidateID
		}
		task.ID = candidateID
	}

	// Remap intra-story dependencies to point to the newly normalized IDs
	for i := range normalized {
		task := &normalized[i]
		if len(task.DependsOn) == 0 {
			continue
		}
		var updatedDeps []string
		for _, dep := range task.DependsOn {
			depTrimmed := strings.TrimSpace(dep)
			if mapped, ok := idMap[depTrimmed]; ok && mapped != "" {
				if mapped != task.ID {
					updatedDeps = append(updatedDeps, mapped)
				}
			} else {
				updatedDeps = append(updatedDeps, depTrimmed)
			}
		}
		task.DependsOn = updatedDeps
	}

	return normalized
}
