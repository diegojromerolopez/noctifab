package services

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

type recordingQAProcess struct {
	processes []QAProcess
	results   []QAProcessResult
	err       error
}

func (r *recordingQAProcess) Run(_ context.Context, process QAProcess) (QAProcessResult, error) {
	r.processes = append(r.processes, process)
	if len(r.results) == 0 {
		return QAProcessResult{}, r.err
	}
	result := r.results[0]
	r.results = r.results[1:]
	return result, r.err
}

func TestDockerQABuildSandboxConstrainsProjectBuild(t *testing.T) {
	workspace := t.TempDir()
	process := &recordingQAProcess{results: []QAProcessResult{
		{Stdout: "27.0", ExitCode: 0}, {ExitCode: 0}, {Stdout: "built", ExitCode: 0},
	}}
	sandbox := NewDockerQABuildSandbox(process, OSQAFileSystem{}, "builder-image", time.Minute, 12, "128m")
	result, err := sandbox.Run(context.Background(), ReviewWorkspace{Path: workspace},
		[]string{"make", "build"}, 1024)
	if err != nil || result.Stdout != "built" {
		t.Fatalf("Run() = %#v, %v", result, err)
	}
	if len(process.processes) != 3 {
		t.Fatalf("got %d process calls", len(process.processes))
	}
	probe := strings.Join(process.processes[1].Args, " ")
	build := strings.Join(process.processes[2].Args, " ")
	for _, required := range []string{"--read-only", "--network none", "--cap-drop ALL",
		"no-new-privileges", "source=" + workspace + ",target=/workspace", "sibling-write-probe",
		"host-write-probe"} {
		if !strings.Contains(probe, required) {
			t.Errorf("probe args omit %q: %s", required, probe)
		}
	}
	if strings.Contains(probe, filepath.Dir(workspace)+",target=") {
		t.Fatalf("host parent was mounted: %s", probe)
	}
	for _, required := range []string{"--entrypoint make", "builder-image build", "--tmpfs /tmp"} {
		if !strings.Contains(build, required) {
			t.Errorf("build args omit %q: %s", required, build)
		}
	}
}

func TestDockerQABuildSandboxFailsClosed(t *testing.T) {
	sandbox := NewDockerQABuildSandbox(nil, OSQAFileSystem{}, "builder", time.Minute, 1, "1m")
	if _, err := sandbox.Run(context.Background(), ReviewWorkspace{Path: t.TempDir()}, []string{"make"}, 10); err == nil {
		t.Fatal("build succeeded without Docker process dependency")
	}
	process := &recordingQAProcess{results: []QAProcessResult{{ExitCode: 1}}}
	sandbox = NewDockerQABuildSandbox(process, OSQAFileSystem{}, "builder", time.Minute, 1, "1m")
	if _, err := sandbox.Run(context.Background(), ReviewWorkspace{Path: t.TempDir()}, []string{"make"}, 10); err == nil {
		t.Fatal("build succeeded when Docker was unavailable")
	}
}

type successfulBuildSandbox struct{}

func (successfulBuildSandbox) Run(context.Context, ReviewWorkspace, []string, int) (QACommandResult, error) {
	return QACommandResult{}, nil
}

