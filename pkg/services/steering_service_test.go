package services

import (
	"testing"
)

func TestSteeringService_TaskDirectives(t *testing.T) {
	svc := NewSteeringService()

	err := svc.InjectDirective("task-1", "Use SQLite instead of in-memory map")
	if err != nil {
		t.Fatalf("unexpected error injecting directive: %v", err)
	}

	err = svc.InjectDirective("task-1", "Ensure proper index on email field")
	if err != nil {
		t.Fatalf("unexpected error injecting directive 2: %v", err)
	}

	// First consumption gets both directives
	dirs := svc.ConsumeDirectives("task-1")
	if len(dirs) != 2 {
		t.Fatalf("expected 2 directives, got %d", len(dirs))
	}
	if dirs[0] != "Use SQLite instead of in-memory map" {
		t.Errorf("got %q, want expected directive text", dirs[0])
	}

	// Second consumption returns empty because directives are marked consumed
	dirs2 := svc.ConsumeDirectives("task-1")
	if len(dirs2) != 0 {
		t.Errorf("expected 0 unconsumed directives on second read, got %d", len(dirs2))
	}
}

func TestSteeringService_GlobalDirectives(t *testing.T) {
	svc := NewSteeringService()

	_ = svc.InjectGlobalDirective("Global architecture directive: strict DDD")

	dirs := svc.ConsumeDirectives("task-any")
	if len(dirs) != 1 || dirs[0] != "Global architecture directive: strict DDD" {
		t.Fatalf("expected global directive to be returned, got %v", dirs)
	}

	dirsAfter := svc.ConsumeDirectives("task-any")
	if len(dirsAfter) != 0 {
		t.Errorf("expected 0 directives after consumption, got %d", len(dirsAfter))
	}
}

func TestSteeringService_PauseResume(t *testing.T) {
	svc := NewSteeringService()

	if svc.IsPaused() {
		t.Errorf("expected initially not paused")
	}

	svc.Pause()
	if !svc.IsPaused() {
		t.Errorf("expected isPaused=true after Pause()")
	}

	select {
	case <-svc.PauseChan():
	default:
		t.Errorf("expected signal on PauseChan")
	}

	svc.Resume()
	if svc.IsPaused() {
		t.Errorf("expected isPaused=false after Resume()")
	}

	select {
	case <-svc.ResumeChan():
	default:
		t.Errorf("expected signal on ResumeChan")
	}
}
