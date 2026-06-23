package domain

import "time"

// Clarification holds questions raised by agents and replies from human operators.
type Clarification struct {
	ID       string    `json:"id"`
	Question string    `json:"question"`
	Answer   string    `json:"answer,omitempty"`
	Resolved bool      `json:"resolved"`
	AskedAt  time.Time `json:"asked_at"`
	TaskID   string    `json:"task_id,omitempty"`
}
