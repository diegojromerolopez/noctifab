package services

import (
	"context"
	"errors"
	"fmt"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/diegojromerolopez/noctifab/pkg/infrastructure/telemetry"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

// defaultDockerMaxDuration bounds each docker exec invocation when the
// sandbox has no explicit MaxDuration configured.
const defaultDockerMaxDuration = 5 * time.Minute

// DockerSandbox routes execution inside a warm Docker container.
// Each command is bounded by MaxDuration (default 5 minutes): the host-side
// docker exec client is cancelled via context, and the in-container process
// is wrapped with `timeout <secs>` so it does not keep running inside the
// container after the host client dies.
type DockerSandbox struct {
	ContainerName string
	MaxDuration   time.Duration
}

var _ Sandbox = (*DockerSandbox)(nil)

// DockerSandboxOption customizes a DockerSandbox created via NewDockerSandbox.
type DockerSandboxOption func(*DockerSandbox)

// WithDockerMaxDuration overrides the per-command max duration. Non-positive
// values keep the 5-minute default.
func WithDockerMaxDuration(d time.Duration) DockerSandboxOption {
	return func(s *DockerSandbox) {
		s.MaxDuration = d
	}
}

func NewDockerSandbox(containerName string, opts ...DockerSandboxOption) *DockerSandbox {
	s := &DockerSandbox{
		ContainerName: containerName,
	}
	for _, opt := range opts {
		opt(s)
	}
	return s
}

// buildDockerExecArgs constructs the argument slice for `docker <args...>`.
// When timeoutSecs > 0 the in-container command is prefixed with
// `timeout <secs>` (busybox/coreutils) so the process is killed inside the
// container when the deadline expires, not just the host-side client.
func buildDockerExecArgs(containerName, pkg, cmdStr string, timeoutSecs int) []string {
	args := []string{"exec", "-w", "/app/" + pkg, containerName}
	if timeoutSecs > 0 {
		args = append(args, "timeout", strconv.Itoa(timeoutSecs))
	}
	if needsShell(cmdStr) {
		// Run through sh -c inside the container so operators work.
		args = append(args, "sh", "-c", cmdStr)
	} else {
		args = append(args, strings.Fields(cmdStr)...)
	}
	return args
}

func (s *DockerSandbox) RunCommand(ctx context.Context, projectPath string, command string, pkg string) (string, error) {
	ctx, span := telemetry.Tracer().Start(ctx, "RunCommand",
		trace.WithAttributes(
			telemetry.Attr("command", command),
			telemetry.Attr("project_path", projectPath),
			telemetry.Attr("package", pkg),
			attribute.String("container_name", s.ContainerName),
		))
	defer span.End()
	cmdStr := command
	if cmdStr == "" {
		if _, err := os.Stat(filepath.Join(projectPath, "go.mod")); err == nil {
			cmdStr = "go test -v ./..."
		} else {
			cmdStr = "python -m unittest discover tests"
		}
	}

	if len(strings.Fields(cmdStr)) == 0 {
		return "", errors.New("empty command")
	}

	maxDuration := s.MaxDuration
	if maxDuration <= 0 {
		maxDuration = defaultDockerMaxDuration
	}
	ctx, cancel := context.WithTimeout(ctx, maxDuration)
	defer cancel()

	timeoutSecs := 0
	if deadline, ok := ctx.Deadline(); ok {
		timeoutSecs = int(math.Ceil(time.Until(deadline).Seconds()))
		if timeoutSecs < 1 {
			timeoutSecs = 1
		}
	}

	args := buildDockerExecArgs(s.ContainerName, pkg, cmdStr, timeoutSecs)
	cmd := exec.CommandContext(ctx, "docker", args...)

	stdoutStderr := NewBoundedBuffer(defaultBoundedBufferMax)
	cmd.Stdout = stdoutStderr
	cmd.Stderr = stdoutStderr

	err := cmd.Run()
	output := stdoutStderr.String()
	if err != nil {
		return output, fmt.Errorf("docker exec command failed: %w (output: %s)", err, output)
	}
	return output, nil
}

// BuildCacheVolumeArgs returns docker -v volume flags for mounting persistent host build caches
func BuildCacheVolumeArgs() []string {
	var cacheArgs []string
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return cacheArgs
	}

	// Go build & module caches
	goModDir := filepath.Join(home, "go", "pkg", "mod")
	goBuildDir := filepath.Join(home, ".cache", "go-build")
	_ = os.MkdirAll(goModDir, 0755)
	_ = os.MkdirAll(goBuildDir, 0755)
	cacheArgs = append(cacheArgs, "-v", fmt.Sprintf("%s:/go/pkg/mod", goModDir))
	cacheArgs = append(cacheArgs, "-v", fmt.Sprintf("%s:/root/.cache/go-build", goBuildDir))

	// Cargo cache for Rust
	cargoReg := filepath.Join(home, ".cargo", "registry")
	cargoGit := filepath.Join(home, ".cargo", "git")
	_ = os.MkdirAll(cargoReg, 0755)
	_ = os.MkdirAll(cargoGit, 0755)
	cacheArgs = append(cacheArgs, "-v", fmt.Sprintf("%s:/usr/local/cargo/registry", cargoReg))
	cacheArgs = append(cacheArgs, "-v", fmt.Sprintf("%s:/usr/local/cargo/git", cargoGit))

	// NPM cache for Node/TypeScript
	npmCache := filepath.Join(home, ".npm")
	_ = os.MkdirAll(npmCache, 0755)
	cacheArgs = append(cacheArgs, "-v", fmt.Sprintf("%s:/root/.npm", npmCache))

	return cacheArgs
}
