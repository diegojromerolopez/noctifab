package services

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestHandoffFile_RoundTrip(t *testing.T) {
	t.Run("when a handoff state is written and read back, it matches", func(t *testing.T) {
		dir := t.TempDir()
		hrm := &HotReloadManager{HandoffPath: filepath.Join(dir, "handoff.json")}

		original := HandoffState{NewPID: 12345, Status: HandoffHanding, Message: "test"}
		hrm.writeHandoff(original)

		got, err := hrm.readHandoff()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got.NewPID != original.NewPID {
			t.Errorf("NewPID: got %d, want %d", got.NewPID, original.NewPID)
		}
		if got.Status != original.Status {
			t.Errorf("Status: got %s, want %s", got.Status, original.Status)
		}
		if got.Message != original.Message {
			t.Errorf("Message: got %s, want %s", got.Message, original.Message)
		}
	})
}

func TestHandoffFile_JSON(t *testing.T) {
	t.Run("when a handoff state is written, the file contains valid JSON", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, "handoff.json")
		hrm := &HotReloadManager{HandoffPath: path}

		hrm.writeHandoff(HandoffState{NewPID: 42, Status: HandoffPending})

		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("failed to read handoff file: %v", err)
		}
		if !json.Valid(data) {
			t.Errorf("handoff file contains invalid JSON: %s", string(data))
		}

		var decoded HandoffState
		if err := json.Unmarshal(data, &decoded); err != nil {
			t.Fatalf("failed to unmarshal handoff JSON: %v", err)
		}
		if decoded.NewPID != 42 {
			t.Errorf("NewPID: got %d, want 42", decoded.NewPID)
		}
		if decoded.Status != HandoffPending {
			t.Errorf("Status: got %s, want %s", decoded.Status, HandoffPending)
		}
	})
}

func TestWaitForHealth_Success(t *testing.T) {
	t.Run("when the health endpoint returns 200, waitForHealth returns nil", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		}))
		defer srv.Close()

		hrm := &HotReloadManager{}
		ctx := context.Background()
		err := hrm.waitForHealth(ctx, srv.URL+"/healthz", 5*time.Second)
		if err != nil {
			t.Fatalf("expected nil, got: %v", err)
		}
	})
}

func TestWaitForHealth_Timeout(t *testing.T) {
	t.Run("when no server responds, waitForHealth returns an error after timeout", func(t *testing.T) {
		hrm := &HotReloadManager{}
		ctx := context.Background()
		err := hrm.waitForHealth(ctx, "http://127.0.0.1:15999/healthz", 100*time.Millisecond)
		if err == nil {
			t.Fatal("expected timeout error, got nil")
		}
	})
}

func TestWaitForActive_Success(t *testing.T) {
	t.Run("when handoff transitions to active, waitForActive returns nil", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, "handoff.json")
		hrm := &HotReloadManager{HandoffPath: path}

		hrm.writeHandoff(HandoffState{NewPID: 99, Status: HandoffHanding})

		go func() {
			time.Sleep(50 * time.Millisecond)
			hrm.writeHandoff(HandoffState{NewPID: 99, Status: HandoffActive})
		}()

		ctx := context.Background()
		err := hrm.waitForActive(ctx, 5*time.Second)
		if err != nil {
			t.Fatalf("expected nil, got: %v", err)
		}
	})
}

func TestWaitForActive_Timeout(t *testing.T) {
	t.Run("when handoff stays at handing_off, waitForActive returns an error after timeout", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, "handoff.json")
		hrm := &HotReloadManager{HandoffPath: path}

		hrm.writeHandoff(HandoffState{NewPID: 99, Status: HandoffHanding})

		ctx := context.Background()
		err := hrm.waitForActive(ctx, 100*time.Millisecond)
		if err == nil {
			t.Fatal("expected timeout error, got nil")
		}
	})
}

func TestHandoffFile_Missing(t *testing.T) {
	t.Run("when the handoff file does not exist, readHandoff returns os.ErrNotExist", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, "nonexistent.json")
		hrm := &HotReloadManager{HandoffPath: path}

		_, err := hrm.readHandoff()
		if err == nil {
			t.Fatal("expected error, got nil")
		}
		if !os.IsNotExist(err) {
			t.Errorf("expected os.ErrNotExist, got: %v", err)
		}
	})
}

func TestHandoffFile_Corrupted(t *testing.T) {
	t.Run("when the handoff file contains invalid JSON, readHandoff returns an unmarshal error", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, "handoff.json")
		hrm := &HotReloadManager{HandoffPath: path}

		if err := os.WriteFile(path, []byte("not-json"), 0644); err != nil {
			t.Fatalf("failed to write corrupted handoff: %v", err)
		}

		_, err := hrm.readHandoff()
		if err == nil {
			t.Fatal("expected error, got nil")
		}

		if _, ok := err.(*json.SyntaxError); !ok {
			t.Errorf("expected json.SyntaxError, got: %T %v", err, err)
		}
	})
}
