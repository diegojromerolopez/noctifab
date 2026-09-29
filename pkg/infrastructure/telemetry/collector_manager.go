package telemetry

import (
	"context"
	"fmt"
	"net"
	"os/exec"
	"strings"
	"time"
)

const (
	// DefaultCollectorEndpoint is the default OTLP HTTP ingestion endpoint.
	DefaultCollectorEndpoint = "localhost:4318"
	// DefaultJaegerContainer is the default name for the auto-provisioned container.
	DefaultJaegerContainer = "noctifab-jaeger"
	// DefaultJaegerImage is the all-in-one image containing the OTel collector and UI.
	DefaultJaegerImage = "jaegertracing/all-in-one:latest"
)

// IsPortReachable tests TCP connectivity to endpoint with a short timeout.
func IsPortReachable(endpoint string, timeout time.Duration) bool {
	addr := endpoint
	addr = strings.TrimPrefix(addr, "http://")
	addr = strings.TrimPrefix(addr, "https://")
	if !strings.Contains(addr, ":") {
		addr = addr + ":4318"
	}
	conn, err := net.DialTimeout("tcp", addr, timeout)
	if err != nil {
		return false
	}
	_ = conn.Close()
	return true
}

// EnsureCollectorOnline checks if an OpenTelemetry collector is online.
// If offline and Docker is installed, it attempts to start a container
// with the OpenTelemetry collector (Jaeger all-in-one) so spans can be viewed.
func EnsureCollectorOnline(ctx context.Context, endpoint string) (string, error) {
	if endpoint == "" {
		endpoint = DefaultCollectorEndpoint
	}

	// 1. Check if collector is already reachable
	if IsPortReachable(endpoint, 500*time.Millisecond) {
		return endpoint, nil
	}

	// 2. Check if Docker is available
	dockerPath, err := exec.LookPath("docker")
	if err != nil {
		return "", fmt.Errorf("docker not found in PATH: %w", err)
	}

	// 3. Check if noctifab-jaeger container exists
	inspectCmd := exec.CommandContext(ctx, dockerPath, "inspect", "--format", "{{.State.Running}}", DefaultJaegerContainer)
	out, err := inspectCmd.Output()
	if err == nil {
		running := strings.TrimSpace(string(out))
		if running == "true" {
			if waitForReachable(endpoint, 2*time.Second) {
				return endpoint, nil
			}
		} else {
			startCmd := exec.CommandContext(ctx, dockerPath, "start", DefaultJaegerContainer)
			if sErr := startCmd.Run(); sErr == nil {
				if waitForReachable(endpoint, 3*time.Second) {
					fmt.Printf("🚀 [Telemetry] Started existing OpenTelemetry collector container %q (Web UI: http://localhost:16686)\n", DefaultJaegerContainer)
					return endpoint, nil
				}
			}
		}
	}

	// 4. Container doesn't exist, launch jaegertracing/all-in-one
	runCmd := exec.CommandContext(ctx, dockerPath, "run", "-d",
		"--name", DefaultJaegerContainer,
		"-p", "16686:16686",
		"-p", "4318:4318",
		DefaultJaegerImage,
	)
	if err := runCmd.Run(); err != nil {
		return "", fmt.Errorf("failed to run docker container %s: %w", DefaultJaegerContainer, err)
	}

	if waitForReachable(endpoint, 4*time.Second) {
		fmt.Printf("🚀 [Telemetry] OpenTelemetry Collector (Jaeger) launched in Docker container %q (Web UI: http://localhost:16686, OTLP: %s)\n", DefaultJaegerContainer, endpoint)
		return endpoint, nil
	}

	return endpoint, nil
}

func waitForReachable(endpoint string, maxWait time.Duration) bool {
	deadline := time.Now().Add(maxWait)
	for time.Now().Before(deadline) {
		if IsPortReachable(endpoint, 200*time.Millisecond) {
			return true
		}
		time.Sleep(150 * time.Millisecond)
	}
	return false
}
