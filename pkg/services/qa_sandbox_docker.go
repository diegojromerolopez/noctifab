package services

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	defaultQADockerPIDs        = 64
	defaultQADockerMemory      = "512m"
	qaDockerControlOutputLimit = 32 * 1024
)

// DockerQASandboxRunner enforces QA isolation with a fresh container per command.
type DockerQASandboxRunner struct {
	process      QAProcessRunner
	fs           QAFileSystem
	image        string
	maxProcesses int
	memoryLimit  string

	mu       sync.RWMutex
	verified bool
	source   string
	artifact string
	runtime  string
	git      string
}

var _ QASandboxRunner = (*DockerQASandboxRunner)(nil)

func NewDockerQASandboxRunner(
	process QAProcessRunner,
	fsys QAFileSystem,
	image string,
	maxProcesses int,
	memoryLimit string,
) *DockerQASandboxRunner {
	if maxProcesses <= 0 {
		maxProcesses = defaultQADockerPIDs
	}
	if memoryLimit == "" {
		memoryLimit = defaultQADockerMemory
	}
	return &DockerQASandboxRunner{
		process: process, fs: fsys, image: image,
		maxProcesses: maxProcesses, memoryLimit: memoryLimit,
	}
}

func (r *DockerQASandboxRunner) Verify(ctx context.Context, sourcePath, artifactPath, runtimePath string) error {
	r.mu.Lock()
	r.verified = false
	r.source, r.artifact, r.runtime, r.git = "", "", "", ""
	r.mu.Unlock()
	if r.process == nil || r.fs == nil || strings.TrimSpace(r.image) == "" {
		return errors.New("qa sandbox verify: missing dependency or image")
	}
	paths, err := r.resolvePaths(sourcePath, artifactPath, runtimePath)
	if err != nil {
		return err
	}
	gitPath, err := r.resolveGitMetadata(paths[0])
	if err != nil {
		return err
	}
	for _, directory := range []string{"tmp", "home", "cache"} {
		if err := r.fs.MkdirAll(filepath.Join(paths[2], directory), 0o700); err != nil {
			return fmt.Errorf("qa sandbox verify: create runtime %s: %w", directory, err)
		}
	}
	version, err := r.process.Run(ctx, QAProcess{
		Name: "docker", Args: []string{"version", "--format", "{{.Server.Version}}"},
		Timeout: 15 * time.Second, OutputLimit: qaDockerControlOutputLimit,
	})
	if err != nil || version.ExitCode != 0 || strings.TrimSpace(version.Stdout) == "" {
		return fmt.Errorf("qa sandbox verify: Docker unavailable: %w", err)
	}
	probeArgs := r.dockerArgs(paths[0], paths[1], paths[2], gitPath)
	probeArgs = append(probeArgs, r.image, "sh", "-c", strings.Join([]string{
		"test ! -w /source",
		"test ! -w /git-metadata",
		"test ! -w /artifacts",
		"! touch /source/.noctifab-qa-write-probe 2>/dev/null",
		"! touch /git-metadata/.noctifab-qa-write-probe 2>/dev/null",
		"! touch /artifacts/.noctifab-qa-write-probe 2>/dev/null",
		"! touch /sibling-write-probe 2>/dev/null",
		"! touch /host-write-probe 2>/dev/null",
		"touch /runtime/tmp/probe /runtime/home/probe /runtime/cache/probe",
	}, " && "))
	probe, err := r.process.Run(ctx, QAProcess{
		Name: "docker", Args: probeArgs, Timeout: 30 * time.Second,
		OutputLimit: qaDockerControlOutputLimit,
	})
	if err != nil || probe.ExitCode != 0 || probe.TimedOut || probe.Truncated {
		return fmt.Errorf("qa sandbox verify: read-only isolation probe failed: %w", err)
	}
	r.mu.Lock()
	r.source, r.artifact, r.runtime, r.git = paths[0], paths[1], paths[2], gitPath
	r.verified = true
	r.mu.Unlock()
	return nil
}

