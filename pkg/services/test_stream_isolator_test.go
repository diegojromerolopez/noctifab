package services

import (
	"context"
	"os/exec"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDetectTestStartAndEnd(t *testing.T) {
	// Go tests
	assert.Equal(t, "TestServerListen", detectTestStart("=== RUN   TestServerListen"))
	assert.Equal(t, "TestClientConnect", detectTestStart(`{"Action":"run","Test":"TestClientConnect"}`))
	assert.Equal(t, "TestServerListen", detectTestEnd("--- PASS: TestServerListen (0.01s)"))
	assert.Equal(t, "TestClientConnect", detectTestEnd(`{"Action":"pass","Test":"TestClientConnect"}`))

	// Rust tests
	assert.Equal(t, "test_socket_connect", detectTestStart("test test_socket_connect ..."))
	assert.Equal(t, "ended", detectTestEnd("test test_socket_connect ... ok"))

	// Python tests
	assert.Equal(t, "test_ping", detectTestStart("test_ping (test_core.TestPing) ..."))
	assert.Equal(t, "ended", detectTestEnd("test_ping (test_core.TestPing) ... ok"))
	assert.Equal(t, "tests/test_api.py::test_login", detectTestStart("tests/test_api.py::test_login"))
	assert.Equal(t, "ended", detectTestEnd("tests/test_api.py::test_login PASSED"))
}

func TestExtractBlockedLocation(t *testing.T) {
	dump := `goroutine 18 [chan receive]:
github.com/diegojromerolopez/noctifab/pkg/services.TestServerHang(0x140001221a0)
	/Users/diegoj/repos/noctifab/pkg/services/server.go:42 +0x64
testing.tRunner(0x140001221a0, 0x10237db80)
	/opt/homebrew/Cellar/go/1.22.0/src/testing/testing.go:1689 +0xf4
`
	loc := extractBlockedLocation(dump)
	assert.Contains(t, loc, "server.go:42")
}

func TestTestStreamIsolator_CleanExecution(t *testing.T) {
	cmd := exec.Command("sh", "-c", "echo '=== RUN   TestOne'; echo '--- PASS: TestOne'; echo '=== RUN   TestTwo'; echo '--- PASS: TestTwo'")
	isolator := NewTestStreamIsolator(StreamIsolatorConfig{
		PerTestTimeout: 5 * time.Second,
		MaxDuration:    15 * time.Second,
		IdleTimeout:    5 * time.Second,
	})

	out, err := isolator.Run(context.Background(), cmd)
	require.NoError(t, err)
	assert.Contains(t, string(out), "TestOne")
	assert.Contains(t, string(out), "TestTwo")
}

func TestTestStreamIsolator_HungTestIsolation(t *testing.T) {
	// A script that starts TestHang and hangs indefinitely
	cmd := exec.Command("sh", "-c", "echo '=== RUN   TestHang'; sleep 10")

	var killedSignals []syscall.Signal
	isolator := NewTestStreamIsolator(StreamIsolatorConfig{
		PerTestTimeout: 100 * time.Millisecond,
		MaxDuration:    5 * time.Second,
		IdleTimeout:    5 * time.Second,
	})
	isolator.killSignal = func(pid int, sig syscall.Signal) error {
		killedSignals = append(killedSignals, sig)
		return nil
	}

	out, err := isolator.Run(context.Background(), cmd)
	require.Error(t, err)

	var timeoutErr *ErrPerTestTimeout
	require.ErrorAs(t, err, &timeoutErr)
	assert.Equal(t, "TestHang", timeoutErr.Result.HungTest)
	assert.Contains(t, string(out), "Per-Test Timeout & Hang Isolated")
	assert.Contains(t, string(out), "Test: TestHang")
	assert.Contains(t, killedSignals, syscall.SIGQUIT)
	assert.Contains(t, killedSignals, syscall.SIGKILL)
}
