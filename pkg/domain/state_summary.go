package domain

import "time"

// StateSummary is the lightweight per-story projection of a State. It
// intentionally omits Files, LastActions, and full task bodies so status
// listings stay cheap even with a long story history.
type StateSummary struct {
	ID                string         `json:"id"`
	FeatureName       string         `json:"feature_name,omitempty"`
	InputPath         string         `json:"input_path,omitempty"`
	IntegrationBranch string         `json:"integration_branch,omitempty"`
	BaseBranch        string         `json:"base_branch,omitempty"`
	StoryStatus       string         `json:"story_status"`
	StoryError        string         `json:"story_error,omitempty"`
	BuildStatus       string         `json:"build_status"`
	Version           int            `json:"version"`
	TotalTasks        int            `json:"total_tasks"`
	TaskCounts        map[string]int `json:"task_counts"`
	CreatedAt         time.Time      `json:"created_at,omitempty"`
	UpdatedAt         time.Time      `json:"updated_at,omitempty"`
}

// SummarizeState projects a full domain State into its lightweight summary.
func SummarizeState(s *State) StateSummary {
	summary := StateSummary{
		ID:                s.ID,
		FeatureName:       s.Metadata.FeatureName,
		InputPath:         s.Metadata.InputPath,
		IntegrationBranch: s.Metadata.IntegrationBranch,
		BaseBranch:        s.Metadata.BaseBranch,
		StoryStatus:       string(s.StoryStatus),
		StoryError:        s.StoryError,
		BuildStatus:       string(s.BuildStatus),
		Version:           s.Version,
		TotalTasks:        len(s.Tasks),
		TaskCounts:        make(map[string]int),
	}
	for _, t := range s.Tasks {
		summary.TaskCounts[string(t.Status)]++
		if summary.CreatedAt.IsZero() || (!t.CreatedAt.IsZero() && t.CreatedAt.Before(summary.CreatedAt)) {
			summary.CreatedAt = t.CreatedAt
		}
		if t.UpdatedAt.After(summary.UpdatedAt) {
			summary.UpdatedAt = t.UpdatedAt
		}
	}
	return summary
}

// IsTerminal reports whether the story reached a terminal outcome
// (SUCCESS or FAILED). Paused and cancelled stories are not considered
// terminal for retention purposes: they may still be resumed or inspected.
func (s StoryStatus) IsTerminal() bool {
	return s == StorySuccess || s == StoryFailed
}
