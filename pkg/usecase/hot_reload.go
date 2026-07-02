package usecase

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"time"

	"github.com/diegojromerolopez/noctifab/pkg/infrastructure/telemetry"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

type HandoffStatus string

const (
	HandoffPending HandoffStatus = "pending"
	HandoffHanding HandoffStatus = "handing_off"
	HandoffActive  HandoffStatus = "active"
	HandoffFailed  HandoffStatus = "failed"
)

type HandoffState struct {
	NewPID  int           `json:"new_pid"`
	Status  HandoffStatus `json:"status"`
	Message string        `json:"message,omitempty"`
}

type HotReloadManager struct {
	PIDPath     string
	HandoffPath string
	NewBinary   string
	Workspace   string
}

func (hrm *HotReloadManager) Reload(ctx context.Context) error {
	ctx, span := telemetry.Tracer().Start(ctx, "noctifab.hot_reload",
		trace.WithAttributes(
			attribute.String("new_binary", hrm.NewBinary),
			attribute.String("handoff_path", hrm.HandoffPath),
		))
	defer span.End()

	cmd := exec.CommandContext(ctx, hrm.NewBinary, "serve", "--port", "18081")
	cmd.Dir = hrm.Workspace
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("hot-reload: failed to start new binary: %w", err)
	}

	newPID := cmd.Process.Pid

	handoff := HandoffState{NewPID: newPID, Status: HandoffHanding}
	hrm.writeHandoff(handoff)

	if err := hrm.waitForHealth(ctx, "http://127.0.0.1:18081/healthz", 30*time.Second); err != nil {
		handoff.Status = HandoffFailed
		handoff.Message = err.Error()
		hrm.writeHandoff(handoff)
		_ = cmd.Process.Kill()
		return fmt.Errorf("hot-reload: new binary health check failed: %w", err)
	}

	if err := hrm.waitForActive(ctx, 10*time.Second); err != nil {
		handoff.Status = HandoffFailed
		handoff.Message = err.Error()
		hrm.writeHandoff(handoff)
		_ = cmd.Process.Kill()
		return fmt.Errorf("hot-reload: handoff confirmation failed: %w", err)
	}

	fmt.Fprintf(os.Stderr, "Hot-reload complete. New PID: %d. Exiting.\n", newPID)
	return nil
}

func (hrm *HotReloadManager) waitForHealth(ctx context.Context, url string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		resp, err := http.Get(url)
		if err == nil && resp.StatusCode == http.StatusOK {
			resp.Body.Close()
			return nil
		}
		if resp != nil {
			resp.Body.Close()
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(500 * time.Millisecond):
		}
	}
	return fmt.Errorf("health check did not pass within %s", timeout)
}

func (hrm *HotReloadManager) waitForActive(ctx context.Context, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		handoff, err := hrm.readHandoff()
		if err == nil && handoff.Status == HandoffActive {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(200 * time.Millisecond):
		}
	}
	return fmt.Errorf("handoff did not reach 'active' within %s", timeout)
}

func (hrm *HotReloadManager) writeHandoff(state HandoffState) {
	data, _ := json.Marshal(state)
	_ = os.WriteFile(hrm.HandoffPath, data, 0644)
}

func (hrm *HotReloadManager) readHandoff() (*HandoffState, error) {
	data, err := os.ReadFile(hrm.HandoffPath)
	if err != nil {
		return nil, err
	}
	var state HandoffState
	if err := json.Unmarshal(data, &state); err != nil {
		return nil, err
	}
	return &state, nil
}
