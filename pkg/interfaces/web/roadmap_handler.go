package web

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/diegojromerolopez/noctifab/pkg/domain"
)

// RoadmapStory represents a parsed user story for the web dashboard and Spec Studio.
type RoadmapStory struct {
	ID                  string   `json:"id"`
	Title               string   `json:"title"`
	Filename            string   `json:"filename"`
	Content             string   `json:"content"`
	DefinitionOfDone    string   `json:"definition_of_done,omitempty"`
	AcceptanceCriteria  []string `json:"acceptance_criteria,omitempty"`
	TotalCheckboxes     int      `json:"total_checkboxes"`
	CompletedCheckboxes int      `json:"completed_checkboxes"`
	Progress            int      `json:"progress"`
	Status              string   `json:"status"` // PENDING, RUNNING, SUCCESS, FAILED
}

// registerRoadmapRoutes registers roadmap story endpoints on the web server.
func (ws *WebServer) registerRoadmapRoutes(mux *http.ServeMux) {
	// GET /api/v1/roadmap — List all user stories with DoD and progress
	mux.HandleFunc("/api/v1/roadmap", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		baseDir := ws.getProjectBaseDir(r.Context())
		stories := ws.loadRoadmapStories(r.Context(), baseDir)

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(stories)
	})

	// GET /api/v1/states — List all story states from database repository
	mux.HandleFunc("/api/v1/states", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		if ws.repo == nil {
			http.Error(w, "state repository unavailable", http.StatusServiceUnavailable)
			return
		}
		states, err := ws.repo.LoadAll(r.Context())
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(states)
	})
}

// loadRoadmapStories scans and parses user story markdown files in the workspace.
func (ws *WebServer) loadRoadmapStories(ctx context.Context, baseDir string) []RoadmapStory {
	storiesDir := filepath.Join(baseDir, "roadmap", "user-stories")
	var files []string

	if matches, err := filepath.Glob(filepath.Join(storiesDir, "*.md")); err == nil {
		files = append(files, matches...)
	}
	if matches, err := filepath.Glob(filepath.Join(baseDir, "roadmap", "US-*.md")); err == nil {
		files = append(files, matches...)
	}
	sort.Strings(files)

	// Fetch states to correlate progress and status
	stateMap := make(map[string]*domain.State)
	if ws.repo != nil {
		if allStates, err := ws.repo.LoadAll(ctx); err == nil {
			for _, st := range allStates {
				if st.ID != "" {
					stateMap[strings.ToUpper(st.ID)] = st
				}
				if st.Metadata.FeatureName != "" {
					stateMap[strings.ToUpper(st.Metadata.FeatureName)] = st
				}
			}
		}
	}

	var results []RoadmapStory
	seen := make(map[string]bool)

	for _, file := range files {
		base := filepath.Base(file)
		if seen[base] {
			continue
		}
		seen[base] = true

		data, err := os.ReadFile(file)
		if err != nil {
			continue
		}
		content := string(data)
		story := parseStoryMarkdown(base, content)

		// Check if state database has matching execution progress
		lookupKey := strings.ToUpper(story.ID)
		if st, ok := stateMap[lookupKey]; ok {
			story.Status = string(st.StoryStatus)
			if len(st.Tasks) > 0 {
				total := 0
				for _, t := range st.Tasks {
					total += t.Progress
				}
				story.Progress = total / len(st.Tasks)
			}
		} else if story.Status == "" {
			if story.TotalCheckboxes > 0 && story.CompletedCheckboxes == story.TotalCheckboxes {
				story.Status = "SUCCESS"
				story.Progress = 100
			} else if story.CompletedCheckboxes > 0 {
				story.Status = "RUNNING"
				story.Progress = int(float64(story.CompletedCheckboxes) / float64(story.TotalCheckboxes) * 100)
			} else {
				story.Status = "PENDING"
				story.Progress = 0
			}
		}

		results = append(results, story)
	}

	return results
}

// parseStoryMarkdown parses a single user story Markdown content.
func parseStoryMarkdown(filename, content string) RoadmapStory {
	story := RoadmapStory{
		Filename: filename,
		Content:  content,
		Status:   "PENDING",
	}

	// Extract ID (e.g. US-001)
	base := strings.TrimSuffix(filename, filepath.Ext(filename))
	parts := strings.Split(base, "-")
	if len(parts) >= 2 && strings.EqualFold(parts[0], "US") {
		story.ID = fmt.Sprintf("US-%s", parts[1])
	} else if strings.HasPrefix(strings.ToUpper(base), "US-") {
		story.ID = base
	} else {
		story.ID = base
	}

	lines := strings.Split(content, "\n")
	inDoD := false
	var dodLines []string

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)

		// Extract Title
		if story.Title == "" && strings.HasPrefix(trimmed, "# ") {
			title := strings.TrimPrefix(trimmed, "# ")
			if idx := strings.Index(title, ":"); idx != -1 {
				title = title[idx+1:]
			}
			title = strings.TrimSpace(title)
			if strings.HasPrefix(title, story.ID) {
				title = strings.TrimSpace(strings.TrimPrefix(title, story.ID))
				title = strings.TrimSpace(strings.TrimPrefix(title, "-"))
				title = strings.TrimSpace(strings.TrimPrefix(title, ":"))
			}
			story.Title = strings.TrimSpace(title)
		}

		// Track Definition of Done section
		if strings.HasPrefix(trimmed, "## ") {
			lower := strings.ToLower(trimmed)
			if strings.Contains(lower, "definition of done") || strings.Contains(lower, "acceptance criteria") {
				inDoD = true
				continue
			} else {
				inDoD = false
			}
		}

		if inDoD && trimmed != "" {
			dodLines = append(dodLines, trimmed)
			if strings.HasPrefix(trimmed, "- ") || strings.HasPrefix(trimmed, "* ") || strings.HasPrefix(trimmed, "1.") {
				cleanCrit := strings.TrimLeft(trimmed, "-*0123456789. ")
				story.AcceptanceCriteria = append(story.AcceptanceCriteria, cleanCrit)
			}
		}

		// Count Checkboxes
		if strings.Contains(trimmed, "[ ]") {
			story.TotalCheckboxes++
		} else if strings.Contains(trimmed, "[x]") || strings.Contains(trimmed, "[X]") {
			story.TotalCheckboxes++
			story.CompletedCheckboxes++
		}
	}

	if story.Title == "" {
		story.Title = story.ID
	}

	if len(dodLines) > 0 {
		story.DefinitionOfDone = strings.Join(dodLines, "\n")
	}

	if story.TotalCheckboxes > 0 {
		story.Progress = int(float64(story.CompletedCheckboxes) / float64(story.TotalCheckboxes) * 100)
	}

	return story
}