func TestArtifactBuilderRejectsExecutableSymlink(t *testing.T) {
	root := t.TempDir()
	workspace := filepath.Join(root, "workspace")
	artifact := filepath.Join(root, "artifact")
	if err := os.MkdirAll(filepath.Join(workspace, "dist"), 0o700); err != nil {
		t.Fatal(err)
	}
	hostFile := filepath.Join(root, "host-secret")
	if err := os.WriteFile(hostFile, []byte("not project output"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(hostFile, filepath.Join(workspace, "dist", "app")); err != nil {
		t.Fatal(err)
	}
	builder := NewArtifactBuildRunner(successfulBuildSandbox{}, OSQAFileSystem{})
	if _, _, err := builder.Build(context.Background(), ReviewWorkspace{Path: workspace}, "commit",
		[]string{"build"}, []string{"dist/app"}, artifact, 100); err == nil {
		t.Fatal("artifact builder copied a symlink outside the build workspace")
	}
}

func TestDockerQASandboxUsesReadOnlyInputsAndWritableRuntime(t *testing.T) {
	root := t.TempDir()
	source := filepath.Join(root, "source")
	artifact := filepath.Join(root, "artifact")
	runtimePath := filepath.Join(root, "runtime")
	siblingWorkspace := filepath.Join(root, "sibling-workspace")
	arbitraryHostPath := filepath.Join(t.TempDir(), "host-target")
	for _, path := range []string{filepath.Join(source, ".git"), artifact, runtimePath} {
		if err := os.MkdirAll(path, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	process := &recordingQAProcess{results: []QAProcessResult{{Stdout: "27.0"}, {ExitCode: 0}, {ExitCode: 0}}}
	sandbox := NewDockerQASandboxRunner(process, OSQAFileSystem{}, "qa-image", 10, "64m")
	if err := sandbox.Verify(context.Background(), source, artifact, runtimePath); err != nil {
		t.Fatal(err)
	}
	if _, err := sandbox.Run(context.Background(), QACommand{Argv: []string{"./app"}, Timeout: time.Second, OutputLimit: 20}); err != nil {
		t.Fatal(err)
	}
	baseArgs := []string{
		"run", "--rm", "--init", "--read-only", "--network", "none",
		"--pids-limit", "10", "--memory", "64m", "--cap-drop", "ALL",
		"--security-opt", "no-new-privileges", "--user", "65534:65534",
		"--workdir", "/artifacts", "--env", "HOME=/runtime/home", "--env", "TMPDIR=/runtime/tmp",
		"--env", "XDG_CACHE_HOME=/runtime/cache", "--env", "GIT_DIR=/git-metadata",
		"--mount", bindMount(source, "/source", true),
		"--mount", bindMount(filepath.Join(source, ".git"), "/git-metadata", true),
		"--mount", bindMount(artifact, "/artifacts", true),
		"--mount", bindMount(runtimePath, "/runtime", false),
	}
	probeScript := strings.Join([]string{
		"test ! -w /source",
		"test ! -w /git-metadata",
		"test ! -w /artifacts",
		"! touch /source/.noctifab-qa-write-probe 2>/dev/null",
		"! touch /git-metadata/.noctifab-qa-write-probe 2>/dev/null",
		"! touch /artifacts/.noctifab-qa-write-probe 2>/dev/null",
		"! touch /sibling-write-probe 2>/dev/null",
		"! touch /host-write-probe 2>/dev/null",
		"touch /runtime/tmp/probe /runtime/home/probe /runtime/cache/probe",
	}, " && ")
	wantProbe := append(append([]string{}, baseArgs...), "qa-image", "sh", "-c", probeScript)
	if !reflect.DeepEqual(process.processes[1].Args, wantProbe) {
		t.Fatalf("probe args:\n got: %#v\nwant: %#v", process.processes[1].Args, wantProbe)
	}
	wantRun := append(append([]string{}, baseArgs...),
		"--attach", "stdout", "--attach", "stderr", "--attach", "stdin", "qa-image", "./app")
	if !reflect.DeepEqual(process.processes[2].Args, wantRun) {
		t.Fatalf("run args:\n got: %#v\nwant: %#v", process.processes[2].Args, wantRun)
	}
	allArgs := strings.Join(process.processes[1].Args, "\x00")
	for _, forbidden := range []string{siblingWorkspace, arbitraryHostPath} {
		if strings.Contains(allArgs, forbidden) {
			t.Errorf("unapproved host path exposed to QA container: %s", forbidden)
		}
	}
	for _, path := range []string{siblingWorkspace, arbitraryHostPath} {
		if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("probe affected unmounted host path %s: %v", path, err)
		}
	}
}

type workspaceGitFake struct {
	calls              [][]string
	manifest           string
	workspaceManifests map[string]string
	failRemove         bool
}

func (g *workspaceGitFake) Run(_ context.Context, repositoryPath string, args ...string) (string, error) {
	call := append([]string{repositoryPath}, args...)
	g.calls = append(g.calls, call)
	if reflect.DeepEqual(args, []string{"rev-parse", "--verify", "source^{commit}"}) {
		return "0123456789abcdef\n", nil
	}
	if len(args) == 4 && args[0] == "ls-tree" {
		if manifest, ok := g.workspaceManifests[repositoryPath]; ok {
			return manifest, nil
		}
		return g.manifest, nil
	}
	if reflect.DeepEqual(args, []string{"rev-parse", "HEAD"}) {
		return "0123456789abcdef\n", nil
	}
	if len(args) == 3 && args[0] == "hash-object" {
		return "abc\n", nil
	}
	if len(args) >= 2 && args[0] == "worktree" && args[1] == "remove" && g.failRemove {
		return "", errors.New("remove failed")
	}
	return "", nil
}

type rootFailFileSystem struct {
	OSQAFileSystem
	failPath    string
	failed      bool
	rootRemoves int
}

func (f *rootFailFileSystem) RemoveAll(path string) error {
	if path == f.failPath {
		f.rootRemoves++
		if !f.failed {
			f.failed = true
			return errors.New("injected root cleanup failure")
		}
	}
	return os.RemoveAll(path)
}

func TestGitReviewWorkspaceFactoryRejectsExactTrackedManifestMismatch(t *testing.T) {
	repository := filepath.Join(t.TempDir(), "repository")
	if err := os.MkdirAll(repository, 0o700); err != nil {
		t.Fatal(err)
	}
	git := &workspaceGitFake{
		manifest:           "100644 blob abc\ttracked.txt\n",
		workspaceManifests: make(map[string]string),
	}
	factory := NewGitReviewWorkspaceFactory(git, OSQAFileSystem{}, qaClockFake{now: time.Unix(50, 0)})
	expectedRoot := filepath.Join(filepath.Dir(repository),
		".noctifab-review-repository-0123456789ab-19700101T000050.000000000")
	git.workspaceManifests[filepath.Join(expectedRoot, "build")] = "100644 blob def\ttracked.txt\n"

	_, _, _, err := factory.Create(context.Background(), repository, "source")
	if err == nil {
		t.Fatal("workspace creation accepted a tracked manifest mismatch")
	}
	want := "review workspace: create build: verify tracked manifest: workspace differs from source commit"
	if err.Error() != want {
		t.Fatalf("Create() error = %q, want %q", err, want)
	}
	if _, statErr := os.Stat(expectedRoot); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatalf("failed creation did not clean the entire review root: %v", statErr)
	}
}

func TestGitReviewWorkspaceFactoryVerifiesManifestAndCleansWholeRoot(t *testing.T) {
	repository := filepath.Join(t.TempDir(), "repository")
	if err := os.MkdirAll(repository, 0o700); err != nil {
		t.Fatal(err)
	}
	git := &workspaceGitFake{manifest: "100644 blob abc\ttracked.txt\n"}
	fsys := &rootFailFileSystem{}
	factory := NewGitReviewWorkspaceFactory(git, fsys, qaClockFake{now: time.Unix(100, 0)})
	build, tester, qa, err := factory.Create(context.Background(), repository, "source")
	if err != nil {
		t.Fatal(err)
	}
	root := filepath.Dir(build.Path)
	for _, workspace := range []ReviewWorkspace{build, tester, qa} {
		if nestedPath(repository, workspace.Path) || nestedPath(workspace.Path, repository) {
			t.Fatalf("workspace is nested with original: %s", workspace.Path)
		}
	}
	manifestCalls := 0
	for _, call := range git.calls {
		if len(call) > 2 && call[1] == "ls-tree" {
			manifestCalls++
		}
	}
	if manifestCalls != 4 {
		t.Fatalf("got %d ls-tree manifest calls, want source plus three worktrees", manifestCalls)
	}
	hashCalls := 0
	for _, call := range git.calls {
		if len(call) > 2 && call[1] == "hash-object" {
			hashCalls++
		}
	}
	if hashCalls != 3 {
		t.Fatalf("got %d hash-object calls, want one tracked-file hash per worktree", hashCalls)
	}
	for _, path := range []string{filepath.Join(root, "runtime-artifact", "app"), filepath.Join(root, "qa-runtime", "tmp", "data")} {
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte("x"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	fsys.failPath = root
	if err := factory.Cleanup(context.Background(), build, tester, qa); err == nil {
		t.Fatal("cleanup unexpectedly succeeded")
	}
	if _, err := os.Stat(root); err != nil {
		t.Fatalf("review root disappeared after failed cleanup: %v", err)
	}
	for _, path := range []string{filepath.Join(root, "runtime-artifact", "app"), filepath.Join(root, "qa-runtime", "tmp", "data")} {
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("root cleanup failure partially removed runtime content %s: %v", path, err)
		}
	}
	if err := factory.Cleanup(context.Background(), build, tester, qa); err != nil {
		t.Fatalf("cleanup retry failed: %v", err)
	}
	if fsys.rootRemoves != 2 {
		t.Fatalf("review root removal attempts = %d, want 2", fsys.rootRemoves)
	}
	if _, err := os.Stat(root); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("review root remains after cleanup: %v", err)
	}
}

func TestGitReviewWorkspaceCleanupRetriesWorktreeRemoval(t *testing.T) {
	repository := filepath.Join(t.TempDir(), "repository")
	if err := os.MkdirAll(repository, 0o700); err != nil {
		t.Fatal(err)
	}
	git := &workspaceGitFake{manifest: "", failRemove: true}
	factory := NewGitReviewWorkspaceFactory(git, OSQAFileSystem{}, qaClockFake{now: time.Unix(200, 0)})
	build, tester, qa, err := factory.Create(context.Background(), repository, "source")
	if err != nil {
		t.Fatal(err)
	}
	root := filepath.Dir(build.Path)
	for _, workspace := range []ReviewWorkspace{build, tester, qa} {
		if err := os.MkdirAll(workspace.Path, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	if err := factory.Cleanup(context.Background(), build, tester, qa); err == nil {
		t.Fatal("cleanup succeeded despite worktree removal failures")
	}
	if _, err := os.Stat(build.Path); err != nil {
		t.Fatalf("workspace path removed before Git cleanup succeeded: %v", err)
	}
	git.failRemove = false
	if err := factory.Cleanup(context.Background(), build, tester, qa); err != nil {
		t.Fatalf("cleanup retry failed: %v", err)
	}
	if _, err := os.Stat(root); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("review root remains after retry: %v", err)
	}
}
