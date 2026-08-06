package services

import (
	"context"
	"errors"
	"os/exec"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/diegojromerolopez/noctifab/pkg/infrastructure/telemetry"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

var (
	ErrWatchdogMaxDuration = errors.New("command killed: max wall-clock duration exceeded")
	ErrWatchdogIdleTimeout = errors.New("command killed: no output produced within idle timeout")
)

type Watchdog struct {
	MaxDuration time.Duration
	IdleTimeout time.Duration
}

// outputCapturer records command output while tracking the last write time
// for idle detection. Output is capped at 1MB, preserving the tail (recent
// output matters most for test logs).
type outputCapturer struct {
	buf        *BoundedBuffer
	lastOutput int64
}

func newOutputCapturer() *outputCapturer {
	return &outputCapturer{buf: NewBoundedBuffer(defaultBoundedBufferMax)}
}

func (c *outputCapturer) Write(p []byte) (int, error) {
	n, err := c.buf.Write(p)
	if n > 0 {
		atomic.StoreInt64(&c.lastOutput, time.Now().UnixNano())
	}
	return n, err
}

func (c *outputCapturer) Output() []byte {
	return c.buf.Bytes()
}

func (c *outputCapturer) SinceLastOutput() time.Duration {
	nano := atomic.LoadInt64(&c.lastOutput)
	return time.Since(time.Unix(0, nano))
}

func (w *Watchdog) Run(ctx context.Context, cmd *exec.Cmd) ([]byte, error) {
	ctx, span := telemetry.Tracer().Start(ctx, "Run",
		trace.WithAttributes(
			attribute.String("max_duration", w.MaxDuration.String()),
			attribute.String("idle_timeout", w.IdleTimeout.String()),
		))
	defer span.End()

	capturer := newOutputCapturer()
	cmd.Stdout = capturer
	cmd.Stderr = capturer
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}

	if err := cmd.Start(); err != nil {
		return nil, err
	}

	atomic.StoreInt64(&capturer.lastOutput, time.Now().UnixNano())

	done := make(chan error, 1)
	go func() {
		done <- cmd.Wait()
	}()

	maxDuration := w.MaxDuration
	if maxDuration <= 0 {
		maxDuration = 5 * time.Minute
	}
	maxTimer := time.NewTimer(maxDuration)
	defer maxTimer.Stop()

	var idleTimer *time.Timer
	var idleCh <-chan time.Time
	if w.IdleTimeout > 0 {
		idleTimer = time.NewTimer(w.IdleTimeout)
		idleCh = idleTimer.C
	}

	for {
		select {
		case err := <-done:
			return capturer.Output(), err

		case <-maxTimer.C:
			_ = killProcessGroup(cmd)
			return capturer.Output(), ErrWatchdogMaxDuration

		case <-idleCh:
			if capturer.SinceLastOutput() >= w.IdleTimeout {
				_ = killProcessGroup(cmd)
				return capturer.Output(), ErrWatchdogIdleTimeout
			}
			remaining := w.IdleTimeout - capturer.SinceLastOutput()
			if remaining < 0 {
				remaining = 0
			}
			idleTimer.Reset(remaining)

		case <-ctx.Done():
			_ = killProcessGroup(cmd)
			return capturer.Output(), ctx.Err()
		}
	}
}

func killProcessGroup(cmd *exec.Cmd) error {
	if cmd == nil || cmd.Process == nil {
		return nil
	}
	pgid, err := syscall.Getpgid(cmd.Process.Pid)
	if err != nil {
		return err
	}
	return syscall.Kill(-pgid, syscall.SIGKILL)
}
