package domain

import "time"

// ValidationType specifies the category of compliance check being run.
type ValidationType string

const (
	// ValidationCommand executes shell verification test suites (e.g. go test).
	ValidationCommand ValidationType = "COMMAND"
	// ValidationFileExists checks for the existence of a target file.
	ValidationFileExists ValidationType = "FILE_EXISTS"
	// ValidationFileContent executes regex checks over file contents.
	ValidationFileContent ValidationType = "FILE_CONTENT"
)

// ValidationCriterion defines a quality checklist item used to evaluate task goals.
type ValidationCriterion struct {
	ID          string         `json:"id"`
	Type        ValidationType `json:"type"`
	Expression  string         `json:"expression"` // Command line, filepath, or regex target
	Description string         `json:"description"`
	Passed      bool           `json:"passed"`
	ErrorLog    string         `json:"error_log,omitempty"`
}

// AgentRole defines the function an agent performs in the orchestration pipeline.
type AgentRole string

const (
	// AgentRolePlanner decomposes specifications into task DAGs.
	AgentRolePlanner AgentRole = "PLANNER"
	// AgentRoleGenerator writes code and executes tool actions.
	AgentRoleGenerator AgentRole = "GENERATOR"
	// AgentRoleTester writes tests and validates output.
	AgentRoleTester AgentRole = "TESTER"
	// AgentRoleResolver resolves source code merge conflicts.
	AgentRoleResolver AgentRole = "RESOLVER"
)

// AgentStatus tracks the lifecycle state of a worker agent.
type AgentStatus string

const (
	// AgentIdle represents an agent available for task assignment.
	AgentIdle AgentStatus = "IDLE"
	// AgentWorking represents an agent actively executing a task.
	AgentWorking AgentStatus = "WORKING"
	// AgentCompleted represents an agent that has finished its task.
	AgentCompleted AgentStatus = "COMPLETED"
)

// Agent represents a processing worker in the execution environment.
type Agent struct {
	ID          string      `json:"id"`
	Name        string      `json:"name"`
	Role        AgentRole   `json:"role"`
	Status      AgentStatus `json:"status"`
	TaskID      string      `json:"task_id,omitempty"`
	StartedAt   time.Time   `json:"started_at,omitempty"`
	CompletedAt time.Time   `json:"completed_at,omitempty"`
	TokensUsed  int64       `json:"tokens_used"`
	LastError   string      `json:"last_error,omitempty"`
}

// FileInfo contains simple metadata about files inside the workspace.
type FileInfo struct {
	Path         string    `json:"path"`
	Size         int64     `json:"size"`
	LastModified time.Time `json:"last_modified"`
}

// BuildStatus tracks the overall health of the workspace build.
type BuildStatus string

const (
	// BuildPassing indicates that compilation, formatting, and all test validations passed.
	BuildPassing BuildStatus = "PASSING"
	// BuildFailing indicates that one or more checks failed.
	BuildFailing BuildStatus = "FAILING"
	// BuildUnknown indicates that verification checks have not been run.
	BuildUnknown BuildStatus = "UNKNOWN"
)

// StateMetadata holds structured session parameters and cost aggregations.
type StateMetadata struct {
	InputSource       string `json:"input_source"`             // Source of the specification (e.g., "markdown", "jira", "github")
	InputPath         string `json:"input_path"`               // Original path or URL of the specification
	IntegrationBranch string `json:"integration_branch"`       // Feature integration branch name (e.g., "feature/feature-auth")
	FeatureName       string `json:"feature_name"`             // Human-readable name of the feature being built
	BaseBranch        string `json:"base_branch"`              // Branch from which the integration branch was created (e.g., "main")
	ProjectVersion    string `json:"project_version"`          // Current project version from VERSION file (e.g., "0.0.1")
	TotalTokensUsed   int64  `json:"total_tokens_used"`        // Cumulative token count across all agents
	TotalCostUSD      string `json:"total_cost_usd,omitempty"` // Estimated LLM API cost in USD
}

// StoryStatus tracks the lifecycle of a user story being processed by the daemon.
type StoryStatus string

const (
	StoryIdle      StoryStatus = ""
	StoryRunning   StoryStatus = "RUNNING"
	StorySuccess   StoryStatus = "SUCCESS"
	StoryFailed    StoryStatus = "FAILED"
	StoryPaused    StoryStatus = "PAUSED"
	StoryCancelled StoryStatus = "CANCELLED"
)

// State represents the complete system database state record.
type State struct {
	ID                 string                `json:"id"`
	ProjectPath        string                `json:"project_path"`
	Version            int                   `json:"version"` // Optimistic Concurrency version tag
	Clarifications     []Clarification       `json:"clarifications,omitempty"`
	ValidationCriteria []ValidationCriterion `json:"validation_criteria,omitempty"`
	Tasks              []Task                `json:"tasks"`
	ActiveAgents       []Agent               `json:"active_agents"`
	Files              []FileInfo            `json:"files"`
	BuildStatus        BuildStatus           `json:"build_status"`
	LastActions        []Action              `json:"last_actions"`
	Metadata           StateMetadata         `json:"metadata"`
	StoryStatus        StoryStatus           `json:"story_status"`
	StoryError         string                `json:"story_error,omitempty"`
}
