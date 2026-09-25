package services

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"io"
	"os/exec"
	"regexp"
	"strings"
	"sync"
	"syscall"
	"time"
)

var (
	// Test start patterns
	reGoTestStart     = regexp.MustCompile(`=== RUN\s+(\S+)`)
	reGoJSONTestStart = regexp.MustCompile(`"Action"\s*:\s*"run"\s*,\s*"Test"\s*:\s*"([^"]+)"`)
	rePytestStart     = regexp.MustCompile(`(?m)^([a-zA-Z0-9_]+)\s+\((?:[a-zA-Z0-9_.]+)\)\s+\.\.\.`)
	rePytestVerbStart = regexp.MustCompile(`(?m)^(\S+::\S+)\s*$`)
	reCargoStart      = regexp.MustCompile(`^test\s+(\S+)\s+\.\.\.`)

	// Test finish patterns
	reGoTestEnd     = regexp.MustCompile(`--- (?:PASS|FAIL|SKIP):\s+(\S+)`)
	reGoJSONTestEnd = regexp.MustCompile(`"Action"\s*:\s*"(?:pass|fail|skip)"\s*,\s*"Test"\s*:\s*"([^"]+)"`)
	rePyEnd         = regexp.MustCompile(`\.\.\.\s*(?:ok|FAIL|ERROR|skipped)`)
	rePyVerbEnd     = regexp.MustCompile(`(?:PASSED|FAILED|SKIPPED|ERROR)`)
	reCargoEnd      = regexp.MustCompile(`\.\.\.\s*(?:ok|FAILED|ignored)`)
)

// HangIsolationResult stores diagnostic metadata when a test hangs.
type HangIsolationResult struct {
	HungTest        string        `json:"hung_test"`
	Elapsed         time.Duration `json:"elapsed"`
	BlockedLocation string        `json:"blocked_location,omitempty"`
	DiagnosticDump  string        `json:"diagnostic_dump,omitempty"`
}

// ErrPerTestTimeout is returned when a specific test exceeds its per-test execution deadline.
type ErrPerTestTimeout struct {
	Result HangIsolationResult
}

func (e *ErrPerTestTimeout) Error() string {
	loc := e.Result.BlockedLocation
	if loc == "" {
		loc = "unknown location"
	}
	return fmt.Sprintf("test %q hung after %s (blocked at %s)", e.Result.HungTest, e.Result.Elapsed.Round(time.Millisecond), loc)
}

// StreamIsolatorConfig configures the streaming test watchdog.
type StreamIsolatorConfig struct {
	PerTestTimeout time.Duration
	MaxDuration    time.Duration
	IdleTimeout    time.Duration
}

// TestStreamIsolator monitors test execution output line-by-line in real time,
// detecting individual test hangs and isolating deadlocks via SIGQUIT stack dumps.
type TestStreamIsolator struct {
	cfg        StreamIsolatorConfig
	killSignal func(pid int, sig syscall.Signal) error
}

// NewTestStreamIsolator constructs a new TestStreamIsolator.
func NewTestStreamIsolator(cfg StreamIsolatorConfig) *TestStreamIsolator {
	if cfg.PerTestTimeout <= 0 {
		cfg.PerTestTimeout = 30 * time.Second
	}
	if cfg.MaxDuration <= 0 {
		cfg.MaxDuration = 5 * time.Minute
	}
	if cfg.IdleTimeout <= 0 {
		cfg.IdleTimeout = 30 * time.Second
	}
	return &TestStreamIsolator{
		cfg:        cfg,
		killSignal: syscall.Kill,
	}
}

