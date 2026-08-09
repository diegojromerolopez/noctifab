package domain

// PublicContract defines observable behavior exposed by a story.
type PublicContract struct {
	ID                     string   `json:"id"`
	Interface              string   `json:"interface"`
	ApplicablePathPrefixes []string `json:"applicable_path_prefixes,omitempty"`
	AllowedExecutables     []string `json:"allowed_executables"`
	ExitCodes              []int    `json:"exit_codes,omitempty"`
	StdoutContains         []string `json:"stdout_contains,omitempty"`
	StderrPrefixes         []string `json:"stderr_prefixes,omitempty"`
}

// StoryContract contains the normalized public contract for a roadmap story.
type StoryContract struct {
	StoryID         string           `json:"story_id"`
	SourcePath      string           `json:"source_path"`
	SourceSHA256    string           `json:"source_sha256"`
	PublicContracts []PublicContract `json:"public_contracts"`
}
