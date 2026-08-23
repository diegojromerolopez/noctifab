package services

import (
	"context"
	"path/filepath"
	"testing"
	"time"
)

func TestHostQABuildSandbox_Run(t *testing.T) {
	workspace := t.TempDir()
	process := &recordingQAProcess{results: []QAProcessResult{
		{Stdout: "built ok", ExitCode: 0},
	}}
	sandbox := NewHostQABuildSandbox(process, OSQAFileSystem{}, time.Minute)

	result, err := sandbox.Run(context.Background(), ReviewWorkspace{Path: workspace},
		[]string{"make", "build"}, 1024)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Stdout != "built ok" {
		t.Fatalf("expected stdout 'built ok', got %q", result.Stdout)
	}
	if len(process.processes) != 1 {
		t.Fatalf("expected 1 process call, got %d", len(process.processes))
	}
	if process.processes[0].Name != "make" || len(process.processes[0].Args) != 1 || process.processes[0].Args[0] != "build" {
		t.Fatalf("unexpected process command: %#v", process.processes[0])
	}
}

func TestHostQABuildSandbox_SkipsServerDaemon(t *testing.T) {
	workspace := t.TempDir()
	process := &recordingQAProcess{results: []QAProcessResult{}}
	sandbox := NewHostQABuildSandbox(process, OSQAFileSystem{}, time.Minute)

	result, err := sandbox.Run(context.Background(), ReviewWorkspace{Path: workspace},
		[]string{"make", "run"}, 1024)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.ExitCode != 0 {
		t.Fatalf("expected exit code 0 on skipped daemon build, got %d", result.ExitCode)
	}
	if len(process.processes) != 0 {
		t.Fatalf("expected 0 process calls for daemon command, got %d", len(process.processes))
	}
}

func TestHostQASandboxRunner_VerifyAndRun(t *testing.T) {
	base := t.TempDir()
	source := filepath.Join(base, "source")
	artifact := filepath.Join(base, "artifact")
	runtime := filepath.Join(base, "runtime")

	fsys := OSQAFileSystem{}
	_ = fsys.MkdirAll(source, 0755)
	_ = fsys.MkdirAll(artifact, 0755)
	_ = fsys.MkdirAll(runtime, 0755)

	process := &recordingQAProcess{results: []QAProcessResult{
		{Stdout: "test pass", ExitCode: 0},
	}}
	runner := NewHostQASandboxRunner(process, fsys, time.Minute)

	if err := runner.Verify(context.Background(), source, artifact, runtime); err != nil {
		t.Fatalf("Verify() failed: %v", err)
	}

	result, err := runner.Run(context.Background(), QACommand{
		Argv:        []string{"bash", "test.sh"},
		Timeout:     10 * time.Second,
		OutputLimit: 1024,
	})
	if err != nil {
		t.Fatalf("Run() failed: %v", err)
	}
	if result.Stdout != "test pass" {
		t.Fatalf("expected stdout 'test pass', got %q", result.Stdout)
	}
	if len(process.processes) != 1 {
		t.Fatalf("expected 1 process execution, got %d", len(process.processes))
	}
	if process.processes[0].Dir != runtime {
		t.Fatalf("expected execution dir %q, got %q", runtime, process.processes[0].Dir)
	}
}