// Run executes the command while streaming and tracking test state in real time.
func (tsi *TestStreamIsolator) Run(ctx context.Context, cmd *exec.Cmd) ([]byte, error) {
	stdoutPipe, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("open stdout pipe: %w", err)
	}
	stderrPipe, err := cmd.StderrPipe()
	if err != nil {
		return nil, fmt.Errorf("open stderr pipe: %w", err)
	}

	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}

	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("start command: %w", err)
	}

	var mu sync.Mutex
	outputBuf := NewBoundedBuffer(defaultBoundedBufferMax)

	var activeTest string
	var activeTestStart time.Time
	var lastOutputTime = time.Now()

	perTestTimer := time.NewTimer(tsi.cfg.PerTestTimeout)
	defer perTestTimer.Stop()

	maxTimer := time.NewTimer(tsi.cfg.MaxDuration)
	defer maxTimer.Stop()

	idleTimer := time.NewTimer(tsi.cfg.IdleTimeout)
	defer idleTimer.Stop()

	lineChan := make(chan string, 128)
	var wg sync.WaitGroup

	readPipe := func(r io.Reader) {
		defer wg.Done()
		scanner := bufio.NewScanner(r)
		// 1MB max line capacity for large stack frames
		buf := make([]byte, 64*1024)
		scanner.Buffer(buf, 1024*1024)
		for scanner.Scan() {
			line := scanner.Text()
			lineChan <- line
		}
	}

	wg.Add(2)
	go readPipe(stdoutPipe)
	go readPipe(stderrPipe)

	go func() {
		wg.Wait()
		close(lineChan)
	}()

	done := make(chan error, 1)
	go func() {
		done <- cmd.Wait()
	}()

	resetPerTestTimer := func() {
		if !perTestTimer.Stop() {
			select {
			case <-perTestTimer.C:
			default:
			}
		}
		perTestTimer.Reset(tsi.cfg.PerTestTimeout)
	}

	resetIdleTimer := func() {
		if !idleTimer.Stop() {
			select {
			case <-idleTimer.C:
			default:
			}
		}
		idleTimer.Reset(tsi.cfg.IdleTimeout)
	}

	for {
		select {
		case line, ok := <-lineChan:
			if !ok {
				lineChan = nil
				continue
			}
			mu.Lock()
			_, _ = outputBuf.Write([]byte(line + "\n"))
			lastOutputTime = time.Now()

			// Check test start markers
			if test := detectTestStart(line); test != "" {
				activeTest = test
				activeTestStart = time.Now()
				resetPerTestTimer()
			} else if test := detectTestEnd(line); test != "" {
				if activeTest == test || strings.HasSuffix(test, activeTest) || strings.HasSuffix(activeTest, test) {
					activeTest = ""
				}
				resetPerTestTimer()
			}
			mu.Unlock()
			resetIdleTimer()

		case <-perTestTimer.C:
			mu.Lock()
			currentTest := activeTest
			elapsed := time.Since(activeTestStart)
			mu.Unlock()

			if currentTest != "" {
				return tsi.isolateAndKill(cmd, outputBuf, currentTest, elapsed)
			}
			resetPerTestTimer()

		case <-idleTimer.C:
			mu.Lock()
			silence := time.Since(lastOutputTime)
			currentTest := activeTest
			elapsed := time.Since(activeTestStart)
			mu.Unlock()

			if silence >= tsi.cfg.IdleTimeout {
				if currentTest != "" {
					return tsi.isolateAndKill(cmd, outputBuf, currentTest, elapsed)
				}
				_ = tsi.killGroup(cmd, syscall.SIGKILL)
				return outputBuf.Bytes(), ErrWatchdogIdleTimeout
			}
			resetIdleTimer()

		case <-maxTimer.C:
			mu.Lock()
			currentTest := activeTest
			elapsed := time.Since(activeTestStart)
			mu.Unlock()

			if currentTest != "" {
				return tsi.isolateAndKill(cmd, outputBuf, currentTest, elapsed)
			}
			_ = tsi.killGroup(cmd, syscall.SIGKILL)
			return outputBuf.Bytes(), ErrWatchdogMaxDuration

		case <-ctx.Done():
			_ = tsi.killGroup(cmd, syscall.SIGKILL)
			return outputBuf.Bytes(), ctx.Err()

		case waitErr := <-done:
			_ = tsi.killGroup(cmd, syscall.SIGKILL)
			return outputBuf.Bytes(), waitErr
		}
	}
}

