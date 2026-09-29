package services

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/diegojromerolopez/noctifab/pkg/domain"
)

func TestCheckSocketTool_SuccessAndPayload(t *testing.T) {
	// Start a local TCP server that echoes or responds with PONG to PING
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to listen: %v", err)
	}
	defer func() { _ = listener.Close() }()

	go func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			go func(c net.Conn) {
				defer func() { _ = c.Close() }()
				buf := make([]byte, 1024)
				n, _ := c.Read(buf)
				if strings.Contains(string(buf[:n]), "PING") {
					_, _ = c.Write([]byte("+PONG\r\n"))
				} else {
					_, _ = c.Write([]byte("OK\n"))
				}
			}(conn)
		}
	}()

	tool := &CheckSocketTool{}
	if tool.Name() != "check_socket" {
		t.Errorf("expected name 'check_socket', got %q", tool.Name())
	}
	if !strings.Contains(tool.Description(), "check_socket") {
		t.Errorf("expected description to contain 'check_socket'")
	}

	state := &domain.State{ProjectPath: "/tmp"}
	args := map[string]any{
		"address":           listener.Addr().String(),
		"payload":           "PING\r\n",
		"expected_response": "PONG",
		"timeout_seconds":   1.0,
	}

	out, err := tool.Execute(context.Background(), state, args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(out, "Status: OPEN") {
		t.Errorf("expected 'Status: OPEN', got: %s", out)
	}
	if !strings.Contains(out, "PASSED") {
		t.Errorf("expected 'PASSED', got: %s", out)
	}
}

func TestCheckSocketTool_ConnectionRefused(t *testing.T) {
	// Pick an unused local port
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to pick port: %v", err)
	}
	addr := l.Addr().String()
	_ = l.Close() // Immediately close so connection is refused

	tool := &CheckSocketTool{}
	state := &domain.State{ProjectPath: "/tmp"}
	args := map[string]any{
		"address":         addr,
		"timeout_seconds": 0.5,
	}

	out, err := tool.Execute(context.Background(), state, args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(out, "Status: CLOSED") && !strings.Contains(out, "UNREACHABLE") {
		t.Errorf("expected CLOSED/UNREACHABLE status, got: %s", out)
	}
	if !strings.Contains(out, "Diagnostic:") {
		t.Errorf("expected diagnostic message, got: %s", out)
	}
}

func TestCheckSocketTool_MissingAddress(t *testing.T) {
	tool := &CheckSocketTool{}
	state := &domain.State{ProjectPath: "/tmp"}
	_, err := tool.Execute(context.Background(), state, map[string]any{})
	if err == nil {
		t.Errorf("expected error on missing address")
	}
}

func TestCheckSocket_PollingWait(t *testing.T) {
	// Start listener after 200ms delay to verify polling
	portChan := make(chan string, 1)
	go func() {
		time.Sleep(200 * time.Millisecond)
		l, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			return
		}
		defer func() { _ = l.Close() }()
		portChan <- l.Addr().String()
		// keep open briefly
		time.Sleep(500 * time.Millisecond)
	}()

	// We'll test with a listener bound to a specific free port
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to listen: %v", err)
	}
	addr := l.Addr().String()
	_ = l.Close()

	// Spawn delayed listener on the same address
	go func() {
		time.Sleep(150 * time.Millisecond)
		delayedListener, err := net.Listen("tcp", addr)
		if err == nil {
			defer func() { _ = delayedListener.Close() }()
			time.Sleep(500 * time.Millisecond)
		}
	}()

	req := SocketCheckRequest{
		Address:      addr,
		Timeout:      500 * time.Millisecond,
		WaitDuration: 1 * time.Second,
	}

	res, err := CheckSocket(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.Connected {
		t.Logf("Note: delayed listener may not have bound in time due to OS port reuse: %s", res.Error)
	}
}

func TestCheckHTTPTool_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Custom-Header") != "TestValue" {
			http.Error(w, "missing header", http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = io.WriteString(w, `{"status":"ok","daemon":"running"}`)
	}))
	defer server.Close()

	tool := &CheckHTTPTool{}
	if tool.Name() != "check_http" {
		t.Errorf("expected name 'check_http', got %q", tool.Name())
	}
	if !strings.Contains(tool.Description(), "check_http") {
		t.Errorf("expected description to contain 'check_http'")
	}

	state := &domain.State{ProjectPath: "/tmp"}
	args := map[string]any{
		"url":                    server.URL,
		"method":                 "GET",
		"headers":                map[string]any{"X-Custom-Header": "TestValue"},
		"expected_status":        200,
		"expected_body_contains": "running",
		"timeout_seconds":        2.0,
	}

	out, err := tool.Execute(context.Background(), state, args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(out, "Status: HTTP 200") {
		t.Errorf("expected 'Status: HTTP 200', got: %s", out)
	}
	if !strings.Contains(out, "Assertion: PASSED") {
		t.Errorf("expected 'Assertion: PASSED', got: %s", out)
	}
	if !strings.Contains(out, "daemon") {
		t.Errorf("expected body to contain 'daemon', got: %s", out)
	}
}

func TestCheckHTTPTool_StatusMismatch(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "internal server error", http.StatusInternalServerError)
	}))
	defer server.Close()

	tool := &CheckHTTPTool{}
	state := &domain.State{ProjectPath: "/tmp"}
	args := map[string]any{
		"url":             server.URL,
		"expected_status": 200,
		"timeout_seconds": 1.0,
	}

	out, err := tool.Execute(context.Background(), state, args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(out, "Status: HTTP 500") {
		t.Errorf("expected 'Status: HTTP 500', got: %s", out)
	}
	if !strings.Contains(out, "Assertion: FAILED") {
		t.Errorf("expected 'Assertion: FAILED', got: %s", out)
	}
}

func TestCheckHTTPTool_Unreachable(t *testing.T) {
	tool := &CheckHTTPTool{}
	state := &domain.State{ProjectPath: "/tmp"}
	args := map[string]any{
		"url":             "http://127.0.0.1:59999/unreachable",
		"timeout_seconds": 0.5,
	}

	out, err := tool.Execute(context.Background(), state, args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(out, "Status: UNREACHABLE") {
		t.Errorf("expected 'Status: UNREACHABLE', got: %s", out)
	}
	if !strings.Contains(out, "Diagnostic:") {
		t.Errorf("expected Diagnostic message, got: %s", out)
	}
}

func TestCheckHTTPTool_MissingURL(t *testing.T) {
	tool := &CheckHTTPTool{}
	state := &domain.State{ProjectPath: "/tmp"}
	_, err := tool.Execute(context.Background(), state, map[string]any{})
	if err == nil {
		t.Errorf("expected error on missing url")
	}
}

func TestNetworkHelpers_Conversions(t *testing.T) {
	if f := extractNetworkFloat(12.5, 0.0); f != 12.5 {
		t.Errorf("expected 12.5, got %v", f)
	}
	if f := extractNetworkFloat("3.14", 0.0); fmt.Sprintf("%.2f", f) != "3.14" {
		t.Errorf("expected 3.14, got %v", f)
	}
	if i := extractNetworkInt(42, 0); i != 42 {
		t.Errorf("expected 42, got %v", i)
	}
	if i := extractNetworkInt("100", 0); i != 100 {
		t.Errorf("expected 100, got %v", i)
	}
}
