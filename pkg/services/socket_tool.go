package services

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"strconv"
	"strings"
	"time"

	"github.com/diegojromerolopez/noctifab/pkg/domain"
)

// SocketCheckRequest holds parameters for testing socket connectivity.
type SocketCheckRequest struct {
	Address          string
	Network          string
	Timeout          time.Duration
	WaitDuration     time.Duration
	Payload          []byte
	ExpectedResponse string
}

// SocketCheckResult holds the outcome of a socket connectivity check.
type SocketCheckResult struct {
	Connected  bool
	Latency    time.Duration
	Response   string
	Matched    bool
	Error      string
	Diagnostic string
}

// CheckSocket performs deterministic socket connectivity checks with optional polling and payload assertion.
func CheckSocket(ctx context.Context, req SocketCheckRequest) (*SocketCheckResult, error) {
	network := strings.ToLower(strings.TrimSpace(req.Network))
	if network == "" {
		network = "tcp"
	}
	addr := strings.TrimSpace(req.Address)
	if addr == "" {
		return nil, errors.New("missing or empty address")
	}
	if network == "tcp" || network == "tcp4" || network == "tcp6" {
		if strings.HasPrefix(addr, ":") {
			addr = "127.0.0.1" + addr
		} else if !strings.Contains(addr, ":") && !strings.HasPrefix(addr, "/") {
			addr = "127.0.0.1:" + addr
		}
	}

	timeout := req.Timeout
	if timeout <= 0 {
		timeout = 3 * time.Second
	}
	waitDuration := req.WaitDuration
	if waitDuration < 0 {
		waitDuration = 0
	}

	deadline := time.Now().Add(waitDuration)
	var lastErr error
	var conn net.Conn
	var latency time.Duration

	for {
		start := time.Now()
		dialer := net.Dialer{Timeout: timeout}
		dialCtx, cancel := context.WithTimeout(ctx, timeout)
		c, err := dialer.DialContext(dialCtx, network, addr)
		cancel()

		if err == nil {
			conn = c
			latency = time.Since(start)
			break
		}
		lastErr = err

		if time.Now().After(deadline) || ctx.Err() != nil {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}

	res := &SocketCheckResult{
		Connected: conn != nil,
		Latency:   latency,
	}

	if conn == nil {
		res.Error = fmt.Sprintf("%v", lastErr)
		if strings.Contains(res.Error, "connection refused") {
			res.Diagnostic = fmt.Sprintf("Target port %s (%s) is closed or not listening. Verify the daemon is running.", addr, network)
		} else if strings.Contains(res.Error, "timeout") || strings.Contains(res.Error, "deadline exceeded") {
			res.Diagnostic = fmt.Sprintf("Connection to %s timed out. Verify network firewalls and binding interfaces.", addr)
		} else {
			res.Diagnostic = fmt.Sprintf("Failed to connect to %s (%s): %v", addr, network, lastErr)
		}
		return res, nil
	}
	defer func() { _ = conn.Close() }()

	if len(req.Payload) > 0 {
		_ = conn.SetDeadline(time.Now().Add(timeout))
		if _, err := conn.Write(req.Payload); err != nil {
			res.Error = fmt.Sprintf("write payload failed: %v", err)
			res.Diagnostic = "Connection succeeded, but sending payload failed."
			return res, nil
		}

		buf := make([]byte, 16384)
		n, err := conn.Read(buf)
		if err != nil && !errors.Is(err, io.EOF) {
			res.Error = fmt.Sprintf("read response failed: %v", err)
			res.Diagnostic = "Payload was sent, but reading response failed or timed out."
			return res, nil
		}
		res.Response = string(buf[:n])

		if req.ExpectedResponse != "" {
			res.Matched = strings.Contains(res.Response, req.ExpectedResponse)
		} else {
			res.Matched = true
		}
	} else {
		res.Matched = true
	}

	return res, nil
}

// CheckSocketTool implements check_socket for LLM agent socket probing.
type CheckSocketTool struct{}

// Name returns the unique tool identifier.
func (t *CheckSocketTool) Name() string { return "check_socket" }

// Description returns LLM documentation for check_socket.
func (t *CheckSocketTool) Description() string {
	return "check_socket tests a TCP/UDP socket or port connectivity. Arguments: address (string, required, e.g. '127.0.0.1:6379' or ':8080'), network (string, optional, default 'tcp'), timeout_seconds (number, optional, default 3), payload (string, optional), expected_response (string, optional), wait_seconds (number, optional, polls until open)."
}

// Execute performs socket connectivity testing.
func (t *CheckSocketTool) Execute(ctx context.Context, state *domain.State, args map[string]any) (string, error) {
	addr, _ := args["address"].(string)
	if strings.TrimSpace(addr) == "" {
		return "", errors.New("missing or empty 'address' argument")
	}

	network, _ := args["network"].(string)
	payloadStr, _ := args["payload"].(string)
	expectedResp, _ := args["expected_response"].(string)
	timeoutSec := extractNetworkFloat(args["timeout_seconds"], 3.0)
	waitSec := extractNetworkFloat(args["wait_seconds"], 0.0)

	req := SocketCheckRequest{
		Address:          addr,
		Network:          network,
		Timeout:          time.Duration(timeoutSec * float64(time.Second)),
		WaitDuration:     time.Duration(waitSec * float64(time.Second)),
		Payload:          []byte(payloadStr),
		ExpectedResponse: expectedResp,
	}

	res, err := CheckSocket(ctx, req)
	if err != nil {
		return "", err
	}

	var sb strings.Builder
	if res.Connected {
		sb.WriteString("Status: OPEN\n")
		sb.WriteString("Address: ")
		sb.WriteString(addr)
		sb.WriteString("\nLatency: ")
		sb.WriteString(res.Latency.String())
		sb.WriteString("\n")
		if len(payloadStr) > 0 {
			sb.WriteString("Payload Sent: ")
			sb.WriteString(strconv.Itoa(len(payloadStr)))
			sb.WriteString(" bytes\nResponse: ")
			sb.WriteString(res.Response)
			sb.WriteString("\nAssertion: ")
			if res.Matched {
				sb.WriteString("PASSED\n")
			} else {
				sb.WriteString("FAILED (expected response not found)\n")
			}
		}
	} else {
		sb.WriteString("Status: CLOSED / UNREACHABLE\n")
		sb.WriteString("Address: ")
		sb.WriteString(addr)
		sb.WriteString("\nError: ")
		sb.WriteString(res.Error)
		sb.WriteString("\nDiagnostic: ")
		sb.WriteString(res.Diagnostic)
		sb.WriteString("\n")
	}

	return sb.String(), nil
}