func (tsi *TestStreamIsolator) isolateAndKill(cmd *exec.Cmd, buf *BoundedBuffer, hungTest string, elapsed time.Duration) ([]byte, error) {
	// Step 1: Send SIGQUIT to process group to trigger runtime stack traces
	_ = tsi.killGroup(cmd, syscall.SIGQUIT)

	// Step 2: Allow brief grace period for runtime to write trace dump to pipe
	time.Sleep(250 * time.Millisecond)

	// Step 3: Send SIGKILL to terminate process group cleanly
	_ = tsi.killGroup(cmd, syscall.SIGKILL)

	captured := string(buf.Bytes())
	blockedLoc := extractBlockedLocation(captured)

	diagnostic := fmt.Sprintf("\n❌ [Per-Test Timeout & Hang Isolated]\nTest: %s\nElapsed: %s (exceeded per-test timeout of %s)\nBlocked Call: %s\nStatus: TERMINATED via SIGQUIT (stack trace dumped) and SIGKILL\n",
		hungTest, elapsed.Round(time.Millisecond), tsi.cfg.PerTestTimeout, blockedLoc)

	var fullReport bytes.Buffer
	fullReport.WriteString(diagnostic)
	fullReport.WriteString(captured)

	res := HangIsolationResult{
		HungTest:        hungTest,
		Elapsed:         elapsed,
		BlockedLocation: blockedLoc,
		DiagnosticDump:  diagnostic,
	}

	return fullReport.Bytes(), &ErrPerTestTimeout{Result: res}
}

func (tsi *TestStreamIsolator) killGroup(cmd *exec.Cmd, sig syscall.Signal) error {
	if cmd == nil || cmd.Process == nil {
		return nil
	}
	pgid, err := syscall.Getpgid(cmd.Process.Pid)
	if err != nil {
		return tsi.killSignal(cmd.Process.Pid, sig)
	}
	return tsi.killSignal(-pgid, sig)
}

func detectTestStart(line string) string {
	if m := reGoTestStart.FindStringSubmatch(line); len(m) > 1 {
		return m[1]
	}
	if m := reGoJSONTestStart.FindStringSubmatch(line); len(m) > 1 {
		return m[1]
	}
	if m := reCargoStart.FindStringSubmatch(line); len(m) > 1 {
		return m[1]
	}
	if m := rePytestStart.FindStringSubmatch(line); len(m) > 1 {
		return m[1]
	}
	if m := rePytestVerbStart.FindStringSubmatch(line); len(m) > 1 {
		return m[1]
	}
	return ""
}

func detectTestEnd(line string) string {
	if m := reGoTestEnd.FindStringSubmatch(line); len(m) > 1 {
		return m[1]
	}
	if m := reGoJSONTestEnd.FindStringSubmatch(line); len(m) > 1 {
		return m[1]
	}
	if rePyEnd.MatchString(line) || rePyVerbEnd.MatchString(line) || reCargoEnd.MatchString(line) {
		return "ended"
	}
	return ""
}

var reStackTraceLoc = regexp.MustCompile(`(?m)^\s*([a-zA-Z0-9_./-]+\.(?:go|py|rs|ts|js):\d+)`)

func extractBlockedLocation(stackDump string) string {
	matches := reStackTraceLoc.FindAllStringSubmatch(stackDump, -1)
	for i := len(matches) - 1; i >= 0; i-- {
		loc := matches[i][1]
		// Prefer user source code over runtime/stdlib internals
		if !strings.Contains(loc, "src/runtime/") && !strings.Contains(loc, "testing/testing.go") {
			return loc
		}
	}
	if len(matches) > 0 {
		return matches[len(matches)-1][1]
	}
	return "unresolved (inspect stack trace)"
}