func (r *DockerQASandboxRunner) Run(ctx context.Context, command QACommand) (QACommandResult, error) {
	if len(command.Argv) == 0 || strings.TrimSpace(command.Argv[0]) == "" {
		return QACommandResult{}, errors.New("qa sandbox: empty command")
	}
	if command.Timeout <= 0 || command.OutputLimit <= 0 {
		return QACommandResult{}, errors.New("qa sandbox: positive timeout and output limit required")
	}
	r.mu.RLock()
	verified := r.verified
	source, artifact, runtimePath, gitPath := r.source, r.artifact, r.runtime, r.git
	r.mu.RUnlock()
	if !verified {
		return QACommandResult{}, errors.New("qa sandbox: Verify must succeed before Run")
	}
	args := r.dockerArgs(source, artifact, runtimePath, gitPath)
	args = append(args, "--attach", "stdout", "--attach", "stderr", "--attach", "stdin", r.image)
	args = append(args, command.Argv...)
	processResult, err := r.process.Run(ctx, QAProcess{
		Name: "docker", Args: args, Stdin: command.Stdin,
		Timeout: command.Timeout, OutputLimit: command.OutputLimit,
	})
	result := commandResult(processResult)
	if err != nil {
		return result, fmt.Errorf("qa sandbox: Docker execution failed: %w", err)
	}
	if processResult.ExitCode == 125 || processResult.ExitCode == 126 || processResult.ExitCode == 127 {
		return result, fmt.Errorf("qa sandbox: container environment failed with status %d", processResult.ExitCode)
	}
	return result, nil
}

func (r *DockerQASandboxRunner) dockerArgs(source, artifact, runtimePath, gitPath string) []string {
	return []string{
		"run", "--rm", "--init", "--read-only", "--network", "none",
		"--pids-limit", strconv.Itoa(r.maxProcesses), "--memory", r.memoryLimit,
		"--cap-drop", "ALL", "--security-opt", "no-new-privileges",
		"--user", "65534:65534", "--workdir", "/artifacts",
		"--env", "HOME=/runtime/home", "--env", "TMPDIR=/runtime/tmp",
		"--env", "XDG_CACHE_HOME=/runtime/cache", "--env", "GIT_DIR=/git-metadata",
		"--mount", bindMount(source, "/source", true),
		"--mount", bindMount(gitPath, "/git-metadata", true),
		"--mount", bindMount(artifact, "/artifacts", true),
		"--mount", bindMount(runtimePath, "/runtime", false),
	}
}

func (r *DockerQASandboxRunner) resolvePaths(values ...string) ([]string, error) {
	paths := make([]string, len(values))
	for index, value := range values {
		path, err := r.fs.Abs(value)
		if err != nil || strings.TrimSpace(value) == "" {
			return nil, fmt.Errorf("qa sandbox verify: invalid path %q: %w", value, err)
		}
		paths[index] = filepath.Clean(path)
	}
	for i := range paths {
		for j := i + 1; j < len(paths); j++ {
			if nestedPath(paths[i], paths[j]) || nestedPath(paths[j], paths[i]) {
				return nil, errors.New("qa sandbox verify: source, artifact, and runtime paths must be distinct and non-nested")
			}
		}
	}
	return paths, nil
}

func (r *DockerQASandboxRunner) resolveGitMetadata(source string) (string, error) {
	gitPath := filepath.Join(source, ".git")
	info, err := r.fs.Stat(gitPath)
	if err != nil {
		return "", fmt.Errorf("qa sandbox verify: Git metadata unavailable: %w", err)
	}
	if info.IsDir() {
		return gitPath, nil
	}
	data, err := r.fs.ReadFile(gitPath)
	if err != nil {
		return "", fmt.Errorf("qa sandbox verify: read Git metadata link: %w", err)
	}
	value := strings.TrimSpace(string(data))
	if !strings.HasPrefix(value, "gitdir:") {
		return "", errors.New("qa sandbox verify: unsupported Git metadata link")
	}
	value = strings.TrimSpace(strings.TrimPrefix(value, "gitdir:"))
	if !filepath.IsAbs(value) {
		value = filepath.Join(source, value)
	}
	value = filepath.Clean(value)
	if _, err := r.fs.Stat(value); err != nil {
		return "", fmt.Errorf("qa sandbox verify: linked Git metadata unavailable: %w", err)
	}
	return value, nil
}

func bindMount(source, target string, readOnly bool) string {
	value := "type=bind,source=" + source + ",target=" + target
	if readOnly {
		value += ",readonly"
	}
	return value
}

func nestedPath(parent, child string) bool {
	relative, err := filepath.Rel(parent, child)
	return err == nil && relative != "." && relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator))
}

var _ fs.FileInfo
