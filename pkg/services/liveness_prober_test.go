package services

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"
)

type mockScriptRunner struct {
	output string
	err    error
}

func (m *mockScriptRunner) RunCommand(ctx context.Context, dir, command, input string) (string, error) {
	return m.output, m.err
}

func TestLivenessProber_DiagnoseParallel(t *testing.T) {
	// Start mock HTTP server
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	}))
	defer ts.Close()

	// Start mock TCP listener
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to listen: %v", err)
	}
	defer l.Close()
	tcpAddr := l.Addr().String()

	runner := &mockScriptRunner{output: "probe script passed", err: nil}
	prober := NewLivenessProber(runner, 2*time.Second)

	targets := []ProbeTarget{
		{Vector: VectorSocket, Address: tcpAddr, Required: true},
		{Vector: VectorHTTP, Address: ts.URL, Required: true},
		{Vector: VectorProcess, PID: os.Getpid(), Required: false},
		{Vector: VectorScript, Path: "tests/probe.sh", Required: true},
	}

	start := time.Now()
	diag := prober.DiagnoseParallel(context.Background(), "/dummy", targets)
	elapsed := time.Since(start)

	if !diag.Healthy {
		t.Fatalf("expected diagnosis to be healthy, got unhealthy: %s", diag.Summary)
	}
	if len(diag.Results) != 4 {
		t.Fatalf("expected 4 probe results, got %d", len(diag.Results))
	}

	// Verify all probes are alive
	for _, res := range diag.Results {
		if !res.Alive {
			t.Errorf("probe for vector %s (%s) failed unexpectedly: %s", res.Vector, res.Target, res.Error)
		}
	}

	// Ensure execution is concurrent and fast (< 1s)
	if elapsed > 1500*time.Millisecond {
		t.Errorf("diagnosis took unexpectedly long (%v), expected parallel execution", elapsed)
	}
}

func TestLivenessProber_FailureDetection(t *testing.T) {
	prober := NewLivenessProber(nil, 500*time.Millisecond)

	targets := []ProbeTarget{
		{Vector: VectorSocket, Address: "127.0.0.1:59999", Required: true}, // Closed port
		{Vector: VectorHTTP, Address: "http://127.0.0.1:59999/healthz", Required: true},
		{Vector: VectorProcess, PID: 99999999, Required: false},
	}

	diag := prober.DiagnoseParallel(context.Background(), "", targets)
	if diag.Healthy {
		t.Fatal("expected unhealthy diagnosis for dead targets")
	}
	if len(diag.Results) != 3 {
		t.Fatalf("expected 3 results, got %d", len(diag.Results))
	}
	for _, r := range diag.Results {
		if r.Alive {
			t.Errorf("expected target %s to be dead, but was reported alive", r.Target)
		}
	}
}

func TestLivenessProber_AutoDetect(t *testing.T) {
	tmpDir := t.TempDir()
	testsDir := filepath.Join(tmpDir, "tests")
	_ = os.MkdirAll(testsDir, 0755)
	probeFile := filepath.Join(testsDir, "probe_liveness.sh")
	_ = os.WriteFile(probeFile, []byte("#!/bin/sh\nexit 0"), 0755)

	prober := NewLivenessProber(nil, time.Second)
	detected := prober.AutoDetectTargets(tmpDir)

	if len(detected) != 1 {
		t.Fatalf("expected 1 detected probe, got %d", len(detected))
	}
	if detected[0].Path != "tests/probe_liveness.sh" {
		t.Errorf("unexpected path: %s", detected[0].Path)
	}
}
