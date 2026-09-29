package services

import (
	"bytes"
	"context"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"syscall"
	"time"
)

var (
	portFlagRE        = regexp.MustCompile(`(?i)(?:--port|-p)\s+([0-9]{2,5})\b`)
	portAssignRE      = regexp.MustCompile(`(?i)\b(?:port|PORT)\s*[:=]\s*"?([0-9]{2,5})"?\b`)
	portKeywordRE     = regexp.MustCompile(`(?i)\b(?:port|PORT)\s+([0-9]{2,5})\b`)
	localhostPortRE   = regexp.MustCompile(`(?:localhost|127\.0\.0\.1):([0-9]{2,5})\b`)
	dockerPortMapRE   = regexp.MustCompile(`(?:^|\n)\s*-\s*"?([0-9]{2,5}):([0-9]{2,5})"?`)
	nonPortYearStatus = map[int]bool{
		200: true, 201: true, 204: true, 301: true, 302: true, 304: true,
		400: true, 401: true, 403: true, 404: true, 409: true, 422: true, 500: true, 502: true, 503: true,
		2020: true, 2021: true, 2022: true, 2023: true, 2024: true, 2025: true, 2026: true,
	}
)

// DetectProjectPorts inspects specification, compose files, and stories in projectPath to discover TCP ports.
func DetectProjectPorts(projectPath string) ([]int, error) {
	if projectPath == "" {
		return nil, nil
	}

	foundMap := make(map[int]bool)

	// Scan candidate files
	candidates := []string{
		"SPEC.md", "SPECIFICATION.md",
		"docker-compose.yml", "docker-compose.yaml",
		"docker-compose.e2e.yml", "docker-compose.e2e.yaml",
		filepath.Join(".noctifab", "config.yaml"),
	}

	for _, cand := range candidates {
		full := filepath.Join(projectPath, cand)
		content, err := os.ReadFile(full)
		if err != nil {
			continue
		}
		extractPortsFromText(string(content), foundMap)
	}

	// Scan roadmap stories if present
	roadmapDir := filepath.Join(projectPath, "roadmap")
	if entries, err := os.ReadDir(roadmapDir); err == nil {
		for _, e := range entries {
			if !e.IsDir() && strings.HasPrefix(e.Name(), "US-") && strings.HasSuffix(e.Name(), ".md") {
				content, err := os.ReadFile(filepath.Join(roadmapDir, e.Name()))
				if err == nil {
					extractPortsFromText(string(content), foundMap)
				}
			}
		}
	}

	if len(foundMap) == 0 {
		return nil, nil
	}

	var ports []int
	for p := range foundMap {
		ports = append(ports, p)
	}
	sort.Ints(ports)
	return ports, nil
}

func extractPortsFromText(text string, foundMap map[int]bool) {
	addPort := func(pStr string) {
		p, err := strconv.Atoi(pStr)
		if err != nil {
			return
		}
		// Valid user-space port check and filter non-port numbers (HTTP statuses, years)
		if p >= 80 && p <= 65535 && !nonPortYearStatus[p] {
			foundMap[p] = true
		}
	}

	for _, m := range portFlagRE.FindAllStringSubmatch(text, -1) {
		if len(m) > 1 {
			addPort(m[1])
		}
	}
	for _, m := range portAssignRE.FindAllStringSubmatch(text, -1) {
		if len(m) > 1 {
			addPort(m[1])
		}
	}
	for _, m := range portKeywordRE.FindAllStringSubmatch(text, -1) {
		if len(m) > 1 {
			addPort(m[1])
		}
	}
	for _, m := range localhostPortRE.FindAllStringSubmatch(text, -1) {
		if len(m) > 1 {
			addPort(m[1])
		}
	}
	for _, m := range dockerPortMapRE.FindAllStringSubmatch(text, -1) {
		if len(m) > 1 {
			addPort(m[1])
		}
		if len(m) > 2 {
			addPort(m[2])
		}
	}
}

