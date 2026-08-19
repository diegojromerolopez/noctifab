package domain

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"time"
)

// SpecTurnKind identifies whether a turn is initial creation, an interactive refinement, an audit, or a rollback.
type SpecTurnKind string

const (
	// SpecTurnInitial represents the first drafting pass from the initial prompt.
	SpecTurnInitial SpecTurnKind = "INITIAL"
	// SpecTurnRefine represents an interactive human refinement turn.
	SpecTurnRefine SpecTurnKind = "REFINE"
	// SpecTurnAudit represents a multi-model consensus audit pass.
	SpecTurnAudit SpecTurnKind = "AUDIT"
	// SpecTurnRollback represents a time-travel rollback turn.
	SpecTurnRollback SpecTurnKind = "ROLLBACK"
)

// SpecRevision represents one immutable version of the specification in the HITL session.
type SpecRevision struct {
	Version      int               `json:"version"`
	Content      string            `json:"content"`
	SnapshotPath string            `json:"snapshot_path,omitempty"`
	SHA256       string            `json:"sha256,omitempty"`
	Prompt       string            `json:"prompt"`
	Kind         SpecTurnKind      `json:"kind"`
	ParentVer    int               `json:"parent_version,omitempty"`
	DiffSummary  string            `json:"diff_summary,omitempty"`
	TokensUsed   int64             `json:"tokens_used"`
	CostUSD      string            `json:"cost_usd,omitempty"`
	ModelRoles   map[string]string `json:"model_roles,omitempty"`
	CreatedAt    time.Time         `json:"created_at"`
}

// SpecSession manages the stateful multi-turn HITL review session.
type SpecSession struct {
	ID             string         `json:"id"`
	ProjectPath    string         `json:"project_path"`
	TargetFile     string         `json:"target_file"`
	ActiveVerIndex int            `json:"active_version_index"`
	Revisions      []SpecRevision `json:"revisions"`
	CurrentSpec    string         `json:"current_spec"`
	IsComplete     bool           `json:"is_complete"`
	CreatedAt      time.Time      `json:"created_at"`
	UpdatedAt      time.Time      `json:"updated_at"`
}

// LatestRevision returns the most recent revision of the specification, or nil if none exists.
func (s *SpecSession) LatestRevision() *SpecRevision {
	if s == nil || len(s.Revisions) == 0 {
		return nil
	}
	return &s.Revisions[len(s.Revisions)-1]
}

// ActiveRevision returns the currently checked-out active revision of the specification.
func (s *SpecSession) ActiveRevision() *SpecRevision {
	if s == nil || len(s.Revisions) == 0 {
		return nil
	}
	if s.ActiveVerIndex < 0 || s.ActiveVerIndex >= len(s.Revisions) {
		return s.LatestRevision()
	}
	return &s.Revisions[s.ActiveVerIndex]
}

// CanUndo returns true if there is a previous historical revision.
func (s *SpecSession) CanUndo() bool {
	if s == nil || len(s.Revisions) <= 1 {
		return false
	}
	return s.ActiveVerIndex > 0
}

// CanRedo returns true if a forward revision exists after an undo.
func (s *SpecSession) CanRedo() bool {
	if s == nil || len(s.Revisions) == 0 {
		return false
	}
	return s.ActiveVerIndex < len(s.Revisions)-1
}

// Undo steps back to the previous historical revision.
func (s *SpecSession) Undo() (*SpecRevision, error) {
	if !s.CanUndo() {
		return nil, fmt.Errorf("cannot undo: already at earliest revision")
	}
	s.ActiveVerIndex--
	rev := &s.Revisions[s.ActiveVerIndex]
	s.CurrentSpec = rev.Content
	s.UpdatedAt = time.Now().UTC()
	return rev, nil
}

// Redo steps forward to the next revision.
func (s *SpecSession) Redo() (*SpecRevision, error) {
	if !s.CanRedo() {
		return nil, fmt.Errorf("cannot redo: already at latest revision")
	}
	s.ActiveVerIndex++
	rev := &s.Revisions[s.ActiveVerIndex]
	s.CurrentSpec = rev.Content
	s.UpdatedAt = time.Now().UTC()
	return rev, nil
}

// Checkout jumps to a specific 1-indexed version number.
func (s *SpecSession) Checkout(version int) (*SpecRevision, error) {
	if s == nil || len(s.Revisions) == 0 {
		return nil, fmt.Errorf("no revisions available")
	}
	if version < 1 || version > len(s.Revisions) {
		return nil, fmt.Errorf("invalid version %d (available: 1-%d)", version, len(s.Revisions))
	}
	s.ActiveVerIndex = version - 1
	rev := &s.Revisions[s.ActiveVerIndex]
	s.CurrentSpec = rev.Content
	s.UpdatedAt = time.Now().UTC()
	return rev, nil
}

// TotalTokensUsed calculates the cumulative tokens across all revisions in the session.
func (s *SpecSession) TotalTokensUsed() int64 {
	if s == nil {
		return 0
	}
	var total int64
	for _, rev := range s.Revisions {
		total += rev.TokensUsed
	}
	return total
}

// AddRevision appends a new revision and updates the current spec content and active version index.
func (s *SpecSession) AddRevision(content, prompt string, kind SpecTurnKind, diffSummary string, tokensUsed int64, costUSD string) SpecRevision {
	hasher := sha256.New()
	hasher.Write([]byte(content))
	sha := hex.EncodeToString(hasher.Sum(nil))

	parentVer := 0
	if active := s.ActiveRevision(); active != nil {
		parentVer = active.Version
	}

	rev := SpecRevision{
		Version:     len(s.Revisions) + 1,
		Content:     content,
		SHA256:      sha,
		Prompt:      prompt,
		Kind:        kind,
		ParentVer:   parentVer,
		CreatedAt:   time.Now().UTC(),
		DiffSummary: diffSummary,
		TokensUsed:  tokensUsed,
		CostUSD:     costUSD,
	}
	s.Revisions = append(s.Revisions, rev)
	s.ActiveVerIndex = len(s.Revisions) - 1
	s.CurrentSpec = content
	s.UpdatedAt = rev.CreatedAt
	return rev
}
