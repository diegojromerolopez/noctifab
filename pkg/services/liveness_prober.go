package services

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/diegojromerolopez/noctifab/pkg/infrastructure/telemetry"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

// LivenessVector identifies the probing mechanism.
type LivenessVector string

const (
	VectorSocket  LivenessVector = "socket"
	VectorHTTP    LivenessVector = "http"
	VectorProcess LivenessVector = "process"
	VectorScript  LivenessVector = "script"
)

// ProbeTarget defines a single diagnostic vector target.
type ProbeTarget struct {
	Vector   LivenessVector `json:"vector"`
	Address  string         `json:"address,omitempty"`
	Path     string         `json:"path,omitempty"`
	PID      int            `json:"pid,omitempty"`
	Required bool           `json:"required"`
}

// ProbeResult captures empirical ground truth from an individual vector check.
type ProbeResult struct {
	Vector   LivenessVector `json:"vector"`
	Target   string         `json:"target"`
	Alive    bool           `json:"alive"`
	Latency  time.Duration  `json:"latency"`
	Details  string         `json:"details"`
	Error    string         `json:"error,omitempty"`
	Required bool           `json:"required"`
}

// LivenessDiagnosis is the aggregated multi-vector report.
type LivenessDiagnosis struct {
	Healthy  bool          `json:"healthy"`
	Duration time.Duration `json:"duration"`
	Results  []ProbeResult `json:"results"`
	Summary  string        `json:"summary"`
}

// LivenessProber coordinates concurrent multi-vector service diagnosis.
type LivenessProber struct {
	runner     Sandbox
	timeout    time.Duration
	dialer     func(ctx context.Context, network, address string) (net.Conn, error)
	httpClient *http.Client
}

// NewLivenessProber creates a new LivenessProber instance.
func NewLivenessProber(runner Sandbox, timeout time.Duration) *LivenessProber {
	if timeout <= 0 {
		timeout = 3 * time.Second
	}
	return &LivenessProber{
		runner:  runner,
		timeout: timeout,
		dialer: func(ctx context.Context, network, address string) (net.Conn, error) {
			var d net.Dialer
			return d.DialContext(ctx, network, address)
		},
		httpClient: &http.Client{
			Timeout: timeout,
		},
	}
}

// DiagnoseParallel executes all probe vectors simultaneously across parallel goroutines,
// eliminating serial socket and HTTP timeouts during failure diagnosis.
func (p *LivenessProber) DiagnoseParallel(ctx context.Context, projectPath string, targets []ProbeTarget) *LivenessDiagnosis {
	start := time.Now()
	ctx, span := telemetry.Tracer().Start(ctx, "LivenessProber.DiagnoseParallel",
		trace.WithAttributes(attribute.Int("target_count", len(targets))))
	defer span.End()

	if len(targets) == 0 {
		targets = p.AutoDetectTargets(projectPath)
	}
	if len(targets) == 0 {
		return &LivenessDiagnosis{
			Healthy:  true,
			Duration: time.Since(start),
			Summary:  "No diagnostic probe targets detected or configured",
		}
	}

	results := make([]ProbeResult, len(targets))
	var wg sync.WaitGroup
	wg.Add(len(targets))

	for i, target := range targets {
		go func(idx int, tgt ProbeTarget) {
			defer wg.Done()
			probeCtx, cancel := context.WithTimeout(ctx, p.timeout)
			defer cancel()
			results[idx] = p.runSingleProbe(probeCtx, projectPath, tgt)
		}(i, target)
	}

	wg.Wait()
	duration := time.Since(start)

	healthy := true
	aliveCount := 0
	for _, res := range results {
		if res.Alive {
			aliveCount++
		} else if res.Required {
			healthy = false
		}
	}
	if len(results) > 0 && aliveCount == 0 {
		healthy = false
	}

	summary := fmt.Sprintf("Multi-vector diagnosis: %d/%d vectors alive in %v", aliveCount, len(results), duration)
	return &LivenessDiagnosis{
		Healthy:  healthy,
		Duration: duration,
		Results:  results,
		Summary:  summary,
	}
}

