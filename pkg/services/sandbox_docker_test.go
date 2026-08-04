package services

import (
	"reflect"
	"testing"
	"time"
)

func TestBuildDockerExecArgs(t *testing.T) {
	t.Run("when a deadline exists it prefixes the in-container command with timeout", func(t *testing.T) {
		args := buildDockerExecArgs("sandbox", "pkg", "go test ./...", 120)
		want := []string{"exec", "-w", "/app/pkg", "sandbox", "timeout", "120", "go", "test", "./..."}
		if !reflect.DeepEqual(args, want) {
			t.Errorf("expected %v, got %v", want, args)
		}
	})

	t.Run("when no deadline exists it omits the timeout prefix", func(t *testing.T) {
		args := buildDockerExecArgs("sandbox", "", "go test ./...", 0)
		want := []string{"exec", "-w", "/app/", "sandbox", "go", "test", "./..."}
		if !reflect.DeepEqual(args, want) {
			t.Errorf("expected %v, got %v", want, args)
		}
	})

	t.Run("when the command needs a shell it wraps with sh -c after the timeout prefix", func(t *testing.T) {
		args := buildDockerExecArgs("sandbox", "", "go vet ./... && go test ./...", 30)
		want := []string{"exec", "-w", "/app/", "sandbox", "timeout", "30", "sh", "-c", "go vet ./... && go test ./..."}
		if !reflect.DeepEqual(args, want) {
			t.Errorf("expected %v, got %v", want, args)
		}
	})
}

func TestNewDockerSandbox(t *testing.T) {
	t.Run("when constructed without options it leaves MaxDuration zero to default to 5 minutes", func(t *testing.T) {
		s := NewDockerSandbox("c")
		if s.MaxDuration != 0 {
			t.Errorf("expected zero-value MaxDuration, got %v", s.MaxDuration)
		}
		if defaultDockerMaxDuration != 5*time.Minute {
			t.Errorf("expected 5m default, got %v", defaultDockerMaxDuration)
		}
	})

	t.Run("when constructed with WithDockerMaxDuration it stores the custom duration", func(t *testing.T) {
		s := NewDockerSandbox("c", WithDockerMaxDuration(90*time.Second))
		if s.MaxDuration != 90*time.Second {
			t.Errorf("expected 90s, got %v", s.MaxDuration)
		}
	})
}