// IsDaemonProject returns true if the project specifies server/daemon ports or TCP wire protocol semantics.
func IsDaemonProject(projectPath string) bool {
	ports, _ := DetectProjectPorts(projectPath)
	if len(ports) > 0 {
		return true
	}

	// Secondary check: look for server/daemon keywords in specification
	specFiles := []string{"SPEC.md", "SPECIFICATION.md"}
	for _, sf := range specFiles {
		content, err := os.ReadFile(filepath.Join(projectPath, sf))
		if err != nil {
			continue
		}
		lower := strings.ToLower(string(content))
		if strings.Contains(lower, "wire-protocol") || strings.Contains(lower, "listen tcp") ||
			(strings.Contains(lower, "daemon") && strings.Contains(lower, "socket")) {
			return true
		}
	}

	return false
}

// PortReaper manages detection and pre-emption of orphan processes holding ports.
type PortReaper struct {
	dialTimeout time.Duration
	killSignal  func(pid int, sig syscall.Signal) error
	runLsof     func(ctx context.Context, port int) ([]int, error)
}

// NewPortReaper creates an operational PortReaper.
func NewPortReaper() *PortReaper {
	return &PortReaper{
		dialTimeout: 150 * time.Millisecond,
		killSignal:  syscall.Kill,
		runLsof:     findPIDsViaLsof,
	}
}

func findPIDsViaLsof(ctx context.Context, port int) ([]int, error) {
	cmd := exec.CommandContext(ctx, "lsof", "-ti", fmt.Sprintf(":%d", port), "-sTCP:LISTEN")
	var stdout bytes.Buffer
	cmd.Stdout = &stdout
	if err := cmd.Run(); err != nil {
		// Fallback without -sTCP:LISTEN
		cmdFallback := exec.CommandContext(ctx, "lsof", "-ti", fmt.Sprintf(":%d", port))
		cmdFallback.Stdout = &stdout
		if err2 := cmdFallback.Run(); err2 != nil {
			return nil, nil
		}
	}

	lines := strings.Split(stdout.String(), "\n")
	var pids []int
	selfPID := os.Getpid()

	for _, l := range lines {
		trimmed := strings.TrimSpace(l)
		if trimmed == "" {
			continue
		}
		pid, err := strconv.Atoi(trimmed)
		if err == nil && pid > 1 && pid != selfPID {
			pids = append(pids, pid)
		}
	}
	return pids, nil
}

// IsPortInUse tests if a local port has an active TCP listener.
func (r *PortReaper) IsPortInUse(port int) bool {
	addr := fmt.Sprintf("127.0.0.1:%d", port)
	conn, err := net.DialTimeout("tcp", addr, r.dialTimeout)
	if err == nil {
		_ = conn.Close()
		return true
	}
	return false
}

// ReapOrphanPortHolders checks the specified ports, discovers any orphan listening processes,
// and terminates them gracefully (SIGTERM) then forcefully (SIGKILL).
func (r *PortReaper) ReapOrphanPortHolders(ctx context.Context, ports []int) ([]int, []int, error) {
	var preemptedPorts []int
	var reapedPIDs []int

	for _, port := range ports {
		if !r.IsPortInUse(port) {
			continue
		}

		pids, err := r.runLsof(ctx, port)
		if err != nil || len(pids) == 0 {
			// If dial connected but lsof found no accessible PIDs, attempt graceful sleep or return error
			continue
		}

		preemptedPorts = append(preemptedPorts, port)

		for _, pid := range pids {
			// Send SIGTERM
			_ = r.killSignal(pid, syscall.SIGTERM)
			reapedPIDs = append(reapedPIDs, pid)
		}

		// Wait briefly for processes to exit
		time.Sleep(200 * time.Millisecond)

		// Check if any PID remains alive and force SIGKILL
		for _, pid := range pids {
			if r.killSignal(pid, 0) == nil { // Process still alive
				_ = r.killSignal(pid, syscall.SIGKILL)
			}
		}

		// Final check: verify port is free
		time.Sleep(100 * time.Millisecond)
		if r.IsPortInUse(port) {
			return preemptedPorts, reapedPIDs, fmt.Errorf("port %d remains in use after SIGKILL to PID(s) %v", port, pids)
		}
	}

	return preemptedPorts, reapedPIDs, nil
}