func (p *LivenessProber) runSingleProbe(ctx context.Context, projectPath string, tgt ProbeTarget) ProbeResult {
	start := time.Now()
	res := ProbeResult{
		Vector:   tgt.Vector,
		Required: tgt.Required,
	}

	switch tgt.Vector {
	case VectorSocket:
		res.Target = tgt.Address
		conn, err := p.dialer(ctx, "tcp", tgt.Address)
		res.Latency = time.Since(start)
		if err != nil {
			res.Alive = false
			res.Error = err.Error()
			res.Details = fmt.Sprintf("TCP dial failed: %v", err)
		} else {
			_ = conn.Close()
			res.Alive = true
			res.Details = "TCP socket connection established"
		}

	case VectorHTTP:
		res.Target = tgt.Address
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, tgt.Address, nil)
		if err != nil {
			res.Latency = time.Since(start)
			res.Alive = false
			res.Error = err.Error()
			res.Details = fmt.Sprintf("HTTP request creation failed: %v", err)
			return res
		}
		resp, err := p.httpClient.Do(req)
		res.Latency = time.Since(start)
		if err != nil {
			res.Alive = false
			res.Error = err.Error()
			res.Details = fmt.Sprintf("HTTP probe request failed: %v", err)
		} else {
			_ = resp.Body.Close()
			res.Alive = resp.StatusCode < 500
			res.Details = fmt.Sprintf("HTTP status %d", resp.StatusCode)
			if !res.Alive {
				res.Error = fmt.Sprintf("Server returned 5xx status: %d", resp.StatusCode)
			}
		}

	case VectorProcess:
		res.Target = fmt.Sprintf("PID %d", tgt.PID)
		if tgt.PID <= 0 {
			res.Alive = false
			res.Error = "invalid PID <= 0"
			return res
		}
		proc, err := os.FindProcess(tgt.PID)
		res.Latency = time.Since(start)
		if err != nil {
			res.Alive = false
			res.Error = err.Error()
			res.Details = "process not found"
		} else {
			signalErr := proc.Signal(syscall.Signal(0))
			res.Alive = (signalErr == nil)
			if res.Alive {
				res.Details = "process responding to signal 0"
			} else {
				res.Error = signalErr.Error()
				res.Details = fmt.Sprintf("process signal check failed: %v", signalErr)
			}
		}

	case VectorScript:
		res.Target = tgt.Path
		if p.runner == nil {
			res.Alive = false
			res.Error = "no sandbox runner configured for script probe"
			return res
		}
		cmd := "sh " + tgt.Path
		out, err := p.runner.RunCommand(ctx, projectPath, cmd, "")
		res.Latency = time.Since(start)
		if err != nil {
			res.Alive = false
			res.Error = err.Error()
			res.Details = strings.TrimSpace(out)
		} else {
			res.Alive = true
			res.Details = strings.TrimSpace(out)
		}

	default:
		res.Alive = false
		res.Error = fmt.Sprintf("unsupported vector %q", tgt.Vector)
	}

	return res
}

// AutoDetectTargets discovers potential probe scripts in the project repository.
func (p *LivenessProber) AutoDetectTargets(projectPath string) []ProbeTarget {
	if projectPath == "" {
		return nil
	}
	candidates := []string{
		"tests/probe_interface.sh",
		"tests/probe_liveness.sh",
		"tests/probe_socket.sh",
		"scripts/probe_liveness.sh",
	}
	var detected []ProbeTarget
	for _, rel := range candidates {
		full := filepath.Join(projectPath, rel)
		if info, err := os.Stat(full); err == nil && !info.IsDir() {
			detected = append(detected, ProbeTarget{
				Vector:   VectorScript,
				Path:     rel,
				Required: true,
			})
		}
	}
	return detected
}
