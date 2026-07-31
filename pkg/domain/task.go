package domain

import "time"

// TaskStatus classifies the topological execution state of a work plan item.
type TaskStatus string

const (
	// TaskPending represents a task awaiting execution or parent dependency resolution.
	TaskPending TaskStatus = "PENDING"
	// TaskInProgress represents a task actively assigned to a generator agent goroutine.
	TaskInProgress TaskStatus = "IN_PROGRESS"
	// TaskSuccess represents a task successfully validated, merged, and completed.
	TaskSuccess TaskStatus = "SUCCESS"
	// TaskFailed represents a task failing build or tests beyond retry threshold.
	TaskFailed TaskStatus = "FAILED"
	// TaskConflictBlocked represents a task temporarily halted due to Git conflicts.
	TaskConflictBlocked TaskStatus = "CONFLICT_BLOCKED"
	// TaskConflictFailed represents a task aborted due to continuous OCC conflicts.
	TaskConflictFailed TaskStatus = "CONFLICT_FAILED"
	// TaskInterrupted represents a task suspended during graceful daemon shutdown.
	TaskInterrupted TaskStatus = "INTERRUPTED"
)

// ChangeType classifies the scope of a task's adjustments for semver release bumping.
type ChangeType string

const (
	// ChangeTypeFeature triggers minor version upgrades (+0.1.0).
	ChangeTypeFeature ChangeType = "FEATURE"
	// ChangeTypeFix triggers patch version upgrades (+0.0.1).
	ChangeTypeFix ChangeType = "FIX"
	// ChangeTypeBreaking triggers major version upgrades (+1.0.0).
	ChangeTypeBreaking ChangeType = "BREAKING"
)

// Task represents a specific item in the scheduling graph.
type Task struct {
	ID               string     `json:"id"`
	Title            string     `json:"title"`
	Description      string     `json:"description"`
	Status           TaskStatus `json:"status"`
	ChangeType       ChangeType `json:"change_type"`
	AssignedTo       string     `json:"assigned_to"`
	Progress         int        `json:"progress"`   // Completion percentage (0 to 100)
	DependsOn        []string   `json:"depends_on"` // Can store parent task IDs or matching titles
	TargetFiles      []string   `json:"target_files,omitempty"`
	PartialChangelog []string   `json:"partial_changelog,omitempty"`
	FailureLog       string     `json:"failure_log,omitempty"`
	Retries          int        `json:"retries"`
	MaxRetries       int        `json:"max_retries"`
	CreatedAt        time.Time  `json:"created_at"`
	UpdatedAt        time.Time  `json:"updated_at"`
}
