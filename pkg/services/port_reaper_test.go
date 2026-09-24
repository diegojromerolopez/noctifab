package services

import (
	"context"
	"net"
	"os"
	"path/filepath"
	"syscall"
	"testing"
)

func TestDetectProjectPorts(t *testing.T) {
	tempDir := t.TempDir()

	specContent := `# Pyedis Spec
The server exposes a Redis RESP wire protocol on port 6379.
It can also be started with --port 6380.
Connect via 127.0.0.1:6379 or localhost:8080.
`
	if err := os.WriteFile(filepath.Join(tempDir, "SPEC.md"), []byte(specContent), 0644); err != nil {
		t.Fatalf("write SPEC.md failed: %v", err)
	}

	composeContent := `services:
  app:
    ports:
      - "9000:9000"
`
	if err := os.WriteFile(filepath.Join(tempDir, "docker-compose.yml"), []byte(composeContent), 0644); err != nil {
		t.Fatalf("write docker-compose.yml failed: %v", err)
	}

	ports, err := DetectProjectPorts(tempDir)
	if err != nil {
		t.Fatalf("DetectProjectPorts failed: %v", err)
	}

	expected := map[int]bool{
		6379: true,
		6380: true,
		8080: true,
		9000: true,
	}

	if len(ports) != len(expected) {
		t.Fatalf("expected %d ports, got %d: %v", len(expected), len(ports), ports)
	}

	for _, p := range ports {
		if !expected[p] {
			t.Errorf("unexpected port detected: %d", p)
		}
	}
}

func TestIsDaemonProject(t *testing.T) {
	tempDirCLI := t.TempDir()
	cliSpec := `# Calculator CLI
A simple calculator command line tool that adds numbers.
Exit code 0 on success.
`
	if err := os.WriteFile(filepath.Join(tempDirCLI, "SPEC.md"), []byte(cliSpec), 0644); err != nil {
		t.Fatalf("write SPEC.md failed: %v", err)
	}

	if IsDaemonProject(tempDirCLI) {
		t.Errorf("expected calculator CLI NOT to be classified as daemon project")
	}

	tempDirDaemon := t.TempDir()
	daemonSpec := `# Redis Daemon
Exposes TCP server on port 6379 with wire-protocol support.
`
	if err := os.WriteFile(filepath.Join(tempDirDaemon, "SPEC.md"), []byte(daemonSpec), 0644); err != nil {
		t.Fatalf("write SPEC.md failed: %v", err)
	}

	if !IsDaemonProject(tempDirDaemon) {
		t.Errorf("expected Redis daemon to be classified as daemon project")
	}
}

func TestPortReaperWithMock(t *testing.T) {
	// Start an actual TCP listener on an ephemeral port
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("net.Listen failed: %v", err)
	}
	defer func() {
		_ = listener.Close()
	}()

	tcpAddr := listener.Addr().(*net.TCPAddr)
	port := tcpAddr.Port

	var killedPIDs []int
	var killedSignals []syscall.Signal

	reaper := &PortReaper{
		dialTimeout: 100 * 1000 * 1000, // 100ms
		killSignal: func(pid int, sig syscall.Signal) error {
			killedPIDs = append(killedPIDs, pid)
			killedSignals = append(killedSignals, sig)
			if sig == syscall.SIGKILL || sig == syscall.SIGTERM {
				_ = listener.Close()
			}
			return nil
		},
		runLsof: func(ctx context.Context, p int) ([]int, error) {
			if p == port {
				return []int{99999}, nil
			}
			return nil, nil
		},
	}

	if !reaper.IsPortInUse(port) {
		t.Errorf("expected port %d to be in use", port)
	}

	preempted, reaped, err := reaper.ReapOrphanPortHolders(context.Background(), []int{port})
	if err != nil {
		t.Fatalf("ReapOrphanPortHolders failed: %v", err)
	}

	if len(preempted) != 1 || preempted[0] != port {
		t.Errorf("expected preempted port %d, got %v", port, preempted)
	}

	if len(reaped) == 0 || reaped[0] != 99999 {
		t.Errorf("expected reaped PID 99999, got %v", reaped)
	}

	// Verify port is now free after closing listener
	if reaper.IsPortInUse(port) {
		t.Errorf("expected port %d to be free after reaping", port)
	}
}
