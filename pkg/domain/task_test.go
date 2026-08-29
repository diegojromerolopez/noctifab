package domain

import (
	"encoding/json"
	"testing"
	"time"
)

func TestTaskFailureEnvelopeJSON(t *testing.T) {
	now := time.Now().UTC()
	env := TaskFailureEnvelope{
		Stage:        FailureStageAntiStub,
		Command:      "make test",
		ExitCode:     1,
		Stdout:       "stub detected",
		Stderr:       "error in src/main.py",
		FailingFiles: []string{"src/main.py"},
		WorktreeDiff: "+pass",
		Timestamp:    now,
	}

	task := Task{
		ID:              "task-001",
		Title:           "Implement Core Engine",
		Status:          TaskInProgress,
		FailureEnvelope: &env,
		StoryID:         "US-001",
	}

	bytes, err := json.Marshal(task)
	if err != nil {
		t.Fatalf("failed to marshal task: %v", err)
	}

	var unmarshaled Task
	if err := json.Unmarshal(bytes, &unmarshaled); err != nil {
		t.Fatalf("failed to unmarshal task: %v", err)
	}

	if unmarshaled.FailureEnvelope == nil {
		t.Fatal("expected FailureEnvelope to be non-nil")
	}
	if unmarshaled.FailureEnvelope.Stage != FailureStageAntiStub {
		t.Errorf("expected FailureStageAntiStub, got %s", unmarshaled.FailureEnvelope.Stage)
	}
	if unmarshaled.StoryID != "US-001" {
		t.Errorf("expected US-001, got %s", unmarshaled.StoryID)
	}
}
